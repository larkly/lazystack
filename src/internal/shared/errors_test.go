package shared

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// newHTTPError creates a real gophercloud ErrUnexpectedResponseCode for testing.
func newHTTPError(statusCode int) gophercloud.ErrUnexpectedResponseCode {
	return gophercloud.ErrUnexpectedResponseCode{
		Actual: statusCode,
		Method: "POST",
		URL:    "http://example.com/test",
	}
}

// Test ParseError for each HTTP status code.
func TestParseError_409Conflict(t *testing.T) {
	err := newHTTPError(http.StatusConflict)
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Conflict — the resource is busy, in use, or the name already exists." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "conflict" {
		t.Errorf("expected category conflict, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != http.StatusConflict {
		t.Errorf("expected status 409, got %d", parsed.HTTPStatusCode)
	}
}

func TestParseError_403Forbidden(t *testing.T) {
	err := newHTTPError(http.StatusForbidden)
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Permission denied — check your role assignments in the cloud console." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "forbidden" {
		t.Errorf("expected category forbidden, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", parsed.HTTPStatusCode)
	}
}

func TestParseError_404NotFound(t *testing.T) {
	err := newHTTPError(http.StatusNotFound)
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Resource not found — it may have been deleted by another user." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "not_found" {
		t.Errorf("expected category not_found, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", parsed.HTTPStatusCode)
	}
}

func TestParseError_400BadRequest(t *testing.T) {
	err := newHTTPError(http.StatusBadRequest)
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Invalid request — check your input values and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "bad_request" {
		t.Errorf("expected category bad_request, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", parsed.HTTPStatusCode)
	}
}

func TestParseError_413QuotaExceeded(t *testing.T) {
	err := newHTTPError(http.StatusRequestEntityTooLarge)
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Quota exceeded — free up resources or contact your cloud admin." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "quota" {
		t.Errorf("expected category quota, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", parsed.HTTPStatusCode)
	}
}

// Test network error patterns.
func TestParseError_NetworkTimeout(t *testing.T) {
	err := errors.New("context deadline exceeded: request timeout")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
	}
}

func TestParseError_ConnectionRefused(t *testing.T) {
	err := errors.New("dial tcp 10.0.0.1:5000: connection refused")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
	}
}

func TestParseError_NetworkUnknown(t *testing.T) {
	err := errors.New("connect: network is unreachable")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
	}
}

func TestParseError_DialError(t *testing.T) {
	err := errors.New("dial tcp 10.0.0.1:5000: lookup api.example.com: no such host")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
	}
}

func TestParseError_IOTimeout(t *testing.T) {
	err := errors.New("net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers) (read tcp 10.0.0.1:5000: i/o timeout)")
	parsed := ParseError(err)

	if parsed.Category != "network" {
		t.Errorf("expected category network for i/o timeout, got %q", parsed.Category)
	}
}

// Resource errors that merely mention the word "network" must NOT be
// classified as connectivity errors (regression test for issue 208).
func TestParseError_ResourceErrorsMentioningNetwork(t *testing.T) {
	cases := []string{
		"Network abc123 not found",
		"network X is in use by port 456",
		"The network 1234567890 could not be deleted: it still has subnets attached",
	}
	for _, raw := range cases {
		parsed := ParseError(errors.New(raw))
		if parsed.Category == "network" {
			t.Errorf("error %q must not be classified as network/connectivity, got category %q (message %q)",
				raw, parsed.Category, parsed.FriendlyMessage)
		}
	}
}

// Test unknown/fallback errors.
func TestParseError_Unknown(t *testing.T) {
	err := errors.New("something went very wrong")
	parsed := ParseError(err)

	if parsed.Categorized() {
		t.Error("unknown error should not be categorized")
	}
	if parsed.Category != "unknown" {
		t.Errorf("expected category unknown, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != 0 {
		t.Errorf("expected status 0, got %d", parsed.HTTPStatusCode)
	}
	// Friendly message should contain the sanitized raw error
	if parsed.FriendlyMessage != "something went very wrong" {
		t.Errorf("unexpected friendly message for unknown: %q", parsed.FriendlyMessage)
	}
}

// Test that URLs are sanitized in raw error.
func TestParseError_URLSanitization(t *testing.T) {
	err := errors.New("failed to connect to https://example.com/api/v1")
	parsed := ParseError(err)

	if len(parsed.RawError) == 0 {
		t.Error("expected non-empty RawError")
	}
	if strings.Contains(parsed.RawError, "https://") {
		t.Errorf("URL not sanitized in RawError: %q", parsed.RawError)
	}
	if !strings.Contains(parsed.RawError, "[endpoint]") {
		t.Errorf("expected [endpoint] replacement in RawError: %q", parsed.RawError)
	}
}

// Test that text surrounding a URL is preserved after sanitization.
// Regression: a broken regex (`[^\\s\"']+` in a raw string) greedily consumed
// past whitespace and mangled the text following the URL.
func TestParseError_PreservesTextAfterURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{
			name:    "keystone auth error with trailing text",
			input:   "Got 401 from https://keystone.foo.com:5000/v3/auth/tokens when authenticating user",
			wantSub: "when authenticating user",
		},
		{
			name:    "network reach error with trailing text",
			input:   "failed to reach https://network.example.com:9696 because the server is down",
			wantSub: "because the server is down",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ParseError(errors.New(tc.input))
			if !strings.Contains(parsed.RawError, tc.wantSub) {
				t.Errorf("text after URL lost: got RawError=%q, want to contain %q",
					parsed.RawError, tc.wantSub)
			}
			if strings.Contains(parsed.RawError, "https://") {
				t.Errorf("URL not sanitized: %q", parsed.RawError)
			}
			if !strings.Contains(parsed.RawError, "[endpoint]") {
				t.Errorf("expected [endpoint] replacement: %q", parsed.RawError)
			}
		})
	}
}

