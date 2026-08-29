package testutil

import (
	"net/http"
	"net/http/httptest"

	"github.com/gophercloud/gophercloud/v2"
)

// FakeServiceClient creates a gophercloud.ServiceClient backed by the given
// http.Handler. All HTTP requests made through the returned client will be
// served by the handler. The returned cleanup func closes the backing test
// server; call it (typically via defer) when the client is no longer needed
// to avoid leaking a listener and goroutine.
func FakeServiceClient(handler http.Handler) (*gophercloud.ServiceClient, func()) {
	srv := httptest.NewServer(handler)
	sc := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
	return sc, srv.Close
}

// FakeServiceClientWithFixture creates a gophercloud.ServiceClient that
// responds with the given JSON body for the given URL path. All other paths
// return 404. See FakeServiceClient for cleanup semantics.
func FakeServiceClientWithFixture(jsonBody, path string) (*gophercloud.ServiceClient, func()) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path || r.URL.Path == path+"/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(jsonBody))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	return FakeServiceClient(handler)
}
