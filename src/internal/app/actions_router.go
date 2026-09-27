package app

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

// routerInterfaceTarget identifies one router interface exactly as it was
// shown in the confirmation dialog.
type routerInterfaceTarget struct {
	routerID, subnetID, portID string
}

// encodeRouterInterfaceTarget packs the captured target into the
// confirmation payload (same "a|b" convention as LB member deletes).
func encodeRouterInterfaceTarget(routerID, subnetID, portID string) string {
	return routerID + "|" + subnetID + "|" + portID
}

func decodeRouterInterfaceTarget(s string) (routerInterfaceTarget, bool) {
	parts := strings.Split(s, "|")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return routerInterfaceTarget{}, false
	}
	return routerInterfaceTarget{routerID: parts[0], subnetID: parts[1], portID: parts[2]}, true
}

// removeRouterInterfaceCmd removes exactly the confirmed interface. The port
// is re-read first so a target that moved, disappeared or changed shape
// since the dialog opened is rejected (or handled as a multi-IP port)
// instead of acting on stale local state.
func (m Model) removeRouterInterfaceCmd(t routerInterfaceTarget, name string) tea.Cmd {
	netClient := m.client.Network
	return func() tea.Msg {
		ctx, cancel := actionCtx()
		defer cancel()
		port, err := network.GetPort(ctx, netClient, t.portID)
		if err != nil {
			shared.Debugf("[action] remove interface: get port %s failed: %s", t.portID, err)
			return shared.ResourceActionErrMsg{Action: "Remove interface", Name: name, Err: err}
		}
		onSubnet := false
		for _, ip := range port.FixedIPs {
			if ip.SubnetID == t.subnetID {
				onSubnet = true
			}
		}
		if port.DeviceID != t.routerID || !onSubnet {
			err := fmt.Errorf("interface (subnet %s, port %s) is no longer attached to this router; refresh and try again", t.subnetID, t.portID)
			shared.Debugf("[action] remove interface from %s: %s", name, err)
			return shared.ResourceActionErrMsg{Action: "Remove interface", Name: name, Err: err}
		}
		if len(port.FixedIPs) > 1 {
			// Multi-IP port: remove just this fixed IP, keep the port.
			shared.Debugf("[action] removing fixed IP (subnet %s) from port %s on router %s", t.subnetID, t.portID, name)
			err = network.RemoveFixedIPFromPort(ctx, netClient, t.portID, t.subnetID)
		} else {
			// Single-IP port: detach the whole interface.
			shared.Debugf("[action] removing router interface (subnet %s) from %s", t.subnetID, name)
			err = network.RemoveRouterInterface(ctx, netClient, t.routerID, t.subnetID)
		}
		if err != nil {
			shared.Debugf("[action] remove interface (subnet %s) from %s failed: %s", t.subnetID, name, err)
			return shared.ResourceActionErrMsg{Action: "Remove interface", Name: name, Err: err}
		}
		shared.Debugf("[action] removed interface (subnet %s) from %s", t.subnetID, name)
		return shared.ResourceActionMsg{Action: "Removed interface from", Name: name}
	}
}
