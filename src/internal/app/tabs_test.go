package app

import (
	"net/http"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// tenTabs is the tab set of a cloud offering every optional service.
func tenTabs() []TabDef {
	return []TabDef{
		{Name: "Servers", Key: "servers"},
		{Name: "Volumes", Key: "volumes"},
		{Name: "Images", Key: "images"},
		{Name: "Floating IPs", Key: "floatingips"},
		{Name: "Sec Groups", Key: "secgroups"},
		{Name: "Networks", Key: "networks"},
		{Name: "Routers", Key: "routers"},
		{Name: "Load Balancers", Key: "loadbalancers"},
		{Name: "DNS", Key: "dns"},
		{Name: "Key Pairs", Key: "keypairs"},
	}
}

func TestZeroKeySwitchesToTenthTab(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})
	m.view = viewServerList
	m.tabs = tenTabs()
	m.tabInited = make([]bool, len(m.tabs))

	m, _ = typeKeys(t, m, "0")
	if m.activeTab != 9 || m.view != viewKeypairList {
		t.Fatalf("0 switched to tab %d (view %v), want the tenth tab, Key Pairs", m.activeTab, m.view)
	}
}

func TestTenTabBarFitsOneLine(t *testing.T) {
	m := initTestModel()
	m.width = 80
	m.tabs = tenTabs()
	bar := m.renderTabBar()
	if strings.Contains(bar, "\n") {
		t.Fatalf("tab bar wrapped: %q", bar)
	}
	if w := lipgloss.Width(bar); w > 80 {
		t.Fatalf("tab bar is %d cells wide, want at most 80", w)
	}
	if !strings.Contains(bar, " 1:Servers ") {
		t.Fatalf("tab bar lost its first tab: %q", bar)
	}

	// Wide enough for every tab, the tenth is labelled with its key.
	m.width = 200
	if bar := m.renderTabBar(); !strings.Contains(bar, " 0:Key Pairs ") {
		t.Fatalf("tenth tab not labelled with the 0 key: %q", bar)
	}
}
