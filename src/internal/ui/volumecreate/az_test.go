package volumecreate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/testutil"
)

// fakeCinder serves volume types, availability zones and volume creation.
type fakeCinder struct {
	mu        sync.Mutex
	azStatus  int
	azBody    string
	creates   []map[string]any
	azQueries int
}

const cinderZones = `{"availabilityZoneInfo":[
	{"zoneName":"eu-north-1a","zoneState":{"available":true}},
	{"zoneName":"eu-north-1b","zoneState":{"available":true}},
	{"zoneName":"retired-zone","zoneState":{"available":false}}]}`

func (c *fakeCinder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/types":
		fmt.Fprint(w, `{"volume_types":[{"id":"11111111-2222","name":"ssd"}]}`)
	case r.Method == http.MethodGet && r.URL.Path == "/os-availability-zone":
		c.azQueries++
		if c.azStatus != 0 {
			http.Error(w, "zones unavailable", c.azStatus)
			return
		}
		fmt.Fprint(w, c.azBody)
	case r.Method == http.MethodPost && r.URL.Path == "/volumes":
		var body struct {
			Volume map[string]any `json:"volume"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		c.creates = append(c.creates, body.Volume)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"volume":{"id":"vol-1","name":"data","status":"creating","size":1}}`)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

// initModel runs the form's initial fetches (everything but the spinner tick)
// and feeds the results back into the model.
func initModel(t *testing.T, c *fakeCinder) Model {
	t.Helper()
	client, cleanup := testutil.FakeServiceClient(c)
	t.Cleanup(cleanup)
	m := New(client)
	m.SetSize(100, 40)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init did not return a batch")
	}
	for _, cmd := range batch[1:] {
		m, _ = m.Update(cmd())
	}
	return m
}

func press(m Model, keys ...tea.KeyPressMsg) Model {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

var (
	keyTab   = tea.KeyPressMsg{Code: tea.KeyTab}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keySave  = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
)

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func submitForm(t *testing.T, c *fakeCinder, m Model) map[string]any {
	t.Helper()
	m.nameInput.SetValue("data")
	m.sizeInput.SetValue("1")
	m, cmd := m.Update(keySave)
	if cmd == nil {
		t.Fatalf("submit rejected: %s", m.err)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("submit did not return a batch")
	}
	batch[len(batch)-1]()
	if len(c.creates) != 1 {
		t.Fatalf("creates = %d, want 1", len(c.creates))
	}
	return c.creates[0]
}

// #315: the AZ picker lists the cloud's zones and the chosen zone is sent.
func TestReviewSelectedAZ(t *testing.T) {
	c := &fakeCinder{azBody: cinderZones}
	m := initModel(t, c)
	if c.azQueries != 1 {
		t.Fatalf("availability zone queries = %d, want 1", c.azQueries)
	}

	// name -> size -> type -> AZ, then open the picker.
	m = press(m, keyTab, keyTab, keyTab, keyEnter)
	view := m.View()
	for _, fake := range []string{"az1", "az2", "nova"} {
		if strings.Contains(view, fake) {
			t.Fatalf("picker shows hard-coded zone %q:\n%s", fake, view)
		}
	}
	if !strings.Contains(view, "eu-north-1a") || !strings.Contains(view, "eu-north-1b") {
		t.Fatalf("picker lacks the cloud's zones:\n%s", view)
	}
	if strings.Contains(view, "retired-zone") {
		t.Fatalf("picker offers an unavailable zone:\n%s", view)
	}

	m = typeText(m, "1b")
	m = press(m, keyEnter)
	if !strings.Contains(m.View(), "eu-north-1b") {
		t.Fatalf("selected zone not displayed:\n%s", m.View())
	}

	body := submitForm(t, c, m)
	if body["availability_zone"] != "eu-north-1b" {
		t.Fatalf("POST availability_zone = %v, want eu-north-1b (body %v)", body["availability_zone"], body)
	}
}

// #315: leaving the zone unset keeps Cinder's default placement.
func TestUnsetAZUsesServerDefault(t *testing.T) {
	c := &fakeCinder{azBody: cinderZones}
	m := initModel(t, c)
	body := submitForm(t, c, m)
	if _, ok := body["availability_zone"]; ok {
		t.Fatalf("availability_zone sent without a selection: %v", body)
	}
}

// #315: choosing the default entry after a zone clears the selection.
func TestAZPickerCanReturnToDefault(t *testing.T) {
	c := &fakeCinder{azBody: cinderZones}
	m := initModel(t, c)
	m = press(m, keyTab, keyTab, keyTab, keyEnter)
	m = typeText(m, "1a")
	m = press(m, keyEnter)
	// Back to the AZ field and pick the first entry (server default).
	m = press(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, keyEnter, keyEnter)
	body := submitForm(t, c, m)
	if _, ok := body["availability_zone"]; ok {
		t.Fatalf("availability_zone still sent after choosing the default: %v", body)
	}
}

// #315: a failed zone lookup and an empty zone list are shown as such and
// do not block creating a volume in the default zone.
func TestAZFetchErrorAndEmptyStates(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    *fakeCinder
		want string
	}{
		{"error", &fakeCinder{azStatus: http.StatusForbidden}, "unavailable"},
		{"empty", &fakeCinder{azBody: `{"availabilityZoneInfo":[]}`}, "no zones"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := initModel(t, tc.c)
			if m.loading != 0 {
				t.Fatalf("loading = %d after fetches, want 0", m.loading)
			}
			if m.err != "" {
				t.Fatalf("zone lookup problem blocks the form: %q", m.err)
			}
			if !strings.Contains(m.View(), tc.want) {
				t.Fatalf("view lacks %q state:\n%s", tc.want, m.View())
			}
			body := submitForm(t, tc.c, m)
			if _, ok := body["availability_zone"]; ok {
				t.Fatalf("availability_zone sent: %v", body)
			}
		})
	}
}
