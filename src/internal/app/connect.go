package app

import (
	"context"

	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/cloudpicker"
	"charm.land/bubbletea/v2"
)

func (m Model) connectToCloud(name string) tea.Cmd {
	shared.Debugf("[app] connectToCloud: start cloud=%s", name)
	return func() tea.Msg {
		client, err := cloud.Connect(context.Background(), name)
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
		}
	}
}

func (m Model) switchToCloudPicker() (Model, tea.Cmd) {
	clouds, err := cloud.ListCloudNames()
	if err != nil {
		shared.Debugf("[app] switchToCloudPicker: error listing clouds: %v", err)
	} else {
		shared.Debugf("[app] switchToCloudPicker: found %d clouds", len(clouds))
	}
	m.cloudPicker = cloudpicker.New(clouds, err)
	m.cloudPicker.SetSize(m.width, m.height)
	m.view = viewCloudPicker
	m.statusBar.CurrentView = "cloudpicker"
	m.statusBar.Hint = "Select a cloud to connect"
	return m, nil
}
