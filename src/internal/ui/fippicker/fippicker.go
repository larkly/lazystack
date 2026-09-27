package fippicker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

type fipsLoadedMsg struct{ fips []network.FloatingIP }
type fetchErrMsg struct{ err error }
type associateDoneMsg struct{ fipAddr, serverName string }
type associateErrMsg struct{ err error }
type allocateDoneMsg struct{ fipAddr, serverName string }
type allocateErrMsg struct{ err error }

// targetsLoadedMsg carries the server addresses a floating IP could be bound
// to. fip is nil when a new floating IP will be allocated from extNetID.
type targetsLoadedMsg struct {
	fip      *network.FloatingIP
	extNetID string
	targets  []network.FloatingIPTarget
}

// releaseTimeout bounds the cleanup of a floating IP that was allocated but
// could not be associated.
const releaseTimeout = 30 * time.Second

// Model is the floating IP picker modal.
type Model struct {
	Active     bool
	client     *gophercloud.ServiceClient
	serverID   string
	serverName string
	projectID  string               // scope of the token; "" if unknown
	fips       []network.FloatingIP // unassociated FIPs
	cursor     int
	loading    bool
	submitting bool
	resolving  bool
	spinner    spinner.Model
	width      int
	height     int
	err        string
	scrollOff  int

	// Port selection step, shown when more than one server address could
	// take the floating IP.
	choosingPort bool
	portChoices  []network.FloatingIPTarget
	portCursor   int
	portWarning  string
	pendingFIP   *network.FloatingIP
	pendingExtID string
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
		projectID:  tokenProjectID(client),
		loading:    true,
		spinner:    s,
	}
}

// tokenProjectID returns the project the client's token is scoped to, or ""
// when it cannot be determined.
func tokenProjectID(client *gophercloud.ServiceClient) string {
	if client == nil || client.ProviderClient == nil {
		return ""
	}
	ar, ok := client.ProviderClient.GetAuthResult().(interface {
		ExtractProject() (*tokens.Project, error)
	})
	if !ok {
		return ""
	}
	proj, err := ar.ExtractProject()
	if err != nil || proj == nil {
		return ""
	}
	return proj.ID
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
			m.resolving = true
			return m, m.resolveTargets(nil)
		}
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case targetsLoadedMsg:
		return m.handleTargets(msg)

	case associateDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Assigned", Name: fmt.Sprintf("%s → %s", msg.fipAddr, msg.serverName)}
		}

	case associateErrMsg:
		m.submitting = false
		m.resolving = false
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
		m.resolving = false
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
		if m.choosingPort && m.err == "" {
			return m.handlePortKey(msg)
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
			if m.err != "" {
				return m, nil
			}
			m.submitting = true
			m.resolving = true
			if m.cursor < len(m.fips) {
				// Selected an existing FIP
				fip := m.fips[m.cursor]
				return m, tea.Batch(m.spinner.Tick, m.resolveTargets(&fip))
			}
			// "Allocate new" option
			return m, tea.Batch(m.spinner.Tick, m.resolveTargets(nil))
		}
	}
	return m, nil
}

// handleTargets decides which server address receives the floating IP.
// Addresses whose subnet a router connects to the floating IP's external
// network are eligible; a single eligible address is used directly, several
// are offered for an explicit choice. Without any eligible address the
// IPv4 addresses are still offered, with a warning, because router
// visibility can be restricted by policy.
func (m Model) handleTargets(msg targetsLoadedMsg) (Model, tea.Cmd) {
	m.resolving = false
	m.pendingFIP = msg.fip
	m.pendingExtID = msg.extNetID
	if len(msg.targets) == 0 {
		m.submitting = false
		m.err = "server " + m.serverName + " has no port with an IPv4 address; floating IPs need an IPv4 fixed IP"
		return m, nil
	}
	var eligible []network.FloatingIPTarget
	for _, t := range msg.targets {
		if t.ReachableFrom(msg.extNetID) {
			eligible = append(eligible, t)
		}
	}
	if len(eligible) == 1 {
		return m, m.submitTarget(eligible[0])
	}
	m.submitting = false
	m.choosingPort = true
	m.portCursor = 0
	m.portWarning = ""
	m.portChoices = eligible
	if len(eligible) == 0 {
		m.portChoices = msg.targets
		m.portWarning = "No router connecting these addresses to the floating IP's network was found; Neutron may reject the association."
	}
	return m, nil
}

func (m Model) handlePortKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.choosingPort = false
		m.portChoices = nil
		if len(m.fips) == 0 {
			m.Active = false
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.portCursor > 0 {
			m.portCursor--
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.portCursor < len(m.portChoices)-1 {
			m.portCursor++
		}
	case key.Matches(msg, shared.Keys.Enter):
		if m.portCursor < len(m.portChoices) {
			return m, m.submitTarget(m.portChoices[m.portCursor])
		}
	}
	return m, nil
}

