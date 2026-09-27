package loadbalancer

import (
	"context"
	"fmt"
	"slices"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/larkly/lazystack/internal/shared"
)

// listenerPoolProtocols mirrors Octavia's VALID_LISTENER_POOL_PROTOCOL_MAP:
// the pool protocols a listener of a given protocol may route to.
var listenerPoolProtocols = map[string][]string{
	"TCP":              {"HTTP", "HTTPS", "PROXY", "PROXYV2", "TCP"},
	"HTTP":             {"HTTP", "PROXY", "PROXYV2"},
	"HTTPS":            {"HTTPS", "PROXY", "PROXYV2", "TCP"},
	"TERMINATED_HTTPS": {"HTTP", "PROXY", "PROXYV2"},
	"UDP":              {"UDP"},
	"SCTP":             {"SCTP"},
}

// CompatiblePoolProtocol reports whether a listener with listenerProtocol can
// use a pool with poolProtocol as its default pool.
func CompatiblePoolProtocol(listenerProtocol, poolProtocol string) bool {
	return slices.Contains(listenerPoolProtocols[listenerProtocol], poolProtocol)
}

// ListenerUpdate holds the listener fields an edit can change. Nil fields
// are left unchanged; an empty DefaultPoolID removes the default pool.
type ListenerUpdate struct {
	Name          *string
	Description   *string
	ConnLimit     *int
	DefaultPoolID *string
}

// UpdateListenerWithPool updates a listener in a single request and, when
// the default pool is part of the update, confirms from the response that
// Octavia recorded the requested binding.
func UpdateListenerWithPool(ctx context.Context, client *gophercloud.ServiceClient, id string, u ListenerUpdate) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdateListenerWithPool: starting, id=%s", id)
	opts := listeners.UpdateOpts{
		Name:          u.Name,
		Description:   u.Description,
		ConnLimit:     u.ConnLimit,
		DefaultPoolID: u.DefaultPoolID,
	}
	l, err := listeners.Update(ctx, client, id, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdateListenerWithPool: error: %v", err)
		return fmt.Errorf("updating listener %s: %w", id, err)
	}
	if u.DefaultPoolID != nil && l.DefaultPoolID != *u.DefaultPoolID {
		return fmt.Errorf("updating listener %s: default pool is %q, expected %q", id, l.DefaultPoolID, *u.DefaultPoolID)
	}
	shared.Debugf("[lb] UpdateListenerWithPool: success, id=%s", id)
	return nil
}

// SetListenerDefaultPool makes poolID the default pool of a listener.
func SetListenerDefaultPool(ctx context.Context, client *gophercloud.ServiceClient, listenerID, poolID string) error {
	return UpdateListenerWithPool(ctx, client, listenerID, ListenerUpdate{DefaultPoolID: &poolID})
}
