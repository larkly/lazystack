package subnetpicker

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type subnetsLoadedMsg struct{ subnets []network.Subnet }
type fetchErrMsg struct{ err error }
type interfaceAddedMsg struct{ routerName string }
type interfaceAddErrMsg struct{ err error }

// Model is the subnet picker modal for adding router interfaces.
type Model struct {
	Active     bool
	client     *gophercloud.ServiceClient
	routerID   string
	routerName string
	subnets    []network.Subnet
	cursor     int
	scrollOff  int
	loading    bool
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a subnet picker for the given router.
func New(client *gophercloud.ServiceClient, routerID, routerName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:     true,
		client:     client,
		routerID:   routerID,
		routerName: routerName,
		loading:    true,
		spinner:    s,
	}
}

// Init fetches all subnets.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchSubnets())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subnetsLoadedMsg:
		m.loading = false
		m.subnets = msg.subnets
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case interfaceAddedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Added interface to", Name: msg.routerName}
		}

	case interfaceAddErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.submitting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.loading || m.submitting {
			return m, nil
		}
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.subnets)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.subnets) > 0 && m.cursor < len(m.subnets) {
				m.submitting = true
				return m, tea.Batch(m.spinner.Tick, m.addInterface(m.subnets[m.cursor]))
			}
		}
	}
	return m, nil
}

func (m *Model) ensureVisible() {
	th := m.listHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) listHeight() int {
	h := m.height - 12 // modal chrome
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the subnet picker modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Add Interface to " + m.routerName)

	var body string
	if m.loading {
		body = m.spinner.View() + " Loading subnets..."
	} else if m.submitting {
		body = m.spinner.View() + " Adding interface..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else if len(m.subnets) == 0 {
		body = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No subnets available")
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else {
		var lines []string
		th := m.listHeight()
		end := m.scrollOff + th
		if end > len(m.subnets) {
			end = len(m.subnets)
		}

		for i := m.scrollOff; i < end; i++ {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}

			sub := m.subnets[i]
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if i == m.cursor {
				style = style.Foreground(shared.ColorHighlight).Bold(true)
			}
			name := sub.Name
			if name == "" {
				name = sub.ID[:8]
			}
			cidr := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" " + sub.CIDR)
			lines = append(lines, fmt.Sprintf("%s%s%s", cursor, style.Render(name), cidr))
		}
		body = strings.Join(lines, "\n")
		body += "\n\n" + shared.StyleHelp.Render("↑↓ navigate • enter select • esc cancel")
	}

	content := title + "\n\n" + body
	modalWidth := 50
	if m.width > 0 && m.width < 60 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) fetchSubnets() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		subnets, err := network.ListSubnets(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return subnetsLoadedMsg{subnets: subnets}
	}
}

func (m Model) addInterface(sub network.Subnet) tea.Cmd {
	client := m.client
	routerID := m.routerID
	routerName := m.routerName
	subnetID := sub.ID
	return func() tea.Msg {
		err := network.AddRouterInterface(context.Background(), client, routerID, subnetID)
		if err != nil {
			return interfaceAddErrMsg{err: err}
		}
		return interfaceAddedMsg{routerName: routerName}
	}
}
