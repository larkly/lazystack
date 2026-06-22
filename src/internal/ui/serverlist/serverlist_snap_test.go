package serverlist

import (
	"testing"
	"time"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/larkly/lazystack/internal/compute"
)

func TestServerList_View_Empty(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.width = 80
	m.height = 24
	m.loading = false
	m.servers = []compute.Server{}
	m.filtered = []compute.Server{}

	got := m.View()
	snaps.MatchSnapshot(t, got)
}

func TestServerList_View_Filtered(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.width = 80
	m.height = 24
	m.loading = false
	m.servers = []compute.Server{
		{ID: "s1", Name: "web-01", Status: "ACTIVE", PowerState: "Running", IPv4: []string{"10.0.0.1"}, IPv6: []string{"fd00::1"}, FloatingIP: []string{}, FlavorName: "m1.small", ImageName: "Ubuntu 22.04", KeyName: "my-key", Created: time.Now().Add(-24 * time.Hour)},
		{ID: "s2", Name: "db-01", Status: "SHUTOFF", PowerState: "Shutdown", IPv4: []string{"10.0.0.2"}, IPv6: []string{}, FloatingIP: []string{}, FlavorName: "m1.medium", ImageName: "Debian 12", KeyName: "", Created: time.Now().Add(-48 * time.Hour)},
	}
	m.filtered = m.servers
	m.columns = DefaultColumns()
	m.columns = ComputeWidths(m.columns, m.width, nil)
	m.scrollOff = 0
	m.cursor = 0

	got := m.View()
	snaps.MatchSnapshot(t, got)
}
