package shared

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func quotaHTTPError() error {
	return gophercloud.ErrUnexpectedResponseCode{
		Actual: http.StatusForbidden, Method: "POST", URL: "http://example.com/servers",
		Body: []byte(`{"forbidden":{"message":"Quota exceeded for instances"}}`),
	}
}

// ParseError keeps the context around an API error and reduces only the
// API error itself to its friendly message.
func TestContextErrorfKeepsContext(t *testing.T) {
	cause := quotaHTTPError()
	err := ContextErrorf("created 2 of 4 servers (a, b); c failed: %v", cause)

	got := ParseError(err).FriendlyMessage
	want := "created 2 of 4 servers (a, b); c failed: Quota exceeded — free up resources or contact your cloud admin."
	if got != want {
		t.Fatalf("FriendlyMessage = %q, want %q", got, want)
	}
	if !strings.Contains(err.Error(), "http://example.com/servers") {
		t.Errorf("Error() = %q, want the raw cause for logs", err.Error())
	}
	var code gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &code) {
		t.Error("cause is not unwrappable")
	}
	if p := ParseError(err); p.HTTPStatusCode != http.StatusForbidden || strings.Contains(p.RawError, "http://") {
		t.Errorf("parsed = %+v, want status 403 and a sanitized raw error", p)
	}
}

func TestContextErrorfLeavesNonErrorsAlone(t *testing.T) {
	err := ContextErrorf("image %s may be left behind: %v; %d tries", "img-1", errors.New("disk full"), 3)
	if got := ParseError(err).FriendlyMessage; got != "image img-1 may be left behind: disk full; 3 tries" {
		t.Fatalf("FriendlyMessage = %q", got)
	}
}

func TestContextErrorKeepsGivenText(t *testing.T) {
	cause := quotaHTTPError()
	err := ContextError(cause, "srv-1: quota")
	if got := ParseError(err).FriendlyMessage; got != "srv-1: quota" {
		t.Fatalf("FriendlyMessage = %q", got)
	}
	var code gophercloud.ErrUnexpectedResponseCode
	if err.Error() != cause.Error() || !errors.As(err, &code) {
		t.Fatalf("Error() = %q, want the cause's text and chain", err.Error())
	}
}
