package serverdetail

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

var (
	keyUp   = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown = tea.KeyPressMsg{Code: tea.KeyDown}
	keyPgUp = tea.KeyPressMsg{Code: tea.KeyPgUp}
	keyPgDn = tea.KeyPressMsg{Code: tea.KeyPgDown}
	keyTab  = tea.KeyPressMsg{Code: tea.KeyTab}
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// bigServer returns a server with many networks, security groups and volumes.
func bigServer(volumes int) *compute.Server {
	s := &compute.Server{
		ID:         "srv-1",
		Name:       "web-01",
		Status:     "ACTIVE",
		PowerState: "Running",
		IPv4:       []string{"10.0.0.1"},
		IPv6:       []string{"2001:db8::1"},
		FlavorName: "m1.small",
		ImageName:  "Ubuntu 22.04",
		KeyName:    "my-key",
		SecGroups:  []string{"default", "web"},
		Networks:   map[string][]string{},
	}
	for i := 0; i < 20; i++ {
		s.Networks[fmt.Sprintf("net-%02d-with-a-rather-long-network-name-that-overflows", i)] = []string{
			fmt.Sprintf("10.%d.0.10", i), fmt.Sprintf("2001:db8:%x::10", i),
		}
	}
	for i := 0; i < volumes; i++ {
		s.VolAttach = append(s.VolAttach, compute.VolumeAttachment{ID: fmt.Sprintf("vol-%02d", i), Device: fmt.Sprintf("/dev/vd%c", 'b'+i%24)})
	}
	return s
}

func loadedModel(t *testing.T, w, h int, srv *compute.Server) Model {
	t.Helper()
	m := New(nil, nil, nil, "srv-1", 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = m.Update(serverDetailLoadedMsg{inst: m.inst, server: srv})
	var console []string
	for i := 1; i <= 100; i++ {
		console = append(console, fmt.Sprintf("LINE-%03d", i))
	}
	m, _ = m.Update(consoleLoadedMsg{inst: m.inst, output: strings.Join(console, "\n")})
	var actions []compute.Action
	for i := 0; i < 40; i++ {
		actions = append(actions, compute.Action{Action: fmt.Sprintf("act-%02d", i)})
	}
	m, _ = m.Update(actionsLoadedMsg{inst: m.inst, actions: actions})
	var ports []network.Port
	for i := 0; i < 15; i++ {
		ports = append(ports, network.Port{
			MACAddress: fmt.Sprintf("fa:16:3e:00:00:%02d", i),
			Status:     "ACTIVE",
			FixedIPs:   []network.FixedIP{{IPAddress: fmt.Sprintf("10.1.0.%d", i)}},
			NetworkID:  fmt.Sprintf("net-id-%02d", i),
		})
	}
	m, _ = m.Update(interfacesLoadedMsg{inst: m.inst, ports: ports})
	return m
}

var layoutSizes = []struct{ w, h int }{
	{120, 30}, {80, 20}, {100, 24}, {200, 50}, {70, 40}, {60, 30},
}

func assertFitsViewport(t *testing.T, m Model, w, h int) {
	t.Helper()
	view := m.View()
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	// The app gives the view height-1 rows (the status bar takes one).
	if len(lines) > h-1 {
		t.Fatalf("%dx%d: view has %d lines, viewport %d", w, h, len(lines), h-1)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			t.Fatalf("%dx%d: line %d width %d > %d: %q", w, h, i, lw, w, l)
		}
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "SSH") {
		t.Fatalf("%dx%d: action bar not on last line: %q", w, h, last)
	}
	if bottom := lines[len(lines)-2]; !strings.Contains(bottom, "╰") {
		t.Fatalf("%dx%d: pane bottom border missing above action bar: %q", w, h, bottom)
	}
}

func TestViewFitsViewport(t *testing.T) {
	for _, sz := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			m := loadedModel(t, sz.w, sz.h, bigServer(20))
			assertFitsViewport(t, m, sz.w, sz.h)

			resize := bigServer(20)
			resize.Status = "VERIFY_RESIZE"
			m, _ = m.Update(serverDetailLoadedMsg{inst: m.inst, server: resize})
			assertFitsViewport(t, m, sz.w, sz.h)
		})
	}
}

func focusPaneTo(m Model, p focusPane) Model {
	for m.focus != p {
		m, _ = m.Update(keyTab)
	}
	return m
}

func scrollToEnd(m Model) Model {
	for i := 0; i < 200; i++ {
		m, _ = m.Update(keyDown)
	}
	return m
}

