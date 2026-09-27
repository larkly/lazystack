package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/auditlog"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/dnslist"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/hypervisorlist"
	"github.com/larkly/lazystack/internal/ui/imageview"
	"github.com/larkly/lazystack/internal/ui/keypaircreate"
	"github.com/larkly/lazystack/internal/ui/keypairdetail"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/lbview"
	"github.com/larkly/lazystack/internal/ui/networkview"
	"github.com/larkly/lazystack/internal/ui/routerview"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/servicecatalog"
	"github.com/larkly/lazystack/internal/ui/usermanagement"
	"github.com/larkly/lazystack/internal/ui/volumecreate"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
	"github.com/larkly/lazystack/internal/ui/volumelist"
)

// renderModel returns a model with every view constructed (no clients) so
// each can render its own header.
func renderModel() Model {
	m := initTestModel()
	m.serverList = serverlist.New(nil, nil, time.Hour)
	m.serverDetail = serverdetail.New(nil, nil, nil, "srv", time.Hour)
	m.serverCreate = servercreate.New(nil, nil, nil)
	m.consoleLog = consolelog.New(nil, "srv", "web")
	m.actionLog = actionlog.New(nil, "srv", "web")
	m.auditLog = auditlog.New()
	m.volumeList = volumelist.New(nil, nil, time.Hour)
	m.volumeDetail = volumedetail.New(nil, nil, "vol")
	m.volumeCreate = volumecreate.New(nil)
	m.floatingIPList = floatingiplist.New(nil, time.Hour)
	m.secGroupView = secgroupview.New(nil, time.Hour)
	m.keypairList = keypairlist.New(nil, time.Hour)
	m.lbView = lbview.New(nil, time.Hour)
	m.keypairCreate = keypaircreate.New(nil)
	m.networkView = networkview.New(nil, time.Hour)
	m.keypairDetail = keypairdetail.New(nil, "kp")
	m.routerView = routerview.New(nil, time.Hour)
	m.imageView = imageview.New(nil, nil, time.Hour)
	m.hypervisorList = hypervisorlist.New(nil)
	m.serviceCatalog = servicecatalog.New(&gophercloud.ProviderClient{}, gophercloud.EndpointOpts{})
	m.dnsList = dnslist.New(nil)
	m.userManagement = usermanagement.New(&gophercloud.ProviderClient{}, gophercloud.EndpointOpts{})
	return m
}

// allViews enumerates every activeView constant after the cloud picker.
func allViews() []activeView {
	var out []activeView
	for v := viewServerList; ; v++ {
		m := Model{view: v}
		if m.viewName() == "" {
			return out
		}
		out = append(out, v)
	}
}

func TestEveryActiveViewHasRenderDispatch(t *testing.T) {
	if len(allViews()) != int(viewUserManagement) {
		t.Fatalf("allViews found %d views, want %d", len(allViews()), viewUserManagement)
	}
	base := renderModel()
	for _, v := range allViews() {
		m := base
		m.view = v
		t.Run(m.viewName(), func(t *testing.T) {
			content, ok := m.activeViewContent()
			if !ok {
				t.Fatal("no render case for view")
			}
			if strings.TrimSpace(content) == "" {
				t.Fatal("view rendered empty content")
			}
			first := strings.SplitN(strings.TrimLeft(content, "\n"), "\n", 2)[0]
			if !strings.Contains(m.viewContent(), first) {
				t.Fatalf("root frame does not include the view's first line %q", first)
			}
		})
	}
}

func hypervisorDNSFixture(t *testing.T, populated bool) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/os-hypervisors/detail":
			if populated {
				fmt.Fprint(w, `{"hypervisors":[{"id":"1","hypervisor_hostname":"compute-01","state":"up","status":"enabled","vcpus":8,"memory_mb":16384,"running_vms":2,"hypervisor_version":2000000,"hypervisor_type":"QEMU","cpu_info":{},"service":{"host":"compute-01","id":1}}]}`)
			} else {
				fmt.Fprint(w, `{"hypervisors":[]}`)
			}
		case r.URL.Path == "/zones":
			if populated {
				fmt.Fprint(w, `{"zones":[{"id":"z1","name":"example.org.","type":"PRIMARY"}]}`)
			} else {
				fmt.Fprint(w, `{"zones":[]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/recordsets"):
			fmt.Fprint(w, `{"recordsets":[]}`)
		default:
			fmt.Fprint(w, `{"servers":[]}`)
		}
	})
	m.view = viewServerList
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
	m.tabs = []TabDef{{Name: "Servers", Key: "servers"}, {Name: "DNS", Key: "dns"}}
	m.tabInited = make([]bool, len(m.tabs))
	return m
}

func drive(m Model, cmd tea.Cmd) Model {
	for _, msg := range quickMessages(cmd) {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestHypervisorAndDNSViewsRenderContent(t *testing.T) {
	cases := []struct {
		name      string
		open      func(m Model) (Model, tea.Cmd)
		header    string
		populated string
		empty     string
	}{
		{"hypervisors", func(m Model) (Model, tea.Cmd) {
			next, cmd := m.Update(press("H"))
			return next.(Model), cmd
		}, "Hypervisors", "compute-01", "No hypervisors found."},
		{"dns", func(m Model) (Model, tea.Cmd) {
			next, cmd := m.Update(press("2"))
			return next.(Model), cmd
		}, "DNS Zones", "example.org.", "No DNS zones found."},
	}
	for _, tc := range cases {
		for _, populated := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/populated=%v", tc.name, populated), func(t *testing.T) {
				m := hypervisorDNSFixture(t, populated)
				m, cmd := tc.open(m)
				m = drive(m, cmd)
				out := m.viewContent()
				if !strings.Contains(out, tc.header) {
					t.Fatalf("missing %q header in:\n%s", tc.header, out)
				}
				want := tc.empty
				if populated {
					want = tc.populated
				}
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in:\n%s", want, out)
				}
			})
		}
	}
}
