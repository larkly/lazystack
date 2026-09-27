package fippicker

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// fakeFIPNeutron serves a server with several ports, routers with and
// without gateways, one unassociated floating IP on ext-1 and records
// floating IP mutations.
type fakeFIPNeutron struct {
	mu        sync.Mutex
	ports     []map[string]any
	fips      string
	failPut   bool
	puts      []string
	creates   int
	deletes   int
	extNets   string
	routers   string
	portsErr  bool
	lastAlloc string
}

func (f *fakeFIPNeutron) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/floatingips"):
			w.Write([]byte(f.fips))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/floatingips/"):
			raw, _ := io.ReadAll(r.Body)
			f.puts = append(f.puts, string(raw))
			if f.failPut {
				http.Error(w, `{"NeutronError":{"message":"External network ext-1 is not reachable from subnet"}}`, http.StatusNotFound)
				return
			}
			w.Write([]byte(`{"floatingip":{"id":"fip-1"}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/floatingips"):
			f.creates++
			raw, _ := io.ReadAll(r.Body)
			f.lastAlloc = string(raw)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"floatingip":{"id":"fip-new","floating_ip_address":"198.51.100.7","floating_network_id":"ext-1"}}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/floatingips/"):
			f.deletes++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/routers"):
			w.Write([]byte(f.routers))
		case strings.HasSuffix(r.URL.Path, "/networks"):
			w.Write([]byte(f.extNets))
		case strings.HasSuffix(r.URL.Path, "/ports"):
			if f.portsErr {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			var out []map[string]any
			for _, p := range f.ports {
				if v := q.Get("device_id"); v != "" && p["device_id"] != v {
					continue
				}
				out = append(out, p)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ports": out})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func port(id, name, device, owner, network, subnet, ip string) map[string]any {
	return map[string]any{"id": id, "name": name, "device_id": device, "device_owner": owner, "network_id": network,
		"fixed_ips": []map[string]string{{"subnet_id": subnet, "ip_address": ip}}}
}

// newFakeFIPNeutron returns a server whose first-listed port is on an
// isolated network; only the second is routed to ext-1.
func newFakeFIPNeutron() *fakeFIPNeutron {
	return &fakeFIPNeutron{
		ports: []map[string]any{
			port("port-isolated", "storage", "srv", "compute:nova", "net-iso", "sub-iso", "192.168.50.4"),
			port("port-routed", "public", "srv", "compute:nova", "net-a", "sub-a", "10.0.0.5"),
			port("r1-a", "", "r1", "network:router_interface", "net-a", "sub-a", "10.0.0.1"),
		},
		fips:    `{"floatingips":[{"id":"fip-1","floating_ip_address":"203.0.113.9","floating_network_id":"ext-1"}]}`,
		routers: `{"routers":[{"id":"r1","external_gateway_info":{"network_id":"ext-1"}}]}`,
		extNets: `{"networks":[{"id":"ext-1","name":"public"}]}`,
	}
}

// drive executes cmd and feeds every resulting message (except spinner
// ticks) back into the model until no work remains.
func drive(t *testing.T, m Model, cmd tea.Cmd) (Model, []tea.Msg) {
	t.Helper()
	var emitted []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		switch msg := msg.(type) {
		case nil, spinner.TickMsg:
			continue
		case tea.BatchMsg:
			queue = append(queue, msg...)
			continue
		case shared.ResourceActionMsg, shared.ResourceActionErrMsg:
			emitted = append(emitted, msg)
			continue
		}
		var next tea.Cmd
		m, next = m.Update(msg)
		queue = append(queue, next)
	}
	return m, emitted
}

func TestAssociateUsesRoutedPortNotFirstPort(t *testing.T) {
	f := newFakeFIPNeutron()
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "srv", "web")
	m.SetSize(80, 30)
	m, _ = drive(t, m, m.Init())
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, emitted := drive(t, m, cmd)

	if len(f.puts) != 1 {
		t.Fatalf("association PUTs = %q, want exactly one", f.puts)
	}
	if !strings.Contains(f.puts[0], `"port_id":"port-routed"`) || !strings.Contains(f.puts[0], `"fixed_ip_address":"10.0.0.5"`) {
		t.Fatalf("associated wrong port: %s", f.puts[0])
	}
	if m.Active || len(emitted) != 1 {
		t.Fatalf("expected completion, active=%v emitted=%v", m.Active, emitted)
	}
}

func TestMultiNetworkServerOffersLabelledEligiblePorts(t *testing.T) {
	f := newFakeFIPNeutron()
	f.ports = append(f.ports,
		port("port-second", "", "srv", "compute:nova", "net-b", "sub-b", "10.1.0.8"),
		port("r1-b", "", "r1", "network:router_interface", "net-b", "sub-b", "10.1.0.1"),
	)
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "srv", "web")
	m.SetSize(100, 30)
	m, _ = drive(t, m, m.Init())
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = drive(t, m, cmd)

	if len(f.puts) != 0 {
		t.Fatalf("associated without asking: %q", f.puts)
	}
	view := m.View()
	for _, want := range []string{"10.0.0.5", "public", "10.1.0.8"} {
		if !strings.Contains(view, want) {
			t.Fatalf("port chooser missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "192.168.50.4") {
		t.Fatalf("unroutable port offered while routed ones exist:\n%s", view)
	}
	// Pick the entry for port-second and confirm it reaches the API.
	for i, tg := range m.portChoices {
		if tg.PortID == "port-second" {
			for range i {
				m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
		}
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, emitted := drive(t, m, cmd)
	if len(f.puts) != 1 || !strings.Contains(f.puts[0], `"port_id":"port-second"`) || !strings.Contains(f.puts[0], `"fixed_ip_address":"10.1.0.8"`) {
		t.Fatalf("chosen port not used: %q", f.puts)
	}
	if m.Active || len(emitted) != 1 {
		t.Fatalf("expected completion, active=%v emitted=%v", m.Active, emitted)
	}
}

func TestNoIPv4PortOrNeutronErrorIsNotSuccess(t *testing.T) {
	t.Run("no IPv4 port", func(t *testing.T) {
		f := newFakeFIPNeutron()
		f.ports = []map[string]any{port("p6", "", "srv", "compute:nova", "net-6", "sub-6", "2001:db8::5")}
		client, cleanup := testutil.FakeServiceClient(f.handler(t))
		defer cleanup()
		m := New(client, "srv", "web")
		m.SetSize(80, 30)
		m, _ = drive(t, m, m.Init())
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m, emitted := drive(t, m, cmd)
		if len(f.puts) != 0 || len(emitted) != 0 || !m.Active || !strings.Contains(m.View(), "IPv4") {
			t.Fatalf("puts=%q emitted=%v view=%s", f.puts, emitted, m.View())
		}
	})
	t.Run("auto allocate without IPv4 port allocates nothing", func(t *testing.T) {
		f := newFakeFIPNeutron()
		f.fips = `{"floatingips":[]}`
		f.ports = []map[string]any{port("p6", "", "srv", "compute:nova", "net-6", "sub-6", "2001:db8::5")}
		client, cleanup := testutil.FakeServiceClient(f.handler(t))
		defer cleanup()
		m := New(client, "srv", "web")
		m.SetSize(80, 30)
		m, emitted := drive(t, m, m.Init())
		if f.creates != 0 || len(emitted) != 0 || !m.Active {
			t.Fatalf("creates=%d emitted=%v", f.creates, emitted)
		}
	})
	t.Run("neutron rejects association", func(t *testing.T) {
		f := newFakeFIPNeutron()
		f.failPut = true
		client, cleanup := testutil.FakeServiceClient(f.handler(t))
		defer cleanup()
		m := New(client, "srv", "web")
		m.SetSize(80, 30)
		m, _ = drive(t, m, m.Init())
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m, emitted := drive(t, m, cmd)
		if len(emitted) != 0 || !m.Active || !strings.Contains(m.View(), "Error") {
			t.Fatalf("emitted=%v view=%s", emitted, m.View())
		}
	})
	t.Run("rejected association releases the freshly allocated IP", func(t *testing.T) {
		f := newFakeFIPNeutron()
		f.fips = `{"floatingips":[]}`
		f.failPut = true
		client, cleanup := testutil.FakeServiceClient(f.handler(t))
		defer cleanup()
		m := New(client, "srv", "web")
		m.SetSize(80, 30)
		m, emitted := drive(t, m, m.Init())
		if f.creates != 1 || f.deletes != 1 || len(emitted) != 0 || !m.Active || !strings.Contains(m.View(), "Error") {
			t.Fatalf("creates=%d deletes=%d emitted=%v view=%s", f.creates, f.deletes, emitted, m.View())
		}
		if !strings.Contains(f.lastAlloc, `"floating_network_id":"ext-1"`) {
			t.Fatalf("allocated from wrong network: %s", f.lastAlloc)
		}
	})
}
