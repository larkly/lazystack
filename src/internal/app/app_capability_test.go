package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func connectedMsg(warning string) shared.CloudConnectedMsg {
	return shared.CloudConnectedMsg{
		ComputeClient: &gophercloud.ServiceClient{},
		ImageClient:   &gophercloud.ServiceClient{},
		NetworkClient: &gophercloud.ServiceClient{},
		Region:        "RegionOne",
		Warning:       warning,
	}
}

func TestCloudConnectedShowsCapabilityWarningOnce(t *testing.T) {
	const warning = "Nova microversion 2.88 (max supported: 2.88, requested: 2.100). Some features may be limited."
	m := newTestModel("dev", false)
	res, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 40})
	m = res.(Model)
	m.cloudName = "dev"

	res, _ = m.Update(connectedMsg(warning))
	m = res.(Model)
	if m.statusBar.StickyHint != warning {
		t.Fatalf("StickyHint = %q, want the capability warning", m.statusBar.StickyHint)
	}
	if !strings.Contains(m.statusBar.Render(), "Some features may be limited") {
		t.Error("warning is not rendered in the status bar")
	}
	if m.statusBar.Region != "RegionOne" {
		t.Errorf("Region = %q, want RegionOne", m.statusBar.Region)
	}

	// A key press dismisses it and refresh ticks must not bring it back.
	res, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = res.(Model)
	res, _ = m.Update(shared.TickMsg{})
	m = res.(Model)
	if m.statusBar.StickyHint == warning {
		t.Error("capability warning reappeared after refresh")
	}
}

func TestCloudConnectedWithoutWarningIsSilent(t *testing.T) {
	m := newTestModel("dev", false)
	m.cloudName = "dev"
	res, _ := m.Update(connectedMsg(""))
	m = res.(Model)
	if m.statusBar.StickyHint != "" {
		t.Errorf("StickyHint = %q, want none at full capability", m.statusBar.StickyHint)
	}
}
