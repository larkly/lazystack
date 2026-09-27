package help

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/larkly/lazystack/internal/shared"
)

func TestHelpOverlay_Render(t *testing.T) {
	m := New()
	m.Visible = true
	m.View = "serverlist"
	m.Width = 80
	m.Height = 24
	m.Open("serverlist")

	got := m.Render()
	snaps.MatchSnapshot(t, got)
}

// entry is one parsed help line: its key column and description.
type entry struct{ keys, desc string }

var sepRe = regexp.MustCompile(`\s{2,}`)

// profiled opens the profiled (first-tier) help for view and returns the
// section names and entries shown, keyed by section.
func profiled(t *testing.T, view string) map[string][]entry {
	t.Helper()
	m := New()
	m.Open(view)
	out := map[string][]entry{}
	current := ""
	for _, line := range m.lines {
		plain := ansi.Strip(line)
		if plain == "" {
			continue
		}
		if !strings.HasPrefix(plain, "  ") {
			current = plain
			out[current] = nil
			continue
		}
		parts := sepRe.Split(strings.TrimSpace(plain), 2)
		if len(parts) != 2 {
			t.Fatalf("help line %q in %s has no key/description separator", plain, current)
		}
		out[current] = append(out[current], entry{parts[0], parts[1]})
	}
	return out
}

func find(entries []entry, desc string) (entry, bool) {
	for _, e := range entries {
		if strings.Contains(e.desc, desc) {
			return e, true
		}
	}
	return entry{}, false
}

func bindingKeys(b key.Binding) string { return strings.Join(b.Keys(), "/") }

func TestGlobalTabRangeCoversNumberKeys(t *testing.T) {
	// Number keys 1-9 switch tabs and there can be up to ten resource tabs.
	g := profiled(t, "serverlist")["Global"]
	e, ok := find(g, "switch tab")
	if !ok {
		t.Fatal("Global section has no tab switching entry")
	}
	if !strings.Contains(e.keys, "1-9") {
		t.Errorf("tab switch keys = %q, want 1-9", e.keys)
	}
}

func TestServerSectionsListRescue(t *testing.T) {
	for view, section := range map[string]string{"serverlist": "Server List", "serverdetail": "Server Detail"} {
		e, ok := find(profiled(t, view)[section], "rescue/unrescue")
		if !ok {
			t.Errorf("%s help has no rescue/unrescue entry", section)
			continue
		}
		if want := bindingKeys(shared.Keys.Rescue); e.keys != want {
			t.Errorf("%s rescue keys = %q, want %q", section, e.keys, want)
		}
	}
}

// TestNewerViewsHaveContextualHelp checks the views added after the original
// help layout get their own profiled section with the keys they handle.
func TestNewerViewsHaveContextualHelp(t *testing.T) {
	tests := []struct {
		view    string
		section string
		want    []entry // keys + description fragment
	}{
		{"hypervisorlist", "Hypervisors", []entry{{"↑/k ↓/j", "navigate"}, {bindingKeys(shared.Keys.Refresh), "refresh"}, {"esc", "back"}}},
		{"servicecatalog", "Service Catalog", []entry{{"↑/k ↓/j", "navigate"}, {"esc", "back"}}},
		{"dnslist", "DNS", []entry{{"↑/k ↓/j", "zone"}, {bindingKeys(shared.Keys.Refresh), "refresh"}, {"esc", "back"}}},
		{"usermanagement", "User Management", []entry{{"enter", "enable/disable"}, {"d", "delete user"}, {"esc", "back"}}},
		{"auditlog", "Audit Trail", []entry{{"a", "action"}, {"r", "resource"}, {"d", "date"}, {"c", "clear"}, {"esc", "back"}}},
	}
	for _, tc := range tests {
		t.Run(tc.view, func(t *testing.T) {
			secs := profiled(t, tc.view)
			entries, ok := secs[tc.section]
			if !ok {
				t.Fatalf("profiled help for %s shows sections %v, want %q", tc.view, keysOf(secs), tc.section)
			}
			for _, w := range tc.want {
				e, ok := find(entries, w.desc)
				if !ok {
					t.Errorf("%s: no entry describing %q", tc.section, w.desc)
					continue
				}
				if e.keys != w.keys {
					t.Errorf("%s %q keys = %q, want %q", tc.section, w.desc, e.keys, w.keys)
				}
			}
		})
	}
}

