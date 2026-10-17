package app

import (
	"slices"
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/networkview"
	"github.com/larkly/lazystack/internal/ui/routerview"
)

// A successful mutation must refresh the view it affects immediately
// instead of waiting for the next periodic tick.
func TestResourceActionRefreshesAffectedView(t *testing.T) {
	cases := []struct {
		name      string
		view      activeView
		wantView  activeView
		wantPath  string
		wantLabel string
	}{
		{"router create", viewRouterView, viewRouterView, "/routers", ""},
		{"network delete", viewNetworkList, viewNetworkList, "/networks", ""},
		{"volume delete from detail", viewVolumeDetail, viewVolumeList, "/volumes/detail", "volumelist"},
		{"keypair delete from detail", viewKeypairDetail, viewKeypairList, "/os-keypairs", "keypairlist"},
		{"floating ip release", viewFloatingIPList, viewFloatingIPList, "/floatingips", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, requests := listFixture(t)
			m.routerView = routerview.New(m.client.Network, time.Hour)
			m.networkView = networkview.New(m.client.Network, time.Hour)
			m.keypairList = keypairlist.New(m.client.Compute, time.Hour)
			m.floatingIPList = floatingiplist.New(m.client.Network, time.Hour)
			m.view = tc.view
			res, cmd := m.Update(shared.ResourceActionMsg{Action: "Changed", Name: "thing"})
			got := res.(Model)
			if got.view != tc.wantView {
				t.Fatalf("view=%v want %v", got.view, tc.wantView)
			}
			if tc.wantLabel != "" && got.statusBar.CurrentView != tc.wantLabel {
				t.Errorf("status view=%q want %q", got.statusBar.CurrentView, tc.wantLabel)
			}
			if cmd == nil {
				t.Fatal("no refresh command after successful mutation")
			}
			before := len(*requests)
			commandMessages(cmd)
			if !slices.Contains((*requests)[before:], tc.wantPath) {
				t.Fatalf("refresh did not fetch %s: %v", tc.wantPath, (*requests)[before:])
			}
		})
	}
}

func TestActionResultMarkerFollowsPlainMode(t *testing.T) {
	prev := shared.PlainMode
	t.Cleanup(func() { shared.PlainMode = prev })
	for _, tc := range []struct {
		plain bool
		want  string
	}{{false, "✓ Deleted vol-1"}, {true, "OK Deleted vol-1"}} {
		m := initTestModel() // New applies the config's plain mode
		shared.PlainMode = tc.plain
		m, _ = m.handleResourceActionMsg(shared.ResourceActionMsg{Action: "Deleted", Name: "vol-1"})
		if m.statusBar.StickyHint != tc.want {
			t.Errorf("plain=%v: hint = %q, want %q", tc.plain, m.statusBar.StickyHint, tc.want)
		}
		m, _ = m.handleServerActionMsg(shared.ServerActionMsg{Action: "Deleted", Name: "vol-1"})
		if m.statusBar.StickyHint != tc.want {
			t.Errorf("plain=%v: server hint = %q, want %q", tc.plain, m.statusBar.StickyHint, tc.want)
		}
	}
}
