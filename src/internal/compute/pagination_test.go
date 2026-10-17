package compute

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// pagedNova serves a list endpoint in pages: each request without a marker
// returns the first page, "?marker=pN" returns page N+1, and every page but
// the last carries a rel=next link under linksKey, as Nova does from 2.35
// (keypairs) and 2.58 (instance actions).
func pagedNova(t *testing.T, path, listKey, linksKey string, pages [][]string) (*gophercloud.ServiceClient, *atomic.Int32) {
	t.Helper()
	requests := new(atomic.Int32)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		n := 0
		if m := r.URL.Query().Get("marker"); m != "" {
			fmt.Sscanf(m, "p%d", &n)
		}
		if n >= len(pages) {
			http.NotFound(w, r)
			return
		}
		links := "[]"
		if n+1 < len(pages) {
			links = fmt.Sprintf(`[{"rel":"next","href":"%s%s?limit=1&marker=p%d"}]`, srv.URL, path, n+1)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{%q:[`, listKey)
		for i, item := range pages[n] {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprint(w, item)
		}
		fmt.Fprintf(w, `],%q:%s}`, linksKey, links)
	}))
	t.Cleanup(srv.Close)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: *srv.Client()},
		Endpoint:       srv.URL + "/",
	}, requests
}

func TestListKeyPairsFollowsNextLinks(t *testing.T) {
	client, requests := pagedNova(t, "/os-keypairs", "keypairs", "keypairs_links", [][]string{
		{`{"keypair":{"name":"first","type":"ssh"}}`},
		{`{"keypair":{"name":"second","type":"ssh"}}`},
	})
	got, err := ListKeyPairs(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Errorf("keypairs = %+v, want first and second", got)
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestListActionsFollowsNextLinks(t *testing.T) {
	client, requests := pagedNova(t, "/servers/vm/os-instance-actions", "instanceActions", "links", [][]string{
		{`{"action":"reboot","request_id":"req-2","start_time":"2026-01-15T11:00:00"}`},
		{`{"action":"create","request_id":"req-1","start_time":"2026-01-15T10:00:00"}`},
	})
	got, err := ListActions(context.Background(), client, "vm")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RequestID != "req-2" || got[1].RequestID != "req-1" {
		t.Errorf("actions = %+v, want req-2 and req-1", got)
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

// A server that keeps returning a next link must not be followed forever.
func TestListKeyPairsStopsAfterPageCap(t *testing.T) {
	pages := make([][]string, maxListPages+5)
	for i := range pages {
		pages[i] = []string{fmt.Sprintf(`{"keypair":{"name":"k%d","type":"ssh"}}`, i)}
	}
	client, requests := pagedNova(t, "/os-keypairs", "keypairs", "keypairs_links", pages)
	got, err := ListKeyPairs(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != maxListPages || len(got) != maxListPages {
		t.Errorf("requests = %d, keypairs = %d, want %d of each", requests.Load(), len(got), maxListPages)
	}
}
