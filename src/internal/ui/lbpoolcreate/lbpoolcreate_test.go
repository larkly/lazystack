package lbpoolcreate

import "testing"

func poolForm(monType int, delay, timeout, codes string) Model {
	m := New(nil, "lb-1", "edge", nil)
	m.nameInput.SetValue("web")
	m.selectedMonType = monType
	m.monDelayInput.SetValue(delay)
	m.monTimeoutInput.SetValue(timeout)
	m.monCodesInput.SetValue(codes)
	return m
}

const (
	monNone = 0
	monHTTP = 1
	monTCP  = 3
)

// The pool form uses the same monitor timing rule as the monitor form, and
// checks it before the pool is created.
func TestPoolSubmitRejectsTimeoutNotBelowDelay(t *testing.T) {
	for _, tc := range []struct{ delay, timeout string }{{"3", "5"}, {"5", "5"}, {"", "9"}} {
		m, cmd := poolForm(monTCP, tc.delay, tc.timeout, "").submit()
		if cmd != nil || m.err == "" {
			t.Errorf("delay=%q timeout=%q: cmd=%v err=%q, want inline error", tc.delay, tc.timeout, cmd != nil, m.err)
		}
	}
	if m, cmd := poolForm(monTCP, "5", "3", "").submit(); cmd == nil || m.err != "" {
		t.Errorf("delay=5 timeout=3 rejected: %q", m.err)
	}
	// Without a monitor the timing fields are irrelevant.
	if m, cmd := poolForm(monNone, "3", "5", "").submit(); cmd == nil || m.err != "" {
		t.Errorf("pool without monitor rejected: %q", m.err)
	}
}

// Malformed expected codes fail before the pool POST instead of after the
// pool already exists.
func TestPoolSubmitValidatesExpectedCodes(t *testing.T) {
	for _, codes := range []string{"200-299-300", "abc", "20", "200,"} {
		if m, cmd := poolForm(monHTTP, "5", "3", codes).submit(); cmd != nil || m.err == "" {
			t.Errorf("codes=%q accepted", codes)
		}
	}
	for _, codes := range []string{"", "200", "200,201", "200-299"} {
		if m, cmd := poolForm(monHTTP, "5", "3", codes).submit(); cmd == nil || m.err != "" {
			t.Errorf("codes=%q rejected: %q", codes, m.err)
		}
	}
	// Non-HTTP monitors ignore the codes field entirely.
	if m, cmd := poolForm(monTCP, "5", "3", "garbage").submit(); cmd == nil || m.err != "" {
		t.Errorf("TCP monitor rejected because of HTTP-only field: %q", m.err)
	}
}
