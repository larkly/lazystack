package shared

import (
	"errors"
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

	if parsed.FriendlyMessage != "Resource is busy (provisioning in progress). Try again shortly." {
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
	err := errors.New("network unreachable")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
	}
}

func TestParseError_DialError(t *testing.T) {
	err := errors.New("unable to dial endpoint")
	parsed := ParseError(err)

	if parsed.FriendlyMessage != "Network error — check connectivity and try again." {
		t.Errorf("unexpected friendly message: %q", parsed.FriendlyMessage)
	}
	if parsed.Category != "network" {
		t.Errorf("expected category network, got %q", parsed.Category)
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
	expected := "Resource is busy (provisioning in progress). Try again shortly."
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
