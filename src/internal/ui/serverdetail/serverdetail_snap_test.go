package serverdetail

import (
	"testing"
	"time"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/larkly/lazystack/internal/compute"
)

func TestServerDetail_View_Loading(t *testing.T) {
	m := New(nil, nil, nil, "srv-1", 5*time.Second)
	m.width = 80
	m.height = 24
	m.loading = true

	got := m.View()
	snaps.MatchSnapshot(t, got)
}

func TestServerDetail_View_WithServer(t *testing.T) {
	m := New(nil, nil, nil, "srv-1", 5*time.Second)
	m.width = 120
	m.height = 30
	m.loading = false
	m.server = &compute.Server{
		ID:         "srv-1",
		Name:       "web-01",
		Status:     "ACTIVE",
		PowerState: "Running",
		IPv4:       []string{"10.0.0.1"},
		IPv6:       []string{"fd00::1"},
		FlavorName: "m1.small",
		ImageName:  "Ubuntu 22.04",
		KeyName:    "my-key",
		Created:    time.Now().Add(-24 * time.Hour),
	}
	m.focus = focusInfo
	m.consoleLines = []string{"line1", "line2"}
	m.actions = []compute.Action{{Action: "create", RequestID: "req-1", UserID: "u1", StartTime: time.Now().Add(-2 * time.Hour), Message: "created"}}

	got := m.View()
	snaps.MatchSnapshot(t, got)
}
