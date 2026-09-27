package dnslist

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
)

var navKeys = []tea.KeyPressMsg{
	{Code: tea.KeyPgDown},
	{Code: tea.KeyPgUp},
	{Code: tea.KeyUp},
	{Code: tea.KeyDown},
	{Code: tea.KeyPgDown},
	{Code: tea.KeyPgDown},
	{Code: tea.KeyUp},
}

func TestEmptyZoneListNavigationIsSafe(t *testing.T) {
	cases := map[string]func(Model) Model{
		"initial": func(m Model) Model { return m },
		"empty": func(m Model) Model {
			m, _ = m.Update(zonesLoadedMsg{})
			return m
		},
		"error": func(m Model) Model {
			m, _ = m.Update(zonesErrMsg{err: errors.New("dns unavailable")})
			return m
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := New(nil)
			m.SetSize(100, 20)
			m = setup(m)
			for _, k := range navKeys {
				var cmd tea.Cmd
				m, cmd = m.Update(k)
				if cmd != nil {
					t.Fatalf("key %q on empty list produced a command", k.String())
				}
				if m.cursor < 0 || m.scroll < 0 {
					t.Fatalf("key %q: cursor=%d scroll=%d", k.String(), m.cursor, m.scroll)
				}
				if m.selectedZone != nil {
					t.Fatalf("key %q selected a zone on an empty list", k.String())
				}
			}
			_ = m.View()
		})
	}
}

func TestZoneNavigationSelectsValidRows(t *testing.T) {
	var zs []zones.Zone
	for i := 0; i < 30; i++ {
		zs = append(zs, zones.Zone{ID: fmt.Sprintf("z%02d", i), Name: fmt.Sprintf("zone%02d.example.", i)})
	}
	m := New(nil)
	m.SetSize(100, 16) // 10 visible rows
	m, _ = m.Update(zonesLoadedMsg{zones: zs})
	visible := m.visibleRows()

	check := func(label string, wantCursor int) {
		t.Helper()
		if m.cursor != wantCursor {
			t.Fatalf("%s: cursor=%d want %d", label, m.cursor, wantCursor)
		}
		if m.selectedZone == nil || m.selectedZone.ID != zs[m.cursor].ID {
			t.Fatalf("%s: selected=%v want %s", label, m.selectedZone, zs[m.cursor].ID)
		}
		if m.scroll < 0 || m.cursor < m.scroll || m.cursor >= m.scroll+visible {
			t.Fatalf("%s: cursor %d not visible (scroll=%d visible=%d)", label, m.cursor, m.scroll, visible)
		}
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	check("pgdown", visible)
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	check("pgdown to end", len(zs)-1)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	check("pgup", len(zs)-1-visible)
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	check("pgup to start", 0)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	check("down", 1)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	check("up", 0)
}
