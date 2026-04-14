package shared

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"']+`)

// Network error patterns to detect connectivity issues.
var networkPatterns = []string{"timeout", "connection refused", "dial", "network"}

// ParsedError carries both a user-friendly message and the raw error for expandable display.
type ParsedError struct {
	FriendlyMessage string // User-friendly, actionable message
	RawError        string // Original error text for debug/details
	HTTPStatusCode  int    // HTTP status code (0 if unknown)
	Category        string // Category: "conflict", "forbidden", "not_found", "bad_request", "quota", "network", "unknown"
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

	// Check HTTP status codes first.
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		return &ParsedError{
			FriendlyMessage: "Resource is busy (provisioning in progress). Try again shortly.",
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
			FriendlyMessage: "Invalid request — check your input values and try again.",
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

	lowerRaw := strings.ToLower(raw)

	// Check for quota-related patterns in raw error if status code wasn't enough.
	if strings.Contains(lowerRaw, "quota exceeded") || strings.Contains(lowerRaw, "limit exceeded") {
		return &ParsedError{
			FriendlyMessage: "Quota exceeded — free up resources or contact your cloud admin.",
			RawError:        urlPattern.ReplaceAllString(raw, "[endpoint]"),
			HTTPStatusCode:  0,
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
