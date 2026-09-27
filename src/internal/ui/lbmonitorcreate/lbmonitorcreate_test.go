package lbmonitorcreate

import (
	"testing"

	"github.com/larkly/lazystack/internal/loadbalancer"
)

func submitWith(m Model, delay, timeout, codes string) (Model, bool) {
	m.delayInput.SetValue(delay)
	m.timeoutInput.SetValue(timeout)
	m.codesInput.SetValue(codes)
	m, cmd := m.submit()
	return m, cmd != nil
}

func forms() map[string]Model {
	return map[string]Model{
		"create": New(nil, "pool-1", "web"),
		"edit": NewEdit(nil, "hm-1", &loadbalancer.HealthMonitor{
			ID: "hm-1", Type: "HTTP", Delay: 10, Timeout: 5, MaxRetries: 3, URLPath: "/", ExpectedCodes: "200", HTTPMethod: "GET",
		}, "web"),
	}
}

// Octavia's API reference requires the timeout to be less than the delay.
func TestSubmitRejectsTimeoutNotBelowDelay(t *testing.T) {
	for name, form := range forms() {
		for _, tc := range []struct{ delay, timeout string }{
			{"3", "5"},
			{"5", "5"},
			{"", "9"}, // default delay 5
		} {
			m, sent := submitWith(form, tc.delay, tc.timeout, "")
			if sent || m.err == "" || m.submitting {
				t.Errorf("%s delay=%q timeout=%q: sent=%v err=%q, want inline error", name, tc.delay, tc.timeout, sent, m.err)
			}
		}
		m, sent := submitWith(form, "5", "3", "")
		if !sent || m.err != "" {
			t.Errorf("%s delay=5 timeout=3 rejected: %q", name, m.err)
		}
		m, sent = submitWith(form, "", "", "")
		if !sent || m.err != "" {
			t.Errorf("%s defaults rejected: %q", name, m.err)
		}
	}
}

func TestSubmitValidatesExpectedCodes(t *testing.T) {
	for name, form := range forms() {
		for _, codes := range []string{"200-299-300", "200,201-299", "200,", "abc", "20", "2000"} {
			if m, sent := submitWith(form, "5", "3", codes); sent || m.err == "" {
				t.Errorf("%s codes=%q accepted", name, codes)
			}
		}
		for _, codes := range []string{"200", "200,201", "200, 202", "200-299", ""} {
			if m, sent := submitWith(form, "5", "3", codes); !sent || m.err != "" {
				t.Errorf("%s codes=%q rejected: %q", name, codes, m.err)
			}
		}
	}
}
