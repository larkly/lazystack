package fippicker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bosse/lazystack/internal/network"
	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type fipsLoadedMsg struct{ fips []network.FloatingIP }
type fetchErrMsg struct{ err error }
type associateDoneMsg struct{ fipAddr, serverName string }
type associateErrMsg struct{ err error }
type allocateDoneMsg struct{ fipAddr, serverName string }
type allocateErrMsg struct{ err error }

// Model is the floating IP picker modal.
type Model struct {
	Active     bool
	client     *gophercloud.ServiceClient
	serverID   string
	serverName string
	fips       []network.FloatingIP // unassociated FIPs
	cursor     int
	loading    bool
	submitting bool
	spinner    spinner.Model
	width      int
	height     int
	err        string
	scrollOff  int
}

// New creates a FIP picker for the given server.
func New(client *gophercloud.ServiceClient, serverID, serverName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:     true,
		client:     client,
		serverID:   serverID,
		serverName: serverName,
		loading:    true,
		spinner:    s,
	}
}

// Init fetches unassociated floating IPs.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchUnassociatedFIPs())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fipsLoadedMsg:
		m.loading = false
		m.fips = msg.fips
		// If no unassociated FIPs, auto-allocate
		if len(m.fips) == 0 {
			m.submitting = true
			return m, m.allocateAndAssociate()
		}
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case associateDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Assigned", Name: fmt.Sprintf("%s → %s", msg.fipAddr, msg.serverName)}
		}

	case associateErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case allocateDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Allocated & assigned", Name: fmt.Sprintf("%s → %s", msg.fipAddr, msg.serverName)}
		}

	case allocateErrMsg:
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
			// +1 for the "Allocate new" option at the end
			if m.cursor < len(m.fips) {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Enter):
			if m.cursor < len(m.fips) {
				// Selected an existing FIP
				m.submitting = true
				return m, tea.Batch(m.spinner.Tick, m.associateFIP(m.fips[m.cursor]))
			}
			// "Allocate new" option
			m.submitting = true
			return m, tea.Batch(m.spinner.Tick, m.allocateAndAssociate())
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

// View renders the FIP picker modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Assign Floating IP to " + m.serverName)

	var body string
	if m.loading {
		body = m.spinner.View() + " Loading floating IPs..."
	} else if m.submitting {
		body = m.spinner.View() + " Assigning..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else {
		var lines []string
		th := m.listHeight()
		totalItems := len(m.fips) + 1 // +1 for allocate new
		end := m.scrollOff + th
		if end > totalItems {
			end = totalItems
		}

		for i := m.scrollOff; i < end; i++ {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}

			if i < len(m.fips) {
				fip := m.fips[i]
				style := lipgloss.NewStyle().Foreground(shared.ColorFg)
				if i == m.cursor {
					style = style.Foreground(shared.ColorHighlight).Bold(true)
				}
				lines = append(lines, cursor+style.Render(fip.FloatingIP))
			} else {
				// "Allocate new" option
				style := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
				if i == m.cursor {
					style = style.Foreground(shared.ColorHighlight).Bold(true)
				}
				lines = append(lines, cursor+style.Render("+ Allocate new floating IP"))
			}
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

func (m Model) fetchUnassociatedFIPs() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		fips, err := network.ListFloatingIPs(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		var unassociated []network.FloatingIP
		for _, fip := range fips {
			if fip.PortID == "" {
				unassociated = append(unassociated, fip)
			}
		}
		return fipsLoadedMsg{fips: unassociated}
	}
}

func (m Model) associateFIP(fip network.FloatingIP) tea.Cmd {
	client := m.client
	serverID := m.serverID
	serverName := m.serverName
	fipID := fip.ID
	fipAddr := fip.FloatingIP
	return func() tea.Msg {
		portID, err := network.FindServerPortID(context.Background(), client, serverID)
		if err != nil {
			return associateErrMsg{err: err}
		}
		err = network.AssociateFloatingIP(context.Background(), client, fipID, portID)
		if err != nil {
			return associateErrMsg{err: err}
		}
		return associateDoneMsg{fipAddr: fipAddr, serverName: serverName}
	}
}

func (m Model) allocateAndAssociate() tea.Cmd {
	client := m.client
	serverID := m.serverID
	serverName := m.serverName
	return func() tea.Msg {
		nets, err := network.ListExternalNetworks(context.Background(), client)
		if err != nil {
			return allocateErrMsg{err: err}
		}
		if len(nets) == 0 {
			return allocateErrMsg{err: fmt.Errorf("no external networks available")}
		}
		fip, err := network.AllocateFloatingIP(context.Background(), client, nets[0].ID)
		if err != nil {
			return allocateErrMsg{err: err}
		}
		portID, err := network.FindServerPortID(context.Background(), client, serverID)
		if err != nil {
			return allocateErrMsg{err: err}
		}
		err = network.AssociateFloatingIP(context.Background(), client, fip.ID, portID)
		if err != nil {
			return allocateErrMsg{err: err}
		}
		return allocateDoneMsg{fipAddr: fip.FloatingIP, serverName: serverName}
	}
}
