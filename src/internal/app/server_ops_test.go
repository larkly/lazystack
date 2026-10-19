package app

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// singleActionLabels pins the result labels of every single-resource
// confirm action that executeAction runs through one service call. Server
// lifecycle actions use one label for success and failure; resource actions
// have separate failure and success labels.
var singleActionLabels = []struct {
	action  string
	server  bool
	errLbl  string
	doneLbl string
	code    int
}{
	{"soft reboot", true, "Reboot", "Reboot", 202},
	{"hard reboot", true, "Hard reboot", "Hard reboot", 202},
	{"pause", true, "Pause", "Pause", 202},
	{"unpause", true, "Unpause", "Unpause", 202},
	{"suspend", true, "Suspend", "Suspend", 202},
	{"resume", true, "Resume", "Resume", 202},
	{"shelve", true, "Shelve", "Shelve", 202},
	{"unshelve", true, "Unshelve", "Unshelve", 202},
	{"stop", true, "Stop", "Stop", 202},
	{"start", true, "Start", "Start", 202},
	{"lock", true, "Lock", "Lock", 202},
	{"unlock", true, "Unlock", "Unlock", 202},
	{"unrescue", true, "Unrescue", "Unrescue", 202},
	{"delete_volume", false, "Delete volume", "Deleted volume", 202},
	{"release_fip", false, "Release FIP", "Released", 204},
	{"disassociate_fip", false, "Disassociate FIP", "Disassociated", 200},
	{"delete_router", false, "Delete router", "Deleted router", 204},
	{"delete_port", false, "Delete port", "Deleted port", 204},
	{"delete_network", false, "Delete network", "Deleted network", 204},
	{"delete_subnet", false, "Delete subnet", "Deleted subnet", 204},
	{"delete_sg", false, "Delete security group", "Deleted security group", 204},
	{"delete_sg_rule", false, "Delete rule", "Deleted rule from", 204},
	{"delete_lb", false, "Delete LB", "Deleted LB", 204},
	{"delete_lb_listener", false, "Delete listener", "Deleted listener", 204},
	{"delete_lb_pool", false, "Delete pool", "Deleted pool", 204},
	{"delete_lb_monitor", false, "Delete monitor", "Removed monitor from", 204},
	{"delete_keypair", false, "Delete keypair", "Deleted keypair", 202},
	{"delete_image", false, "Delete image", "Deleted image", 204},
}

func TestSingleActionResultLabels(t *testing.T) {
	for _, tc := range singleActionLabels {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/error=%v", tc.action, failed), func(t *testing.T) {
				m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if failed {
						http.Error(w, "fixture failure", http.StatusConflict)
						return
					}
					w.WriteHeader(tc.code)
					if tc.action == "disassociate_fip" {
						fmt.Fprint(w, `{"floatingip":{"id":"id","port_id":null}}`)
					}
				})
				name := "name"
				if tc.action == "delete_keypair" {
					name = "id" // keypairs are identified by name, carried in ServerID
				}
				_, cmd := m.executeAction(modal.ConfirmAction{Action: tc.action, ServerID: "id", Name: "name"})
				if cmd == nil {
					t.Fatal("no command")
				}
				msg := cmd()
				var gotAction, gotName string
				var ok bool
				switch {
				case tc.server && failed:
					var e shared.ServerActionErrMsg
					e, ok = msg.(shared.ServerActionErrMsg)
					gotAction, gotName = e.Action, e.Name
				case tc.server:
					var s shared.ServerActionMsg
					s, ok = msg.(shared.ServerActionMsg)
					gotAction, gotName = s.Action, s.Name
				case failed:
					var e shared.ResourceActionErrMsg
					e, ok = msg.(shared.ResourceActionErrMsg)
					gotAction, gotName = e.Action, e.Name
				default:
					var s shared.ResourceActionMsg
					s, ok = msg.(shared.ResourceActionMsg)
					gotAction, gotName = s.Action, s.Name
				}
				want := tc.doneLbl
				if failed {
					want = tc.errLbl
				}
				if !ok || gotAction != want || gotName != name {
					t.Fatalf("result=%#v, want action %q name %q", msg, want, name)
				}
			})
		}
	}
}

// Every op in the table must write the audit action and resource type the
// per-action closures it replaced wrote.
func TestServerOpsAuditActions(t *testing.T) {
	want := map[string]struct {
		audit    audit.ActionType
		resource string
	}{
		"soft reboot":        {audit.ActionReboot, "server"},
		"hard reboot":        {audit.ActionReboot, "server"},
		"pause":              {audit.ActionPause, "server"},
		"unpause":            {audit.ActionUnpause, "server"},
		"suspend":            {audit.ActionSuspend, "server"},
		"resume":             {audit.ActionResume, "server"},
		"shelve":             {audit.ActionShelve, "server"},
		"unshelve":           {audit.ActionUnshelve, "server"},
		"stop":               {audit.ActionStop, "server"},
		"start":              {audit.ActionStart, "server"},
		"lock":               {audit.ActionLock, "server"},
		"unlock":             {audit.ActionUnlock, "server"},
		"unrescue":           {audit.ActionUnrescue, "server"},
		"delete_volume":      {audit.ActionDelete, "volume"},
		"release_fip":        {audit.ActionReleaseFIP, "floating_ip"},
		"disassociate_fip":   {audit.ActionDetachFIP, "floating_ip"},
		"delete_router":      {audit.ActionDeleteRouter, "router"},
		"delete_port":        {audit.ActionDeletePort, "port"},
		"delete_network":     {audit.ActionDeleteNet, "network"},
		"delete_subnet":      {audit.ActionDeleteSubnet, "subnet"},
		"delete_sg":          {audit.ActionDeleteNet, "security_group"},
		"delete_sg_rule":     {audit.ActionDeleteNet, "security_group_rule"},
		"delete_lb":          {audit.ActionDeleteLB, "load_balancer"},
		"delete_lb_listener": {audit.ActionDeleteLB, "lb_listener"},
		"delete_lb_pool":     {audit.ActionDeleteLB, "lb_pool"},
		"delete_lb_monitor":  {audit.ActionDeleteLB, "lb_monitor"},
		"delete_keypair":     {audit.ActionDeleteKey, "keypair"},
		"delete_image":       {audit.ActionDeleteImage, "image"},
	}
	for name, op := range serverOps {
		w, ok := want[name]
		if !ok {
			t.Errorf("op %q has no expected audit action", name)
			continue
		}
		if op.audit != w.audit || op.resource != w.resource {
			t.Errorf("op %q audits %s/%s, want %s/%s", name, op.audit, op.resource, w.audit, w.resource)
		}
		if op.svc == nil || op.run == nil || op.label == "" {
			t.Errorf("op %q is incomplete: %+v", name, op)
		}
	}
	for name := range want {
		if _, ok := serverOps[name]; !ok {
			t.Errorf("op %q missing from serverOps", name)
		}
	}
	// Dedicated code keeps the actions with special results.
	for _, name := range []string{"delete", "rescue", "detach_volume", "delete_lb_member", "remove_router_interface"} {
		if _, ok := serverOps[name]; ok {
			t.Errorf("%q must keep its dedicated handling", name)
		}
	}
}
