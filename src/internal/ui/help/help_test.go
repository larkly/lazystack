package help

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
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
	// Number keys 1-9 and 0 switch tabs; there can be up to ten resource tabs.
	g := profiled(t, "serverlist")["Global"]
	e, ok := find(g, "switch tab")
	if !ok {
		t.Fatal("Global section has no tab switching entry")
	}
	if !strings.Contains(e.keys, "1-9, 0") {
		t.Errorf("tab switch keys = %q, want 1-9, 0", e.keys)
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
		{"dnslist", "DNS", []entry{{"↑/k ↓/j", "zone"}, {bindingKeys(shared.Keys.Refresh), "refresh"}, {"esc", "clear filter"}}},
		{"usermanagement", "User Management", []entry{{bindingKeys(shared.Keys.Enter), "enable/disable"}, {bindingKeys(shared.Keys.Delete), "delete user"}, {"esc", "back"}}},
		{"auditlog", "Audit Trail", []entry{{bindingKeys(shared.Keys.AuditFilterAction), "action"}, {bindingKeys(shared.Keys.AuditFilterResource), "resource"}, {bindingKeys(shared.Keys.AuditFilterDate), "date"}, {bindingKeys(shared.Keys.AuditFilterClear), "clear"}, {"esc", "back"}}},
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
		{bindingKeys(shared.Keys.Deactivate), "(de)activate"},
		{bindingKeys(shared.Keys.Download), "download"},
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
			if len(b.bindings) > 0 {
				continue
			}
			k := strings.ToLower(b.key)
			if strings.Contains(k, "ctrl+a") || strings.Contains(k, "ctrl+b") {
				t.Errorf("%s: literal help key %q uses a reserved key", s.name, b.key)
			}
		}
	}
}

// TestListViewsDocumentFilterAndSort checks that every list view with a "/"
// filter (and s/S sort) says so in its profiled help, along with what esc
// clears there.
func TestListViewsDocumentFilterAndSort(t *testing.T) {
	tests := []struct {
		view, section string
		sort          bool
	}{
		{"volumelist", "Volume List", true},
		{"floatingiplist", "Floating IPs", true},
		{"keypairlist", "Key Pairs", true},
		{"secgroupview", "Security Groups", true},
		{"networkview", "Networks", true},
		{"routerview", "Routers", true},
		{"dnslist", "DNS", true},
		{"lbview", "LB List", true},
		{"imageview", "Images", true},
		{"serverlist", "Server List", true},
	}
	for _, tc := range tests {
		t.Run(tc.view, func(t *testing.T) {
			entries := profiled(t, tc.view)[tc.section]
			if e, ok := find(entries, "filter"); !ok || e.keys != bindingKeys(shared.Keys.Filter) {
				t.Errorf("%s: filter entry = %+v (found %v), want keys %q", tc.section, e, ok, bindingKeys(shared.Keys.Filter))
			}
			if e, ok := find(entries, "clear filter"); !ok || e.keys != "esc" {
				t.Errorf("%s: no esc entry clearing the filter", tc.section)
			}
			if tc.sort {
				if e, ok := find(entries, "sort"); !ok || e.keys != "s/S" {
					t.Errorf("%s: no s/S sort entry", tc.section)
				}
			}
		})
	}
}

// navigationOnly are the KeyMap fields help shows as arrow shorthand
// ("↑/k ↓/j", "←/→ h/l", "pgup/pgdn") rather than through the binding.
var navigationOnly = map[string]bool{
	"Up": true, "Down": true, "Left": true, "Right": true,
	"PageUp": true, "PageDown": true,
}

// TestEveryBindingIsDocumented guards that each rebindable key in
// shared.Keys appears in some help section, so a binding added to the key
// map cannot silently go missing from the user's key reference.
func TestEveryBindingIsDocumented(t *testing.T) {
	documented := map[*key.Binding]bool{}
	for _, s := range allSections {
		for _, b := range s.binds {
			for _, kb := range b.bindings {
				documented[kb] = true
			}
		}
	}
	v := reflect.ValueOf(&shared.Keys).Elem()
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if navigationOnly[name] {
			continue
		}
		b, ok := v.Field(i).Addr().Interface().(*key.Binding)
		if !ok {
			continue
		}
		if !documented[b] {
			t.Errorf("shared.Keys.%s appears in no help section", name)
		}
	}
}

// TestListViewSortFollowsRebinding checks the sort entries are built from
// the Sort and ReverseSort bindings rather than a fixed "s/S".
func TestListViewSortFollowsRebinding(t *testing.T) {
	prevS, prevR := shared.Keys.Sort, shared.Keys.ReverseSort
	defer func() { shared.Keys.Sort, shared.Keys.ReverseSort = prevS, prevR }()
	shared.Keys.Sort = key.NewBinding(key.WithKeys("o"))
	shared.Keys.ReverseSort = key.NewBinding(key.WithKeys("O"))

	for view, section := range map[string]string{"volumelist": "Volume List", "networkview": "Networks"} {
		e, ok := find(profiled(t, view)[section], "sort")
		if !ok || e.keys != "o/O" {
			t.Errorf("%s sort entry = %+v, want keys o/O", section, e)
		}
	}
}

// TestEntriesFitOnOneLine guards that no help entry wraps inside the box:
// a wrapped description lands under the key column and misreads.
func TestEntriesFitOnOneLine(t *testing.T) {
	if inner := helpWidth - shared.StyleModal.GetHorizontalFrameSize(); inner != entryWidth {
		t.Fatalf("entryWidth = %d, but the box holds %d columns", entryWidth, inner)
	}
	for _, s := range allSections {
		for _, b := range s.binds {
			line := fmt.Sprintf("  %-14s %s", b.keys(), b.desc)
			if w := lipgloss.Width(line); w > entryWidth {
				t.Errorf("%s: %q is %d columns, wider than %d", s.name, line, w, entryWidth)
			}
		}
	}
}
