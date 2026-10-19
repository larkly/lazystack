package quotaview

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/quota"
	"github.com/larkly/lazystack/internal/shared"
)

func openQuota(lines int) Model {
	m := New()
	m.Visible = true
	m.Width, m.Height = 80, 12
	for i := 0; i < lines; i++ {
		m.lines = append(m.lines, fmt.Sprintf("line %d", i))
	}
	return m
}

func TestQuotaOverlayClosesWithConfiguredBinding(t *testing.T) {
	prev := shared.Keys.Quota
	t.Cleanup(func() { shared.Keys.Quota = prev })
	shared.Keys.Quota = key.NewBinding(key.WithKeys("f8"), key.WithHelp("f8", "quotas"))

	m := openQuota(3)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	if !m.Visible {
		t.Fatal("old Q binding still closes the overlay after rebinding")
	}
	if r := m.Render(); !strings.Contains(r, "f8 or esc to close") || strings.Contains(r, "Q or esc") {
		t.Errorf("hint does not show the configured key: %q", r)
	}
	m.loading = true
	if r := m.Render(); !strings.Contains(r, "f8 or esc to close") {
		t.Errorf("loading hint does not show the configured key: %q", r)
	}
	m.loading = false

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF8})
	if m.Visible {
		t.Fatal("configured quota key did not close the overlay")
	}
}

func TestQuotaOverlayDefaultBindingEscAndScroll(t *testing.T) {
	m := openQuota(3)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	if m.Visible {
		t.Fatal("default Q did not close the overlay")
	}

	m = openQuota(3)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Visible {
		t.Fatal("esc did not close the overlay")
	}

	m = openQuota(40)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !m.Visible || m.scroll != 1 {
		t.Fatalf("down: visible=%v scroll=%d", m.Visible, m.scroll)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.scroll != 0 {
		t.Fatalf("up: scroll=%d", m.scroll)
	}
}

// A failed refetch (the overlay reopened after the cache expired) must keep
// the cached quotas on screen and show the error as one friendly line; the
// next good load clears it.
func TestFailedRefetchKeepsCachedQuotas(t *testing.T) {
	m := New()
	m.Width, m.Height = 100, 40
	m.Visible = true
	m, _ = m.Update(quotaLoadedMsg{compute: []quota.QuotaUsage{{Resource: "Instances", Used: 3, Limit: 10}}})

	apiErr := gophercloud.ErrUnexpectedResponseCode{Actual: 503}
	m, _ = m.Update(quotaLoadedMsg{err: apiErr})
	out := ansi.Strip(m.Render())
	// The overlay is narrow, so the line is the friendly message cut to fit.
	friendly := strings.TrimSpace(ansi.Strip(shared.LoadErrorLine(apiErr, contentWidth)))
	if !strings.Contains(friendly, "The cloud API is unavailable (HTTP 503)") {
		t.Fatalf("error line %q is not the friendly message", friendly)
	}
	if !strings.Contains(out, "Compute") || !strings.Contains(out, "Instances") {
		t.Fatalf("failed refetch wiped the cached quotas:\n%s", out)
	}
	if !strings.Contains(out, friendly) {
		t.Fatalf("overlay lacks the friendly error %q:\n%s", friendly, out)
	}

	m, _ = m.Update(quotaLoadedMsg{compute: []quota.QuotaUsage{{Resource: "Instances", Used: 4, Limit: 10}}})
	if out := ansi.Strip(m.Render()); strings.Contains(out, friendly) {
		t.Fatalf("successful load did not clear the error:\n%s", out)
	}
}