func keysOf(m map[string][]entry) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestServerBindingsMatchKeyMap checks shared-binding entries show the keys
// that are actually bound, so help follows config rebinding.
func TestServerBindingsMatchKeyMap(t *testing.T) {
	checks := []struct {
		desc string
		b    key.Binding
	}{
		{"attach volume", shared.Keys.Attach},
		{"assign floating IP", shared.Keys.AssignFIP},
		{"hard reboot", shared.Keys.HardReboot},
		{"lock/unlock", shared.Keys.Lock},
		{"rebuild", shared.Keys.Rebuild},
		{"snapshot", shared.Keys.Snapshot},
		{"admin password", shared.Keys.GetPassword},
		{"admin actions", shared.Keys.AdminActions},
		{"metadata", shared.Keys.Metadata},
		{"user management", shared.Keys.UserManagement},
		{"audit trail", shared.Keys.AuditLog},
	}
	for _, view := range []string{"serverlist", "serverdetail"} {
		secs := profiled(t, view)
		section := map[string]string{"serverlist": "Server List", "serverdetail": "Server Detail"}[view]
		for _, c := range checks {
			e, ok := find(secs[section], c.desc)
			if !ok {
				t.Errorf("%s: no %q entry", section, c.desc)
				continue
			}
			if want := bindingKeys(c.b); e.keys != want {
				t.Errorf("%s %q keys = %q, want %q", section, c.desc, e.keys, want)
			}
		}
	}
	list := profiled(t, "serverlist")["Server List"]
	for _, c := range []struct {
		desc string
		b    key.Binding
	}{{"save", shared.Keys.SaveFilter}, {"load", shared.Keys.LoadFilter}, {"columns", shared.Keys.ColumnPick}} {
		e, ok := find(list, c.desc)
		if !ok || e.keys != bindingKeys(c.b) {
			t.Errorf("Server List %q = %+v, want keys %q", c.desc, e, bindingKeys(c.b))
		}
	}
	g := profiled(t, "serverlist")["Global"]
	for _, c := range []struct {
		desc string
		b    key.Binding
	}{{"hypervisors", shared.Keys.Hypervisors}, {"service catalog", shared.Keys.Browse}, {"configuration", shared.Keys.Config}} {
		e, ok := find(g, c.desc)
		if !ok || e.keys != bindingKeys(c.b) {
			t.Errorf("Global %q = %+v, want keys %q", c.desc, e, bindingKeys(c.b))
		}
	}
}

func TestImageHelpMatchesImageViewRoutes(t *testing.T) {
	img := profiled(t, "imageview")["Images"]
	for _, c := range []struct{ keys, desc string }{
		{bindingKeys(shared.Keys.Deactivate), "deactivate/reactivate"},
		{"ctrl+g", "download"}, // app routes the literal ctrl+g in the properties pane
		{"enter", "edit image"},
		{bindingKeys(shared.Keys.Create), "upload"},
	} {
		e, ok := find(img, c.desc)
		if !ok || e.keys != c.keys {
			t.Errorf("Images %q = %+v, want keys %q", c.desc, e, c.keys)
		}
	}
}

func TestReboundKeyIsShown(t *testing.T) {
	prev := shared.Keys.Rescue
	defer func() { shared.Keys.Rescue = prev }()
	shared.Keys.Rescue = key.NewBinding(key.WithKeys("ctrl+q"))

	e, ok := find(profiled(t, "serverlist")["Server List"], "rescue/unrescue")
	if !ok || e.keys != "ctrl+q" {
		t.Errorf("rescue entry = %+v, want rebound key ctrl+q", e)
	}
}

func TestEveryProfiledSectionExists(t *testing.T) {
	names := map[string]bool{}
	for _, s := range allSections {
		names[s.name] = true
		if len(s.binds) == 0 {
			t.Errorf("section %q has no entries", s.name)
		}
	}
	for view, secs := range viewSections {
		for _, s := range secs {
			if !names[s] {
				t.Errorf("view %s maps to unknown section %q", view, s)
			}
		}
	}
}

// TestNoLiteralReservedKeys guards the rule that Ctrl+A (Screen) and Ctrl+B
// (tmux) are never documented as lazystack bindings in fixed help text.
func TestNoLiteralReservedKeys(t *testing.T) {
	for _, s := range allSections {
		for _, b := range s.binds {
			if b.binding != nil {
				continue
			}
			k := strings.ToLower(b.key)
			if strings.Contains(k, "ctrl+a") || strings.Contains(k, "ctrl+b") {
				t.Errorf("%s: literal help key %q uses a reserved key", s.name, b.key)
			}
		}
	}
}
