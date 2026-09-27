package image

import (
	"context"
	"encoding/json"
	"github.com/larkly/lazystack/internal/testutil"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateImagePatchHTTP(t *testing.T) {
	name, visibility := "", "private"
	disk, ram := 0, 1024
	protected := false
	tags := []string{}
	want := []map[string]any{{"op": "replace", "path": "/name", "value": ""}, {"op": "replace", "path": "/visibility", "value": "private"}, {"op": "replace", "path": "/min_disk", "value": float64(0)}, {"op": "replace", "path": "/min_ram", "value": float64(1024)}, {"op": "replace", "path": "/tags", "value": []any{}}, {"op": "replace", "path": "/protected", "value": false}}
	for _, status := range []int{200, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "PATCH" || r.URL.Path != "/images/img" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "application/openstack-images-v2.1-json-patch" {
					t.Errorf("content-type=%q", r.Header.Get("Content-Type"))
				}
				var got []map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("patch=%#v want=%#v", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"id":"img"}`))
			}))
			defer close()
			if err := UpdateImage(context.Background(), client, "img", UpdateImageOpts{}); err != nil || calls != 0 {
				t.Fatalf("empty update: calls=%d err=%v", calls, err)
			}
			err := UpdateImage(context.Background(), client, "img", UpdateImageOpts{Name: &name, Visibility: &visibility, MinDisk: &disk, MinRAM: &ram, Tags: &tags, Protected: &protected})
			if (err != nil) != (status == 403) || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if err != nil && !strings.Contains(err.Error(), "updating image img") {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateImageClearedTagsEncodeEmptyArray(t *testing.T) {
	var nilTags []string
	for _, tc := range []struct {
		name string
		tags *[]string
		want any
	}{
		{"nil slice", &nilTags, []any{}},
		{"empty slice", &[]string{}, []any{}},
		{"values", &[]string{"a", "b"}, []any{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []map[string]any
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"img"}`))
			}))
			defer close()
			if err := UpdateImage(context.Background(), client, "img", UpdateImageOpts{Tags: tc.tags}); err != nil {
				t.Fatal(err)
			}
			want := []map[string]any{{"op": "replace", "path": "/tags", "value": tc.want}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("patch=%#v want=%#v", got, want)
			}
		})
	}
	t.Run("nil pointer leaves tags unchanged", func(t *testing.T) {
		calls := 0
		client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
		defer close()
		if err := UpdateImage(context.Background(), client, "img", UpdateImageOpts{}); err != nil || calls != 0 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}

func TestDownloadImageDataHTTP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		chunked bool
		status  int
		length  int64
	}{{"known", false, 200, 6}, {"unknown", true, 200, -1}, {"not found", false, 404, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/images/img/file" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(tc.status)
				if tc.chunked {
					w.(http.Flusher).Flush()
				}
				w.Write([]byte{0, 1, 2, 3, 254, 255})
			}))
			defer close()
			body, n, err := DownloadImageData(context.Background(), client, "img")
			if tc.status != 200 {
				if err == nil || body != nil || !strings.Contains(err.Error(), "downloading image data img") {
					t.Fatalf("body=%v err=%v", body, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()
			data, err := io.ReadAll(body)
			if err != nil || !reflect.DeepEqual(data, []byte{0, 1, 2, 3, 254, 255}) || n != tc.length {
				t.Fatalf("data=%v length=%d err=%v", data, n, err)
			}
		})
	}
}
