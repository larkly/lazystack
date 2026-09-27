package lbview

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// On a slow Octavia, the lookups for the first pools must not use up the
// deadline of the later ones: each call gets its own RequestTimeout.
func TestSlowDetailLookupsGetTheirOwnDeadlines(t *testing.T) {
	orig := shared.RequestTimeout
	shared.RequestTimeout = 250 * time.Millisecond
	t.Cleanup(func() { shared.RequestTimeout = orig })

	const pools = 3
	delay := 80 * time.Millisecond // 2 slow calls per pool exceed one deadline
	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/loadbalancers/lb-1"):
			fmt.Fprint(w, `{"loadbalancer":{"id":"lb-1","name":"edge"}}`)
		case strings.HasSuffix(p, "/listeners"):
			fmt.Fprint(w, `{"listeners":[]}`)
		case strings.HasSuffix(p, "/members"):
			time.Sleep(delay)
			fmt.Fprint(w, `{"members":[]}`)
		case strings.HasSuffix(p, "/pools"):
			var list []string
			for i := range pools {
				list = append(list, fmt.Sprintf(`{"id":"pool-%d","healthmonitor_id":"hm-%d"}`, i, i))
			}
			fmt.Fprintf(w, `{"pools":[%s]}`, strings.Join(list, ","))
		case strings.Contains(p, "/healthmonitors/"):
			time.Sleep(delay)
			id := p[strings.LastIndex(p, "/")+1:]
			fmt.Fprintf(w, `{"healthmonitor":{"id":%q,"type":"TCP"}}`, id)
		default:
			http.Error(w, "unexpected "+r.Method+" "+p, http.StatusNotFound)
		}
	}))
	defer cleanup()

	msg, ok := New(client, time.Minute).fetchDetail("lb-1")().(detailLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want detailLoadedMsg", msg)
	}
	if len(msg.memberErrs) != 0 || len(msg.monitorErrs) != 0 {
		t.Fatalf("later lookups starved: memberErrs=%v monitorErrs=%v", msg.memberErrs, msg.monitorErrs)
	}
	if len(msg.members) != pools || len(msg.monitors) != pools {
		t.Fatalf("members=%d monitors=%d, want %d each", len(msg.members), len(msg.monitors), pools)
	}
}

// fakeOctavia serves one load balancer with one pool that has a health
// monitor; member and monitor lookups fail while failLookups is set.
func fakeOctavia(failLookups *atomic.Bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/loadbalancers/lb-1"):
			fmt.Fprint(w, `{"loadbalancer":{"id":"lb-1","name":"edge"}}`)
		case strings.HasSuffix(p, "/listeners"):
			fmt.Fprint(w, `{"listeners":[]}`)
		case strings.HasSuffix(p, "/pools/pool-1/members"):
			if failLookups.Load() {
				http.Error(w, "members backend down", http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{"members":[]}`)
		case strings.HasSuffix(p, "/pools"):
			fmt.Fprint(w, `{"pools":[{"id":"pool-1","name":"web","protocol":"HTTP","lb_algorithm":"ROUND_ROBIN","healthmonitor_id":"hm-1"}]}`)
		case strings.HasSuffix(p, "/healthmonitors/hm-1"):
			if failLookups.Load() {
				http.Error(w, "monitor backend down", http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{"healthmonitor":{"id":"hm-1","type":"TCP","delay":5,"timeout":3,"max_retries":3}}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+p, http.StatusNotFound)
		}
	})
}

func TestDetailShowsFailedMemberAndMonitorLookups(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	client, cleanup := testutil.FakeServiceClient(fakeOctavia(&fail))
	defer cleanup()

	m := New(client, time.Minute)
	m.SetSize(140, 50)
	m, _ = m.Update(lbsLoadedMsg{lbs: []loadbalancer.LoadBalancer{{ID: "lb-1", Name: "edge"}}})
	m, _ = m.Update(m.fetchDetail("lb-1")())

	members := m.renderMembersContent(120, 10)
	if strings.Contains(members, "No members in this pool") || !strings.Contains(members, "unavailable") {
		t.Fatalf("members pane = %q, want the failed lookup shown", members)
	}
	pools := m.renderPoolsContent(120, 20)
	if strings.Contains(pools, "[0]") {
		t.Fatalf("pools pane claims 0 members after a failed lookup:\n%s", pools)
	}
	if !strings.Contains(pools, "monitor unavailable") {
		t.Fatalf("pools pane hides the failed monitor lookup:\n%s", pools)
	}
	if m.SelectedPoolMembersErr() == "" {
		t.Fatal("SelectedPoolMembersErr is empty after a failed member lookup")
	}

	// A successful refresh clears the errors.
	fail.Store(false)
	m, _ = m.Update(m.fetchDetail("lb-1")())
	if members := m.renderMembersContent(120, 10); !strings.Contains(members, "No members in this pool") {
		t.Fatalf("after recovery members pane = %q", members)
	}
	if m.SelectedPoolMembersErr() != "" {
		t.Fatal("member error not cleared after a successful refresh")
	}
	if pools := m.renderPoolsContent(120, 20); strings.Contains(pools, "unavailable") || !strings.Contains(pools, "TCP") {
		t.Fatalf("after recovery pools pane:\n%s", pools)
	}
}
