package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"']+`)

// Network error patterns to detect connectivity issues.
// Patterns are deliberately connection-specific: a bare word like "network"
// also appears in resource errors (e.g. "Network abc123 not found") and must
// NOT trigger connectivity classification.
var networkPatterns = []string{
	"connection refused",
	"connection reset",
	"no route to host",
	"network is unreachable",
	"network is down",
	"no such host",
	"i/o timeout",
	"context deadline exceeded",
	"dial tcp",
	"tls handshake",
	"temporarily unavailable",
}

// quotaPatterns match the quota and over-limit wording of Nova, Neutron,
// Cinder and Octavia. They are checked before the status code because the
// services disagree on it: Nova answers 403, Neutron 409, Cinder 413.
var quotaPatterns = []string{
	"quota exceeded",
	"limit exceeded",
	"overquota",
	"exceeded for",
}

// httpStatus returns the HTTP status carried by err, or 0 when err is not
// an unexpected-response-code error.
func httpStatus(err error) int {
	var codeErr gophercloud.ErrUnexpectedResponseCode
	if errors.As(err, &codeErr) {
		return codeErr.Actual
	}
	return 0
}

// APIFaultMessage extracts the human-readable message from an OpenStack
// error response body. It understands the Nova/Cinder fault shape
// {"<fault>": {"message": ...}}, Neutron's {"NeutronError": {"message": ...}}
// and Octavia's {"faultstring": ...}. It returns "" when err carries no
// response body or the body has no message.
func APIFaultMessage(err error) string {
	var codeErr gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &codeErr) || len(codeErr.Body) == 0 {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(codeErr.Body, &fields) != nil {
		return ""
	}
	var faultstring string
	if json.Unmarshal(fields["faultstring"], &faultstring) == nil {
		if m := strings.TrimSpace(faultstring); m != "" {
			return m
		}
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var fault struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(fields[k], &fault) != nil {
			continue
		}
		if m := strings.TrimSpace(fault.Message); m != "" {
			return m
		}
	}
	return ""
}

// withAPIFault appends the API's own reason to a friendly message.
func withAPIFault(friendly string, err error) string {
	if m := APIFaultMessage(err); m != "" {
		return friendly + " (" + m + ")"
	}
	return friendly
}

// ParsedError carries both a user-friendly message and the raw error for expandable display.
type ParsedError struct {
	FriendlyMessage string // User-friendly, actionable message
	RawError        string // Original error text for debug/details
	HTTPStatusCode  int    // HTTP status code (0 if unknown)
	Category        string // Category: "conflict", "forbidden", "not_found", "bad_request", "quota", "unauthorized", "unavailable", "network", "unknown"
}

func (e *ParsedError) Error() string {
	return e.FriendlyMessage
}

// ParseError converts a raw error into a ParsedError with friendly messaging.
func ParseError(err error) *ParsedError {
	if err == nil {
		return &ParsedError{
			FriendlyMessage: "An unexpected error occurred.",
			RawError:        "nil error",
			HTTPStatusCode:  0,
			Category:        "unknown",
		}
	}

	raw := err.Error()
	lowerRaw := strings.ToLower(raw)

	// Quota text wins over the status code; see quotaPatterns.
	for _, pattern := range quotaPatterns {
		if strings.Contains(lowerRaw, pattern) {
			return &ParsedError{
				FriendlyMessage: "Quota exceeded — free up resources or contact your cloud admin.",
				RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
				HTTPStatusCode:  httpStatus(err),
				Category:        "quota",
			}
		}
	}

	// Then the HTTP status code.
	switch status := httpStatus(err); status {
	case http.StatusUnauthorized:
		return &ParsedError{
			FriendlyMessage: "Session expired or credentials rejected — press C to reconnect.",
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  status,
			Category:        "unauthorized",
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return &ParsedError{
			FriendlyMessage: fmt.Sprintf("The cloud API is unavailable (HTTP %d) — try again shortly.", status),
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  status,
			Category:        "unavailable",
		}
	}
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		return &ParsedError{
			FriendlyMessage: withAPIFault("Conflict — the resource is busy, in use, or the name already exists.", err),
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  http.StatusConflict,
			Category:        "conflict",
		}
	}
	if gophercloud.ResponseCodeIs(err, http.StatusForbidden) {
		return &ParsedError{
			FriendlyMessage: "Permission denied — check your role assignments in the cloud console.",
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  http.StatusForbidden,
			Category:        "forbidden",
		}
	}
	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		return &ParsedError{
			FriendlyMessage: "Resource not found — it may have been deleted by another user.",
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  http.StatusNotFound,
			Category:        "not_found",
		}
	}
	if gophercloud.ResponseCodeIs(err, http.StatusBadRequest) {
		return &ParsedError{
			FriendlyMessage: withAPIFault("Invalid request — check your input values and try again.", err),
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  http.StatusBadRequest,
			Category:        "bad_request",
		}
	}
	if gophercloud.ResponseCodeIs(err, http.StatusRequestEntityTooLarge) {
		return &ParsedError{
			FriendlyMessage: "Quota exceeded — free up resources or contact your cloud admin.",
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  http.StatusRequestEntityTooLarge,
			Category:        "quota",
		}
	}

	// Check for network-related error patterns.
	for _, pattern := range networkPatterns {
		if strings.Contains(lowerRaw, pattern) {
			return &ParsedError{
				FriendlyMessage: "Network error — check connectivity and try again.",
				RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
				HTTPStatusCode:  0,
				Category:        "network",
			}
		}
	}

	// Fallback: sanitized raw error.
	sanitized := urlPattern.ReplaceAllString(raw, "[endpoint]")
	return &ParsedError{
		FriendlyMessage: sanitized,
		RawError:        sanitized,
		HTTPStatusCode:  0,
		Category:        "unknown",
	}
}

// SanitizeAPIError wraps raw gophercloud errors with user-friendly messages.
// Deprecated: Use ParseError for structured error handling with categories.
func SanitizeAPIError(err error) string {
	if err == nil {
		return "An unexpected error occurred."
	}
	return ParseError(err).FriendlyMessage
}
