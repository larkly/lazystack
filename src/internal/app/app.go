package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bosse/lazystack/internal/cloud"
	"github.com/bosse/lazystack/internal/compute"
	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/ui/cloudpicker"
	"github.com/bosse/lazystack/internal/ui/help"
	"github.com/bosse/lazystack/internal/ui/modal"
	"github.com/bosse/lazystack/internal/ui/servercreate"
	"github.com/bosse/lazystack/internal/ui/serverdetail"
	"github.com/bosse/lazystack/internal/ui/serverlist"
	"github.com/bosse/lazystack/internal/ui/statusbar"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

type activeView int

const (
	viewCloudPicker activeView = iota
	viewServerList
	viewServerDetail
	viewServerCreate
)

type modalType int

const (
	modalNone modalType = iota
	modalConfirm
	modalError
)

// Model is the root application model.
type Model struct {
	view         activeView
	width        int
	height       int
	client       *cloud.Client
	cloudPicker  cloudpicker.Model
	serverList   serverlist.Model
	serverDetail serverdetail.Model
	serverCreate servercreate.Model
	statusBar    statusbar.Model
	help         help.Model
	confirm      modal.ConfirmModel
	errModal     modal.ErrorModel
	activeModal  modalType
	cloudName       string
	autoCloud       string
	refreshInterval time.Duration
	minWidth        int
	minHeight    int
	tooSmall     bool
}

// Options configures the application.
type Options struct {
	AlwaysPickCloud bool
	RefreshInterval time.Duration
}

// New creates the root model.
func New(opts Options) Model {
	clouds, err := cloud.ListCloudNames()

	refresh := opts.RefreshInterval
	if refresh == 0 {
		refresh = 5 * time.Second
	}

	// Auto-select if exactly one cloud and not forced to pick
	if err == nil && len(clouds) == 1 && !opts.AlwaysPickCloud {
		return Model{
			view:            viewCloudPicker,
			cloudPicker:     cloudpicker.New(clouds, nil),
			statusBar:       statusbar.New(),
			help:            help.New(),
			minWidth:        80,
			minHeight:       20,
			autoCloud:       clouds[0],
			refreshInterval: refresh,
		}
	}

	cp := cloudpicker.New(clouds, err)
	return Model{
		view:            viewCloudPicker,
		cloudPicker:     cp,
		statusBar:       statusbar.New(),
		help:            help.New(),
		minWidth:        80,
		minHeight:       20,
		refreshInterval: refresh,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	if m.autoCloud != "" {
		name := m.autoCloud
		return func() tea.Msg {
			return shared.CloudSelectedMsg{CloudName: name}
		}
	}
	return nil
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tooSmall = m.width < m.minWidth || m.height < m.minHeight
		m.cloudPicker.SetSize(m.width, m.height)
		m.confirm.SetSize(m.width, m.height)
		m.errModal.SetSize(m.width, m.height)
		m.help.Width = m.width
		m.help.Height = m.height
		m.statusBar.Width = m.width
		return m.updateActiveView(msg)

	case tea.KeyMsg:
		if m.help.Visible {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}

		if m.activeModal != modalNone {
			return m.updateModal(msg)
		}

		if m.view != viewServerCreate {
			switch {
			case key.Matches(msg, shared.Keys.Quit) && m.view != viewCloudPicker:
				return m, tea.Quit
			case key.Matches(msg, shared.Keys.Help):
				m.help.Visible = true
				m.help.View = m.viewName()
				return m, nil
			case key.Matches(msg, shared.Keys.CloudPick) && m.view != viewCloudPicker:
				return m.switchToCloudPicker()
			}
		}

		if m.view == viewServerList || m.view == viewServerDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Reboot) && msg.String() == "r" {
				return m.openRebootConfirm("soft reboot")
			}
			if m.view == viewServerDetail && key.Matches(msg, shared.Keys.HardReboot) && msg.String() == "R" {
				return m.openRebootConfirm("hard reboot")
			}
		}

		return m.updateActiveView(msg)

	case shared.CloudSelectedMsg:
		m.cloudName = msg.CloudName
		m.statusBar.Hint = "Connecting..."
		return m, m.connectToCloud(msg.CloudName)

	case shared.CloudConnectedMsg:
		m.client = &cloud.Client{
			CloudName: m.cloudName,
			Compute:   msg.ComputeClient,
			Image:     msg.ImageClient,
			Network:   msg.NetworkClient,
		}
		m.statusBar.CloudName = m.cloudName
		m.statusBar.Region = msg.Region
		m.serverList = serverlist.New(msg.ComputeClient, m.refreshInterval)
		m.serverList.SetSize(m.width, m.height)
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, m.serverList.Init()

	case shared.CloudConnectErrMsg:
		m.errModal = modal.NewError("Cloud Connection", msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case shared.ViewChangeMsg:
		return m.handleViewChange(msg)

	case modal.ConfirmAction:
		m.activeModal = modalNone
		if msg.Confirm {
			return m.executeAction(msg)
		}
		return m, nil

	case modal.ErrorDismissedMsg:
		m.activeModal = modalNone
		return m, nil

	case shared.ServerActionMsg:
		m.statusBar.Hint = fmt.Sprintf("%s %s: success", msg.Action, msg.Name)
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }

	case shared.ServerActionErrMsg:
		m.errModal = modal.NewError(
			fmt.Sprintf("%s %s", msg.Action, msg.Name), msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case shared.TickMsg:
		// Always route ticks to serverlist so auto-refresh survives view changes
		var cmd tea.Cmd
		m.serverList, cmd = m.serverList.Update(msg)
		return m, cmd

	default:
		return m.updateActiveView(msg)
	}
}

