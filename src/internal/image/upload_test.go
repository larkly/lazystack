package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func TestUploadReaderDeliversExactBytesAndRewinds(t *testing.T) {
	u := NewUploadReader(strings.NewReader("hello world"), 11)
	buf := make([]byte, 4)
	var got []byte
	for {
		n, err := u.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if string(got) != "hello world" || u.BytesRead() != 11 {
		t.Fatalf("got %q (%d bytes counted)", got, u.BytesRead())
	}
	if _, err := u.Read(buf); !errors.Is(err, errUploadReplayed) {
		t.Fatalf("read after EOF without rewind: err=%v, want errUploadReplayed", err)
	}
	if _, err := u.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if u.BytesRead() != 0 {
		t.Fatalf("BytesRead after rewind = %d, want 0", u.BytesRead())
	}
	again, err := io.ReadAll(u)
	if err != nil || string(again) != "hello world" {
		t.Fatalf("replay = %q, %v", again, err)
	}
}

func TestUploadReaderRejectsShortAndLongSources(t *testing.T) {
	if _, err := io.ReadAll(NewUploadReader(strings.NewReader("short"), 10)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short source err = %v, want io.ErrUnexpectedEOF", err)
	}
	if _, err := io.ReadAll(NewUploadReader(strings.NewReader("much too long"), 4)); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("long source err = %v, want size error", err)
	}
	data, err := io.ReadAll(NewUploadReader(strings.NewReader(""), 0))
	if err != nil || len(data) != 0 {
		t.Fatalf("empty source = %q, %v", data, err)
	}
}

func TestUploadReaderConcurrentProgress(t *testing.T) {
	u := NewUploadReader(strings.NewReader(strings.Repeat("a", 1000)), 1000)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, u)
	}()
	for i := 0; i < 100; i++ {
		_ = u.BytesRead()
	}
	wg.Wait()
	if u.BytesRead() != 1000 {
		t.Fatalf("BytesRead = %d, want 1000", u.BytesRead())
	}
}

type uploadServer struct {
	mu         sync.Mutex
	putStatus  []int
	bodies     []string
	storedSize int64
}

func (s *uploadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/images/img-1/file":
		data, _ := io.ReadAll(r.Body)
		s.bodies = append(s.bodies, string(data))
		status := s.putStatus[min(len(s.bodies)-1, len(s.putStatus)-1)]
		w.WriteHeader(status)
	case r.Method == http.MethodGet && r.URL.Path == "/images/img-1":
		size := s.storedSize
		if size < 0 {
			size = int64(len(s.bodies[len(s.bodies)-1]))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"img-1","status":"active","size":%d}`, size)
	default:
		http.NotFound(w, r)
	}
}

func uploadClient(t *testing.T, s *uploadServer) *gophercloud.ServiceClient {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	pc := &gophercloud.ProviderClient{HTTPClient: *srv.Client()}
	pc.ReauthFunc = func(context.Context) error { return nil }
	return &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: srv.URL + "/"}
}

func TestUploadImageDataReplaysFullBodyAfterReauth(t *testing.T) {
	s := &uploadServer{putStatus: []int{http.StatusUnauthorized, http.StatusNoContent}, storedSize: -1}
	client := uploadClient(t, s)

	err := UploadImageData(context.Background(), client, "img-1", NewUploadReader(strings.NewReader("image-data"), 10))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.bodies) != 2 || s.bodies[0] != "image-data" || s.bodies[1] != "image-data" {
		t.Fatalf("bodies = %q, want the full body twice", s.bodies)
	}
}

func TestUploadImageDataFailsOnStoredSizeMismatch(t *testing.T) {
	s := &uploadServer{putStatus: []int{http.StatusNoContent}, storedSize: 4}
	client := uploadClient(t, s)

	err := UploadImageData(context.Background(), client, "img-1", NewUploadReader(strings.NewReader("image-data"), 10))
	if err == nil || !strings.Contains(err.Error(), "stored size 4") {
		t.Fatalf("err = %v, want size mismatch", err)
	}
}

func TestUploadImageDataFailsExplicitlyForShortSource(t *testing.T) {
	s := &uploadServer{putStatus: []int{http.StatusNoContent}, storedSize: -1}
	client := uploadClient(t, s)

	err := UploadImageData(context.Background(), client, "img-1", NewUploadReader(strings.NewReader("short"), 10))
	if err == nil {
		t.Fatal("expected an error for a source shorter than its declared size")
	}
}

func TestParseMinimum(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"", 0, true},
		{"  ", 0, true},
		{"0", 0, true},
		{" 20 ", 20, true},
		{"2147483647", 2147483647, true},
		{"2147483648", 0, false},
		{"-1", 0, false},
		{"abc", 0, false},
		{"1.5", 0, false},
		{"99999999999999999999", 0, false},
	} {
		got, err := ParseMinimum("Min Disk", tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("ParseMinimum(%q) = %d, %v; want %d ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "Min Disk") {
			t.Errorf("error %q does not name the field", err)
		}
	}
}
