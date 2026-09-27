package app

import (
	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/auditlog"
	"github.com/larkly/lazystack/internal/ui/cloudpicker"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/hypervisorlist"
	"github.com/larkly/lazystack/internal/ui/keypairdetail"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/servicecatalog"
	"github.com/larkly/lazystack/internal/ui/usermanagement"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
)

func (m Model) connectToCloud(name string) tea.Cmd {
	shared.Debugf("[app] connectToCloud: start cloud=%s", name)
	return func() tea.Msg {
		ctx, cancel := actionCtxLong()
		defer cancel()
		client, err := cloud.Connect(ctx, name)
		if err != nil {
			shared.Debugf("[app] connectToCloud: error: %v", err)
			return shared.CloudConnectErrMsg{Err: err}
		}
		shared.Debugf("[app] connectToCloud: success cloud=%s", name)
		return shared.CloudConnectedMsg{
			ComputeClient:      client.Compute,
			ImageClient:        client.Image,
			NetworkClient:      client.Network,
			BlockStorageClient: client.BlockStorage,
			LoadBalancerClient: client.LoadBalancer,
			DNSClient:          client.DNS,
			ProviderClient:     client.ProviderClient,
			EndpointOpts:       client.EndpointOpts,
			Region:             client.Region,
			Warning:            client.CapabilityWarning,
		}
	}
}

// listCloudNames reads clouds.yaml. It is a variable so tests can inject a
// slow or failing implementation.
var listCloudNames = cloud.ListCloudNames

// cloudsListedMsg delivers the result of an asynchronous clouds.yaml read.
type cloudsListedMsg struct {
	seq    uint64
	clouds []string
	err    error
}

// switchToCloudPicker opens the cloud picker and reads clouds.yaml off the
// Update goroutine; the picker is filled when cloudsListedMsg arrives.
func (m Model) switchToCloudPicker() (Model, tea.Cmd) {
	m.cloudListSeq++
	seq := m.cloudListSeq
	// An explicit pick always shows the list, never the placeholder of the
	// automatic startup connection.
	m.autoCloud = ""
	m.cloudPicker = cloudpicker.New(nil, nil)
	m.cloudPicker.SetSize(m.width, m.height)
	m.view = viewCloudPicker
	m.statusBar.CurrentView = "cloudpicker"
	m.statusBar.Hint = "Loading clouds..."
	return m, func() tea.Msg {
		clouds, err := listCloudNames()
		return cloudsListedMsg{seq: seq, clouds: clouds, err: err}
	}
}

// applyCloudList fills the picker with a clouds.yaml listing, unless the
// listing was superseded or the user already left the picker.
func (m Model) applyCloudList(msg cloudsListedMsg) (Model, tea.Cmd) {
	if msg.seq != m.cloudListSeq || m.view != viewCloudPicker {
		shared.Debugf("[app] dropping stale cloud listing %d", msg.seq)
		return m, nil
	}
	if msg.err != nil {
		shared.Debugf("[app] switchToCloudPicker: error listing clouds: %v", msg.err)
	} else {
		shared.Debugf("[app] switchToCloudPicker: found %d clouds", len(msg.clouds))
	}
	m.cloudPicker = cloudpicker.New(msg.clouds, msg.err)
	m.cloudPicker.SetSize(m.width, m.height)
	m.statusBar.Hint = "Select a cloud to connect"
	return m, nil
}

// resetConnectionViews drops drill-down and overlay view models built with
// the previous connection's clients, so back-navigation can never restore
// them after a cloud or project switch. Tab models are rebuilt lazily
// because tabInited is reset on connect.
func (m *Model) resetConnectionViews() {
	m.serverDetail = serverdetail.Model{}
	m.volumeDetail = volumedetail.Model{}
	m.keypairDetail = keypairdetail.Model{}
	m.consoleLog = consolelog.Model{}
	m.actionLog = actionlog.Model{}
	m.auditLog = auditlog.Model{}
	m.hypervisorList = hypervisorlist.Model{}
	m.serviceCatalog = servicecatalog.Model{}
	m.userManagement = usermanagement.Model{}
}
