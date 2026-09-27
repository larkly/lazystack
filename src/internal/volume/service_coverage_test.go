package volume

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/testutil"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAttachmentHTTPFailuresAndFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name, listing            string
		listStatus, deleteStatus int
		wantDelete               bool
		wantErr                  string
	}{
		// Before microversion 2.89 Nova reports the volume ID as "id".
		{"resolved", `{"volumeAttachments":[{"id":"other","volumeId":"other"},{"id":"vol","volumeId":"vol"}]}`, 200, 204, true, ""},
		// From 2.89 "id" is gone; the Cinder attachment UUID must not be used as the path key.
		{"resolved 2.89", `{"volumeAttachments":[{"attachment_id":"cinder-uuid","volumeId":"vol"}]}`, 200, 204, true, ""},
		{"unmatched", `{"volumeAttachments":[]}`, 200, 204, true, ""},
		{"lookup forbidden", `{}`, 403, 204, false, "403"},
		{"lookup server error", `{}`, 500, 204, false, "500"},
		{"malformed lookup", `{"volumeAttachments":"bad"}`, 200, 204, false, "looking up attachment"},
		{"delete failure", `{"volumeAttachments":[{"id":"vol","volumeId":"vol"}]}`, 200, 409, true, "409"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					w.WriteHeader(tc.listStatus)
					w.Write([]byte(tc.listing))
					return
				}
				w.WriteHeader(tc.deleteStatus)
			}))
			defer close()
			err := DetachVolume(context.Background(), client, "vm", "vol")
			want := []string{"GET /servers/vm/os-volume_attachments"}
			if tc.wantDelete {
				want = append(want, "DELETE /servers/vm/os-volume_attachments/vol")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls=%v want=%v", calls, want)
			}
			if (err != nil) != (tc.wantErr != "") {
				t.Fatalf("err=%v", err)
			}
			if err != nil && (!strings.Contains(err.Error(), "detaching volume vol from server vm") || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatal(err)
			}
			if tc.listStatus != 200 && !gophercloud.ResponseCodeIs(err, tc.listStatus) {
				t.Fatalf("lookup status %d not preserved: %v", tc.listStatus, err)
			}
		})
	}
	t.Run("lookup cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		deletes := 0
		client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "DELETE" {
				deletes++
			}
			w.WriteHeader(204)
		}))
		defer close()
		err := DetachVolume(ctx, client, "vm", "vol")
		if !errors.Is(err, context.Canceled) || deletes != 0 {
			t.Fatalf("deletes=%d err=%v", deletes, err)
		}
	})
	for _, status := range []int{200, 409} {
		t.Run("attach/"+http.StatusText(status), func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/servers/vm/os-volume_attachments" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(body, map[string]map[string]any{"volumeAttachment": {"volumeId": "vol"}}) {
					t.Errorf("body=%v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"volumeAttachment":{"id":"attachment","volumeId":"vol"}}`))
			}))
			defer close()
			id, err := AttachVolume(context.Background(), client, "vm", "vol")
			if status == 200 {
				if err != nil || id != "attachment" {
					t.Fatalf("id=%q err=%v", id, err)
				}
			} else if err == nil || id != "" || !strings.Contains(err.Error(), "attaching volume vol to server vm") {
				t.Fatalf("id=%q err=%v", id, err)
			}
		})
	}
}
func TestListVolumeTypesHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{{"mapped", `{"volume_types":[{"id":"fast","name":"SSD"}]}`, 200, false}, {"empty", `{"volume_types":[]}`, 200, false}, {"malformed", `{"volume_types":"bad"}`, 200, true}, {"forbidden", `{}`, 403, true}} {
		t.Run(tc.name, func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/types" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer close()
			got, err := ListVolumeTypes(context.Background(), client)
			if (err != nil) != tc.fail {
				t.Fatalf("got=%v err=%v", got, err)
			}
			if tc.fail {
				if got != nil || !strings.Contains(err.Error(), "listing volume types") {
					t.Fatalf("got=%v err=%v", got, err)
				}
			} else if tc.name == "mapped" && !reflect.DeepEqual(got, []VolumeType{{ID: "fast", Name: "SSD"}}) {
				t.Fatalf("got=%v", got)
			} else if tc.name == "empty" && len(got) != 0 {
				t.Fatal(got)
			}
		})
	}
}