// submitTarget associates the pending (or a newly allocated) floating IP
// with the chosen server address.
func (m *Model) submitTarget(target network.FloatingIPTarget) tea.Cmd {
	m.submitting = true
	m.choosingPort = false
	if m.pendingFIP != nil {
		return tea.Batch(m.spinner.Tick, m.associateFIP(*m.pendingFIP, target))
	}
	return tea.Batch(m.spinner.Tick, m.allocateAndAssociate(m.pendingExtID, target))
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
	} else if m.resolving {
		body = m.spinner.View() + " Finding server addresses..."
	} else if m.submitting {
		body = m.spinner.View() + " Assigning..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else if m.choosingPort {
		body = m.portChoiceView()
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

func (m Model) portChoiceView() string {
	what := "the new floating IP"
	if m.pendingFIP != nil {
		what = m.pendingFIP.FloatingIP
	}
	lines := []string{"Choose the server address for " + what + ":", ""}
	for i, t := range m.portChoices {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.portCursor {
			cursor = "▸ "
			style = style.Foreground(shared.ColorHighlight).Bold(true)
		}
		lines = append(lines, cursor+style.Render(t.Label()))
	}
	if m.portWarning != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(m.portWarning))
	}
	lines = append(lines, "", shared.StyleHelp.Render("↑↓ navigate • enter assign • esc back"))
	return strings.Join(lines, "\n")
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) fetchUnassociatedFIPs() tea.Cmd {
	client := m.client
	projectID := m.projectID
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		fips, err := network.ListFloatingIPs(ctx, client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		var unassociated []network.FloatingIP
		for _, fip := range fips {
			// Admin credentials list every project's FIPs; only offer
			// our own so a server is never given another project's IP.
			if projectID != "" && fip.TenantID != "" && fip.TenantID != projectID {
				continue
			}
			if fip.PortID == "" {
				unassociated = append(unassociated, fip)
			}
		}
		return fipsLoadedMsg{fips: unassociated}
	}
}

// resolveTargets looks up the server's IPv4 addresses. For a new floating
// IP (fip == nil) it also picks the external network to allocate from,
// preferring one that a router connects to the server.
func (m Model) resolveTargets(fip *network.FloatingIP) tea.Cmd {
	client := m.client
	serverID := m.serverID
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		targets, err := network.ListFloatingIPTargets(ctx, client, serverID)
		if err != nil {
			shared.Debugf("[fippicker] error listing addresses for server %s: %v", serverID, err)
			if fip == nil {
				return allocateErrMsg{err: err}
			}
			return associateErrMsg{err: err}
		}
		if fip != nil {
			return targetsLoadedMsg{fip: fip, extNetID: fip.FloatingNetworkID, targets: targets}
		}
		if len(targets) == 0 {
			// Nothing can take a floating IP; do not allocate one.
			return targetsLoadedMsg{}
		}
		nets, err := network.ListExternalNetworks(ctx, client)
		if err != nil {
			shared.Debugf("[fippicker] error listing external networks: %v", err)
			return allocateErrMsg{err: err}
		}
		if len(nets) == 0 {
			shared.Debugf("[fippicker] no external networks available")
			return allocateErrMsg{err: fmt.Errorf("no external networks available")}
		}
		extNetID := nets[0].ID
	pick:
		for _, n := range nets {
			for _, t := range targets {
				if t.ReachableFrom(n.ID) {
					extNetID = n.ID
					break pick
				}
			}
		}
		return targetsLoadedMsg{extNetID: extNetID, targets: targets}
	}
}

func (m Model) associateFIP(fip network.FloatingIP, target network.FloatingIPTarget) tea.Cmd {
	client := m.client
	serverName := m.serverName
	fipID := fip.ID
	fipAddr := fip.FloatingIP
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[fippicker] associating FIP %s (%s) to %s on server %s", fipID, fipAddr, target.Label(), serverName)
		err := network.AssociateFloatingIPToAddress(ctx, client, fipID, target.PortID, target.IPAddress)
		if err != nil {
			shared.Debugf("[fippicker] error associating FIP %s: %v", fipID, err)
			return associateErrMsg{err: err}
		}
		shared.Debugf("[fippicker] associated FIP %s to server %s", fipAddr, serverName)
		return associateDoneMsg{fipAddr: fipAddr, serverName: serverName}
	}
}

func (m Model) allocateAndAssociate(extNetID string, target network.FloatingIPTarget) tea.Cmd {
	client := m.client
	serverName := m.serverName
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[fippicker] allocating FIP from %s for %s on server %s", extNetID, target.Label(), serverName)
		fip, err := network.AllocateFloatingIP(ctx, client, extNetID)
		if err != nil {
			shared.Debugf("[fippicker] error allocating FIP: %v", err)
			return allocateErrMsg{err: err}
		}
		err = network.AssociateFloatingIPToAddress(ctx, client, fip.ID, target.PortID, target.IPAddress)
		if err != nil {
			shared.Debugf("[fippicker] error associating FIP %s: %v", fip.ID, err)
			// Do not leave an unused, billable address behind.
			return allocateErrMsg{err: releaseAfterFailure(client, fip, err)}
		}
		shared.Debugf("[fippicker] allocated and associated FIP %s to server %s", fip.FloatingIP, serverName)
		return allocateDoneMsg{fipAddr: fip.FloatingIP, serverName: serverName}
	}
}

// releaseAfterFailure releases a floating IP that was allocated for a server
// but could not be associated, so it does not linger unused and hold quota.
// The returned error wraps cause and says whether the IP was released or, if
// that failed too, which IP is left over.
func releaseAfterFailure(client *gophercloud.ServiceClient, fip *network.FloatingIP, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()
	if err := network.ReleaseFloatingIP(ctx, client, fip.ID); err != nil {
		shared.Debugf("[fippicker] error releasing unassociated FIP %s: %v", fip.ID, err)
		return fmt.Errorf("%w; floating IP %s (ID %s) is still allocated, release it manually: %v", cause, fip.FloatingIP, fip.ID, err)
	}
	shared.Debugf("[fippicker] released unassociated FIP %s", fip.ID)
	return fmt.Errorf("%w (allocated floating IP %s was released)", cause, fip.FloatingIP)
}