// Test nil error handling.
func TestParseError_Nil(t *testing.T) {
	parsed := ParseError(nil)

	if parsed.FriendlyMessage != "An unexpected error occurred." {
		t.Errorf("unexpected friendly message for nil: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "unknown" {
		t.Errorf("expected category unknown for nil, got %q", parsed.Category)
	}
}

// Test the Error() method.
func TestParsedError_Error(t *testing.T) {
	parsed := &ParsedError{
		FriendlyMessage: "Test message",
		RawError:        "raw stuff",
		Category:        "unknown",
	}
	if parsed.Error() != "Test message" {
		t.Errorf("Error() = %q, want %q", parsed.Error(), "Test message")
	}
}

// Test backward compatibility: SanitizeAPIError delegates to ParseError.
func TestSanitizeAPIError_DelegatesToParseError(t *testing.T) {
	conflictErr := newHTTPError(http.StatusConflict)

	result := SanitizeAPIError(conflictErr)
	expected := "Conflict — the resource is busy, in use, or the name already exists."
	if result != expected {
		t.Errorf("SanitizeAPIError = %q, want %q", result, expected)
	}
}

func TestSanitizeAPIError_NilError(t *testing.T) {
	result := SanitizeAPIError(nil)
	expected := "An unexpected error occurred."
	if result != expected {
		t.Errorf("SanitizeAPIError(nil) = %q, want %q", result, expected)
	}
}

// Test that gophercloud actual error types work.
func TestParseError_GophercloudErrUnexpected(t *testing.T) {
	// Use gophercloud's ErrUnexpectedResponseCode to check it works with real types.
	err := gophercloud.ErrUnexpectedResponseCode{
		Actual: 409,
		Method: "POST",
		URL:    "http://example.com/test",
	}
	parsed := ParseError(err)

	if parsed.Category != "conflict" {
		t.Errorf("expected category conflict for gophercloud error, got %q", parsed.Category)
	}
	if parsed.HTTPStatusCode != 409 {
		t.Errorf("expected status 409, got %d", parsed.HTTPStatusCode)
	}
}

// Helper: check if this is a categorized (non-unknown) error.
func (e *ParsedError) Categorized() bool {
	return e.Category != "unknown"
}

// newHTTPErrorWithBody creates a gophercloud ErrUnexpectedResponseCode
// carrying an API response body.
func newHTTPErrorWithBody(statusCode int, body string) gophercloud.ErrUnexpectedResponseCode {
	e := newHTTPError(statusCode)
	e.Body = []byte(body)
	return e
}

// Nova reports OverQuota as 403 and Neutron as 409; the quota text in the
// body must win over the status code.
func TestParseError_QuotaBodyBeatsStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"nova 403", http.StatusForbidden, `{"forbidden": {"code": 403, "message": "Quota exceeded for cores: Requested 4, but already used 20 of 20 cores"}}`},
		{"neutron 409", http.StatusConflict, `{"NeutronError": {"type": "OverQuota", "message": "Quota exceeded for resources: ['port'].", "detail": ""}}`},
		{"neutron type only", http.StatusConflict, `{"NeutronError": {"type": "OverQuota", "message": "", "detail": ""}}`},
		{"cinder 413", http.StatusRequestEntityTooLarge, `{"overLimit": {"message": "Maximum number of volumes allowed (10) exceeded for quota 'volumes'."}}`},
		{"limit 403", http.StatusForbidden, `{"forbidden": {"message": "Maximum number of key pairs limit exceeded"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ParseError(newHTTPErrorWithBody(tc.status, tc.body))
			if parsed.Category != "quota" {
				t.Errorf("category=%q, want quota", parsed.Category)
			}
			if parsed.FriendlyMessage != "Quota exceeded — free up resources or contact your cloud admin." {
				t.Errorf("friendly=%q", parsed.FriendlyMessage)
			}
			if parsed.HTTPStatusCode != tc.status {
				t.Errorf("status=%d, want %d", parsed.HTTPStatusCode, tc.status)
			}
		})
	}
}

func TestParseError_PlainStatusWithoutQuotaBody(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		category string
	}{
		{http.StatusForbidden, `{"forbidden": {"code": 403, "message": "Policy doesn't allow os_compute_api:servers:create to be performed."}}`, "forbidden"},
		{http.StatusConflict, `{"conflictingRequest": {"code": 409, "message": "Cannot 'reboot' instance while it is in task_state rebooting"}}`, "conflict"},
	} {
		if got := ParseError(newHTTPErrorWithBody(tc.status, tc.body)).Category; got != tc.category {
			t.Errorf("status %d: category=%q, want %q", tc.status, got, tc.category)
		}
	}
}

func TestAPIFaultMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"nova", newHTTPErrorWithBody(409, `{"conflictingRequest": {"code": 409, "message": "Instance srv is locked"}}`), "Instance srv is locked"},
		{"neutron", newHTTPErrorWithBody(409, `{"NeutronError": {"type": "RouterInUse", "message": "Router r1 still has ports", "detail": ""}}`), "Router r1 still has ports"},
		{"octavia", newHTTPErrorWithBody(409, `{"faultcode": "Client", "faultstring": "Load Balancer lb-1 is immutable", "debuginfo": null}`), "Load Balancer lb-1 is immutable"},
		{"trimmed", newHTTPErrorWithBody(400, `{"badRequest": {"message": "  Invalid flavorRef  "}}`), "Invalid flavorRef"},
		{"wrapped", fmt.Errorf("creating server: %w", newHTTPErrorWithBody(400, `{"badRequest": {"message": "Invalid key_name"}}`)), "Invalid key_name"},
		{"not json", newHTTPErrorWithBody(409, `<html>Conflict</html>`), ""},
		{"empty body", newHTTPError(409), ""},
		{"empty message", newHTTPErrorWithBody(409, `{"conflictingRequest": {"code": 409, "message": ""}}`), ""},
		{"not an HTTP error", errors.New("boom"), ""},
		{"nil", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := APIFaultMessage(tc.err); got != tc.want {
				t.Errorf("APIFaultMessage=%q, want %q", got, tc.want)
			}
		})
	}
}

// 409 and 400 carry the API's own reason, which the friendly message must
// not hide.
func TestParseError_AppendsAPIFaultMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"409 nova", newHTTPErrorWithBody(409, `{"conflictingRequest": {"code": 409, "message": "Instance srv is locked"}}`),
			"Conflict — the resource is busy, in use, or the name already exists. (Instance srv is locked)"},
		{"409 octavia", newHTTPErrorWithBody(409, `{"faultstring": "Load Balancer lb-1 is immutable"}`),
			"Conflict — the resource is busy, in use, or the name already exists. (Load Balancer lb-1 is immutable)"},
		{"409 not json", newHTTPErrorWithBody(409, `Conflict`),
			"Conflict — the resource is busy, in use, or the name already exists."},
		{"400 neutron", newHTTPErrorWithBody(400, `{"NeutronError": {"type": "HTTPBadRequest", "message": "Invalid input for name"}}`),
			"Invalid request — check your input values and try again. (Invalid input for name)"},
		{"400 no body", newHTTPError(400),
			"Invalid request — check your input values and try again."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseError(tc.err).FriendlyMessage; got != tc.want {
				t.Errorf("friendly=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseError_401Unauthorized(t *testing.T) {
	parsed := ParseError(fmt.Errorf("listing servers: %w", newHTTPError(http.StatusUnauthorized)))
	if parsed.Category != "unauthorized" {
		t.Errorf("category=%q, want unauthorized", parsed.Category)
	}
	if parsed.FriendlyMessage != "Session expired or credentials rejected — press C to reconnect." {
		t.Errorf("friendly=%q", parsed.FriendlyMessage)
	}
	if parsed.HTTPStatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d", parsed.HTTPStatusCode)
	}
}

func TestParseError_5xxUnavailable(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		parsed := ParseError(newHTTPErrorWithBody(code, `<html>Service Unavailable</html>`))
		if parsed.Category != "unavailable" {
			t.Errorf("%d: category=%q, want unavailable", code, parsed.Category)
		}
		want := fmt.Sprintf("The cloud API is unavailable (HTTP %d) — try again shortly.", code)
		if parsed.FriendlyMessage != want {
			t.Errorf("%d: friendly=%q, want %q", code, parsed.FriendlyMessage, want)
		}
		if parsed.HTTPStatusCode != code {
			t.Errorf("%d: status=%d", code, parsed.HTTPStatusCode)
		}
	}
	if got := ParseError(newHTTPError(501)).Category; got == "unavailable" {
		t.Errorf("501 Not Implemented classified as unavailable")
	}
}
