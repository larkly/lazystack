package loadbalancer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Health monitor form defaults, shared by the pool and monitor forms.
const (
	DefaultMonitorDelay         = 5
	DefaultMonitorTimeout       = 3
	DefaultMonitorRetries       = 3
	DefaultMonitorExpectedCodes = "200"
)

// ValidationError is a user-facing form validation message.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// expectedCodesRe is Octavia's expected_codes pattern: a single code, a
// comma-separated list, or one range.
var expectedCodesRe = regexp.MustCompile(`^(\d{3}(\s*,\s*\d{3})*)$|^(\d{3}-\d{3})$`)

// MonitorTiming holds validated health monitor timing values.
type MonitorTiming struct {
	Delay, Timeout, MaxRetries int
}

// ParseMonitorTiming parses the delay, timeout and max-retries form values
// (blank means the default). Octavia's API reference states that the
// timeout must be less than the delay, so equal values are rejected too.
func ParseMonitorTiming(delayStr, timeoutStr, retriesStr string) (MonitorTiming, error) {
	delay, err := parseDefault(delayStr, DefaultMonitorDelay)
	if err != nil || delay < 1 {
		return MonitorTiming{}, ValidationError("Delay must be a positive number (seconds)")
	}
	timeout, err := parseDefault(timeoutStr, DefaultMonitorTimeout)
	if err != nil || timeout < 1 {
		return MonitorTiming{}, ValidationError("Timeout must be a positive number (seconds)")
	}
	if timeout >= delay {
		return MonitorTiming{}, ValidationError(fmt.Sprintf("Timeout (%ds) must be less than the delay (%ds)", timeout, delay))
	}
	retries, err := parseDefault(retriesStr, DefaultMonitorRetries)
	if err != nil || retries < 1 || retries > 10 {
		return MonitorTiming{}, ValidationError("Max retries must be a number between 1 and 10")
	}
	return MonitorTiming{Delay: delay, Timeout: timeout, MaxRetries: retries}, nil
}

// NormalizeExpectedCodes validates an HTTP(S) monitor's expected codes and
// returns the value to send; blank means DefaultMonitorExpectedCodes.
func NormalizeExpectedCodes(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultMonitorExpectedCodes, nil
	}
	if !expectedCodesRe.MatchString(s) {
		return "", ValidationError("Expected codes: single (200), list (200,201), or range (200-299)")
	}
	return s, nil
}

func parseDefault(s string, def int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	return strconv.Atoi(s)
}