func TestEachPaneScrollsToFinalLine(t *testing.T) {
	for _, sz := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			base := loadedModel(t, sz.w, sz.h, bigServer(20))
			lastNet := "net-19"
			cases := []struct {
				pane focusPane
				want string
			}{
				{focusInfo, lastNet},
				{focusConsole, "LINE-100"},
				{focusActions, "act-39"},
				{focusInterfaces, ""}, // final interface line, computed below
				{focusVolumes, "vol-19"},
			}
			for _, tc := range cases {
				m := scrollToEnd(focusPaneTo(base, tc.pane))
				if tc.pane == focusInterfaces {
					lines := m.interfaceLines(m.layout().interfaces.contentWidth())
					tc.want = strings.TrimSpace(ansi.Strip(lines[len(lines)-1]))
					if !strings.Contains(tc.want, "14") {
						t.Fatalf("unexpected final interface line %q", tc.want)
					}
				}
				if v := ansi.Strip(m.View()); !strings.Contains(v, tc.want) {
					t.Fatalf("pane %d: %q not visible after scrolling to the end:\n%s", tc.pane, tc.want, v)
				}
			}
			// G jumps the console to its last line.
			m := focusPaneTo(base, focusConsole)
			m.consoleScroll = 0
			m, _ = m.Update(runeKey('G'))
			if v := m.View(); !strings.Contains(v, "LINE-100") {
				t.Fatalf("G did not reach final console line:\n%s", v)
			}
		})
	}
}

func TestResizeClampsScrollOffsets(t *testing.T) {
	m := loadedModel(t, 120, 20, bigServer(20))
	for _, p := range []focusPane{focusInfo, focusConsole, focusActions, focusInterfaces, focusVolumes} {
		m = scrollToEnd(focusPaneTo(m, p))
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	if m.scroll > m.infoMaxScroll() || m.consoleScroll > m.consoleMaxScroll() ||
		m.actionsScroll > m.actionsMaxScroll() || m.interfacesScroll > m.interfacesMaxScroll() ||
		m.volumeScroll > m.volumeMaxScroll() {
		t.Fatalf("offsets not clamped: info=%d/%d console=%d/%d actions=%d/%d ifaces=%d/%d vols=%d/%d",
			m.scroll, m.infoMaxScroll(), m.consoleScroll, m.consoleMaxScroll(),
			m.actionsScroll, m.actionsMaxScroll(), m.interfacesScroll, m.interfacesMaxScroll(),
			m.volumeScroll, m.volumeMaxScroll())
	}
	m.SetSize(80, 20)
	if !strings.Contains(m.View(), "vol-19") {
		t.Fatal("selected volume not visible after SetSize shrink")
	}
}

func TestConsoleGGoesToTopWhenConsoleFocused(t *testing.T) {
	m := loadedModel(t, 120, 30, bigServer(2))
	m = focusPaneTo(m, focusConsole)
	if m.consoleScroll == 0 {
		t.Fatal("console should auto-scroll to the bottom on load")
	}
	m, cmd := m.Update(runeKey('g'))
	if cmd != nil {
		t.Fatalf("g on focused console emitted %#v", cmd())
	}
	if m.consoleScroll != 0 {
		t.Fatalf("consoleScroll=%d want 0", m.consoleScroll)
	}

	m = focusPaneTo(m, focusInfo)
	_, cmd = m.Update(runeKey('g'))
	if cmd == nil {
		t.Fatal("g outside the console must still jump to security groups")
	}
	if nav, ok := cmd().(shared.NavigateToResourceMsg); !ok || nav.Tab != "secgroups" {
		t.Fatalf("g outside the console emitted %#v", nav)
	}
}

func TestVolumeCursorStaysVisible(t *testing.T) {
	m := loadedModel(t, 120, 24, bigServer(20))
	m = focusPaneTo(m, focusVolumes)
	selected := func(m Model) string { return fmt.Sprintf("vol-%02d", m.volumeCursor) }
	visible := func(m Model) {
		t.Helper()
		if !strings.Contains(ansi.Strip(m.View()), "▸ "+selected(m)) {
			t.Fatalf("selected %s (scroll=%d) not visible:\n%s", selected(m), m.volumeScroll, m.View())
		}
	}
	for i := 0; i < 19; i++ {
		m, _ = m.Update(keyDown)
		visible(m)
	}
	if m.volumeCursor != 19 {
		t.Fatalf("cursor=%d", m.volumeCursor)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if nav, ok := cmd().(shared.NavigateToDetailMsg); !ok || nav.ID != "vol-19" {
		t.Fatalf("enter navigated to %#v", nav)
	}
	for _, k := range []tea.KeyPressMsg{keyPgUp, keyUp, keyPgUp, keyPgUp, keyPgDn, keyDown, keyPgDn, keyPgDn} {
		m, _ = m.Update(k)
		visible(m)
	}

	// Shrinking the attachment list reclamps cursor and scroll.
	m, _ = m.Update(serverDetailLoadedMsg{inst: m.inst, server: bigServer(3)})
	if m.volumeCursor != 2 || m.volumeScroll != 0 {
		t.Fatalf("after shrink cursor=%d scroll=%d", m.volumeCursor, m.volumeScroll)
	}
	visible(m)
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if nav := cmd().(shared.NavigateToDetailMsg); nav.ID != "vol-02" {
		t.Fatalf("enter after shrink navigated to %q", nav.ID)
	}
}
