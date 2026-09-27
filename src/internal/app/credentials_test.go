package app

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// Rescue admin passwords must only ever appear in the masked credentials
// modal after an explicit reveal: never in action labels, the status bar,
// the default render or the audit log.
func TestRescuePasswordOnlyInMaskedReveal(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprintf("bulk=%v", bulk), func(t *testing.T) {
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				id := strings.Split(r.URL.Path, "/")[2]
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"adminPass":"pw-%s-secret"}`, id)
			})
			action := confirmed("rescue", "alpha")
			if bulk {
				action.Servers = []modal.ServerRef{{ID: "alpha", Name: "alpha"}, {ID: "beta", Name: "beta"}}
			}
			res, cmd := m.Update(action)
			m, _ = deliver(t, res.(Model), cmd)

			if strings.Contains(m.statusBar.StickyHint, "secret") || !strings.Contains(m.statusBar.StickyHint, "✓") {
				t.Fatalf("status bar: %q", m.statusBar.StickyHint)
			}
			if !m.vmPassword.Active {
				t.Fatal("credentials modal not opened")
			}
			if strings.Contains(m.viewContent(), "secret") {
				t.Fatal("password rendered before explicit reveal")
			}
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) == 0 || strings.Contains(string(raw), "secret") {
				t.Fatalf("audit log leaks password or is empty: %s err=%v", raw, err)
			}

			res, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
			m = res.(Model)
			view := m.viewContent()
			names := []string{"alpha"}
			if bulk {
				names = append(names, "beta")
			}
			for _, name := range names {
				found := false
				for _, line := range strings.Split(view, "\n") {
					if strings.Contains(line, name) && strings.Contains(line, "pw-"+name+"-secret") {
						found = true
					}
				}
				if !found {
					t.Errorf("revealed view does not associate %s with its password:\n%s", name, view)
				}
			}
		})
	}
}
