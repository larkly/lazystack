package lbpoolcreate

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
)

func TestPoolCreateErrorWarnsAboutLeftoverPool(t *testing.T) {
	m := New(nil, "lb", "web-lb")
	m.submitting = true
	// The original failure is an HTTP error, which SanitizeAPIError maps to a
	// generic friendly message that would otherwise hide the leftover pool.
	orig := fmt.Errorf("creating health monitor for pool pool-1: %w", gophercloud.ErrUnexpectedResponseCode{Actual: 403})
	err := &loadbalancer.PoolCleanupError{PoolID: "pool-1", Err: orig, CleanupErr: errors.New("500")}
	m, _ = m.Update(poolCreateErrMsg{err: err})
	if m.submitting {
		t.Fatal("still submitting after error")
	}
	if !strings.Contains(m.err, "pool-1") || !strings.Contains(m.err, "may remain") {
		t.Fatalf("leftover pool not surfaced: %q", m.err)
	}

	m, _ = m.Update(poolCreateErrMsg{err: errors.New("plain failure")})
	if strings.Contains(m.err, "may remain") {
		t.Fatalf("unexpected leftover warning: %q", m.err)
	}
}