func (m Model) updateActiveView(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case viewCloudPicker:
		m.cloudPicker, cmd = m.cloudPicker.Update(msg)
	case viewServerList:
		m.serverList, cmd = m.serverList.Update(msg)
		m.statusBar.Hint = m.serverList.Hints()
	case viewServerDetail:
		m.serverDetail, cmd = m.serverDetail.Update(msg)
		m.statusBar.Hint = m.serverDetail.Hints()
	case viewServerCreate:
		m.serverCreate, cmd = m.serverCreate.Update(msg)
		m.statusBar.Hint = m.serverCreate.Hints()
	}
	return m, cmd
}

func (m Model) updateModal(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeModal {
	case modalConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	case modalError:
		m.errModal, cmd = m.errModal.Update(msg)
	}
	return m, cmd
}

func (m Model) handleViewChange(msg shared.ViewChangeMsg) (Model, tea.Cmd) {
	switch msg.View {
	case "serverlist":
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }

	case "serverdetail":
		if s := m.serverList.SelectedServer(); s != nil {
			m.serverDetail = serverdetail.New(m.client.Compute, s.ID)
			m.serverDetail.SetSize(m.width, m.height)
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, m.serverDetail.Init()
		}
		return m, nil

	case "servercreate":
		m.serverCreate = servercreate.New(m.client.Compute, m.client.Image, m.client.Network)
		m.serverCreate.SetSize(m.width, m.height)
		m.view = viewServerCreate
		m.statusBar.CurrentView = "servercreate"
		m.statusBar.Hint = m.serverCreate.Hints()
		return m, m.serverCreate.Init()
	}
	return m, nil
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

func (m Model) openDeleteConfirm() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete", id, name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openRebootConfirm(action string) (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm(action, id, name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) executeAction(action modal.ConfirmAction) (Model, tea.Cmd) {
	client := m.client.Compute
	switch action.Action {
	case "delete":
		return m, func() tea.Msg {
			err := compute.DeleteServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Delete", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Delete", Name: action.Name}
		}
	case "soft reboot":
		return m, func() tea.Msg {
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.SoftReboot)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Reboot", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Reboot", Name: action.Name}
		}
	case "hard reboot":
		return m, func() tea.Msg {
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.HardReboot)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Hard reboot", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Hard reboot", Name: action.Name}
		}
	}
	return m, nil
}

func (m Model) connectToCloud(name string) tea.Cmd {
	return func() tea.Msg {
		client, err := cloud.Connect(context.Background(), name)
		if err != nil {
			return shared.CloudConnectErrMsg{Err: err}
		}
		return shared.CloudConnectedMsg{
			ComputeClient: client.Compute,
			ImageClient:   client.Image,
			NetworkClient: client.Network,
			Region:        client.Region,
		}
	}
}

func (m Model) viewName() string {
	switch m.view {
	case viewCloudPicker:
		return "cloudpicker"
	case viewServerList:
		return "serverlist"
	case viewServerDetail:
		return "serverdetail"
	case viewServerCreate:
		return "servercreate"
	}
	return ""
}

// View renders the full UI.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.tooSmall {
		msg := fmt.Sprintf("Terminal too small (%dx%d). Need at least %dx%d.",
			m.width, m.height, m.minWidth, m.minHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(msg))
	}

	if m.help.Visible {
		return m.help.Render()
	}

	if m.activeModal == modalConfirm {
		return m.confirm.View()
	}
	if m.activeModal == modalError {
		return m.errModal.View()
	}

	var content string
	switch m.view {
	case viewCloudPicker:
		return m.cloudPicker.View()
	case viewServerList:
		content = m.serverList.View()
	case viewServerDetail:
		content = m.serverDetail.View()
	case viewServerCreate:
		content = m.serverCreate.View()
	}

	contentHeight := m.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	padded := lipgloss.NewStyle().Height(contentHeight).Render(content)
	return padded + "\n" + m.statusBar.Render()
}
