package app

import (
	"context"

	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/cloudpicker"
	"charm.land/bubbletea/v2"
)

func (m Model) connectToCloud(name string) tea.Cmd {
	return func() tea.Msg {
		client, err := cloud.Connect(context.Background(), name)
		if err != nil {
			return shared.CloudConnectErrMsg{Err: err}
		}
		return shared.CloudConnectedMsg{
			ComputeClient:      client.Compute,
			ImageClient:        client.Image,
			NetworkClient:      client.Network,
			BlockStorageClient: client.BlockStorage,
			LoadBalancerClient: client.LoadBalancer,
			ProviderClient:     client.ProviderClient,
			EndpointOpts:       client.EndpointOpts,
			Region:             client.Region,
		}
	}
}

func (m Model) switchToCloudPicker() (Model, tea.Cmd) {
	clouds, err := cloud.ListCloudNames()
	m.cloudPicker = cloudpicker.New(clouds, err)
	m.cloudPicker.SetSize(m.width, m.height)
	m.view = viewCloudPicker
	m.statusBar.CurrentView = "cloudpicker"
	m.statusBar.Hint = "Select a cloud to connect"
	return m, nil
}
