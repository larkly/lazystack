package testutil

import (
	"net/http"
	"net/http/httptest"

	"github.com/gophercloud/gophercloud/v2"
)

// FakeServiceClient creates a gophercloud.ServiceClient backed by the given
// http.Handler. All HTTP requests made through the returned client will be
// served by the handler. Suitable for unit testing Gophercloud API callers.
func FakeServiceClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	sc := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
	return sc
}

// FakeServiceClientWithFixture creates a gophercloud.ServiceClient that
// responds with the given JSON body for the given URL path. All other paths
// return 404.
func FakeServiceClientWithFixture(jsonBody, path string) *gophercloud.ServiceClient {
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
