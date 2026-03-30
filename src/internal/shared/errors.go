package shared

import (
	"net/http"
	"regexp"

	"github.com/gophercloud/gophercloud/v2"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"']+`)

// SanitizeAPIError wraps raw gophercloud errors with user-friendly messages.
func SanitizeAPIError(err error) string {
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		return "Resource is busy (provisioning in progress). Try again shortly."
	}
	if gophercloud.ResponseCodeIs(err, http.StatusForbidden) {
		return "Permission denied. Check your project role."
	}
	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		return "Resource not found. It may have been deleted."
	}
	if gophercloud.ResponseCodeIs(err, http.StatusBadRequest) {
		return "Invalid request. Check your input values."
	}
	if gophercloud.ResponseCodeIs(err, http.StatusRequestEntityTooLarge) {
		return "Quota exceeded. Check your resource limits."
	}

	return urlPattern.ReplaceAllString(err.Error(), "[endpoint]")
}
