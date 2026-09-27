package usermanagement

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

type usersLoadedMsg struct {
	items []compute.User
}

type usersErrMsg struct {
	err error
}

// userMutatedMsg reports a toggle or delete. err is the mutation's own
// failure; refreshErr is a failure of the follow-up list, which must not
// hide a mutation that did succeed.
type userMutatedMsg struct {
	action     actionKind
	user       compute.User
	err        error
	items      []compute.User
	refreshErr error
}

type actionKind int

const (
	actionToggle actionKind = iota + 1
	actionDelete
)

// pendingAction is a mutation waiting for explicit confirmation.
type pendingAction struct {
	kind actionKind
	user compute.User
}

// Model is the user management viewer.
type Model struct {
	providerClient *gophercloud.ProviderClient
	endpointOpts   gophercloud.EndpointOpts
	items          []compute.User
	cursor         int
	scroll         int
	width          int
	height         int
	loading        bool
	spinner        spinner.Model
	err            string
	notice         string         // result of the last mutation, or why it was refused
	pending        *pendingAction // mutation awaiting confirmation
	busy           bool           // a mutation request is in flight
}

// New creates a user management model.
func New(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		providerClient: pc,
		endpointOpts:   eo,
		loading:        true,
		spinner:        s,
	}
}

// Init fetches users.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetch())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case usersLoadedMsg:
		m.loading = false
		m.setItems(msg.items)
		m.err = ""
		m.pending = nil
		m.busy = false
		return m, nil

	case usersErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		m.pending = nil
		m.busy = false
		return m, nil

	case userMutatedMsg:
		m.loading = false
		m.busy = false
		m.notice = mutationNotice(msg)
		if msg.err != nil {
			return m, nil
		}
		if msg.refreshErr == nil {
			m.err = ""
			m.setItems(msg.items)
			return m, nil
		}
		// The change went through but the list could not be reloaded:
		// reflect it locally rather than showing stale state.
		m.setItems(applyMutation(m.items, msg))
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.busy {
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
		if m.pending != nil {
			switch {
			case key.Matches(msg, shared.Keys.Confirm):
				p := *m.pending
				m.pending = nil
				m.busy = true
				m.notice = ""
				return m, tea.Batch(m.spinner.Tick, m.doAction(p))
			case key.Matches(msg, shared.Keys.Deny, shared.Keys.Back):
				m.pending = nil
				return m, nil
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}

		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scroll {
					m.scroll = m.cursor
				}
			}

		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.scroll+visible {
					m.scroll = m.cursor - visible + 1
				}
			}

		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.visibleRows()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.scroll = m.cursor

		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.visibleRows()
			if m.cursor >= len(m.items) {
				m.cursor = len(m.items) - 1
			}
			visible := m.visibleRows()
			if m.cursor >= m.scroll+visible {
				m.scroll = m.cursor - visible + 1
			}

		case key.Matches(msg, shared.Keys.Enter):
			m.request(actionToggle)

		case key.Matches(msg, shared.Keys.Delete):
			m.request(actionDelete)
		}
	}

	return m, nil
}

// request asks for confirmation of an action on the selected user, unless
// it would disable or delete the identity lazystack is authenticated as
// (which would revoke the session's own token).
func (m *Model) request(kind actionKind) {
	if m.busy || m.cursor < 0 || m.cursor >= len(m.items) {
		return
	}
	u := m.items[m.cursor]
	removesAccess := kind == actionDelete || u.Enabled
	if removesAccess && u.ID != "" && u.ID == m.currentUserID() {
		m.notice = fmt.Sprintf("Refusing to %s %s: it is the user this session is authenticated as.",
			actionVerb(kind, u), userLabel(u))
		return
	}
	m.notice = ""
	m.pending = &pendingAction{kind: kind, user: u}
}

// currentUserID returns the authenticated user's ID from the Keystone token,
// or "" when it cannot be determined.
func (m Model) currentUserID() string {
	if m.providerClient == nil {
		return ""
	}
	result, ok := m.providerClient.GetAuthResult().(interface {
		ExtractUser() (*tokens.User, error)
	})
	if !ok {
		return ""
	}
	user, err := result.ExtractUser()
	if err != nil || user == nil {
		return ""
	}
	return user.ID
}

func (m *Model) setItems(items []compute.User) {
	m.items = items
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
}

func actionVerb(kind actionKind, u compute.User) string {
	switch {
	case kind == actionDelete:
		return "delete"
	case u.Enabled:
		return "disable"
	default:
		return "enable"
	}
}

func userLabel(u compute.User) string {
	if u.Name == "" {
		return u.ID
	}
	return fmt.Sprintf("%s (%s)", u.Name, u.ID)
}

func mutationNotice(msg userMutatedMsg) string {
	verb := actionVerb(msg.action, msg.user)
	if msg.err != nil {
		return fmt.Sprintf("Failed to %s %s: %v", verb, userLabel(msg.user), msg.err)
	}
	past := map[string]string{"delete": "Deleted", "disable": "Disabled", "enable": "Enabled"}[verb]
	notice := fmt.Sprintf("%s %s.", past, userLabel(msg.user))
	if msg.refreshErr != nil {
		notice += fmt.Sprintf(" Refreshing the user list failed: %v", msg.refreshErr)
	}
	return notice
}

// applyMutation updates items locally after a successful mutation.
func applyMutation(items []compute.User, msg userMutatedMsg) []compute.User {
	out := make([]compute.User, 0, len(items))
	for _, u := range items {
		if u.ID == msg.user.ID {
			if msg.action == actionDelete {
				continue
			}
			u.Enabled = !msg.user.Enabled
		}
		out = append(out, u)
	}
	return out
}

func (m Model) visibleRows() int {
	h := m.height - 6 // title + header + footer + status bar
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the user management list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("User Management")
	if m.loading || m.busy {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n")

	if m.pending != nil {
		u := m.pending.user
		prompt := fmt.Sprintf("  Really %s user %s? %s confirm • %s cancel ",
			actionVerb(m.pending.kind, u), userLabel(u),
			shared.Keys.Confirm.Help().Key, shared.Keys.Deny.Help().Key)
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(prompt) + "\n\n")
	}

	if m.notice != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorWarning).Render("  "+m.notice) + "\n\n")
	}

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.items) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No users found.") + "\n")
		return b.String()
	}

	// Header row
	header := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
	headerRow := fmt.Sprintf("  %-30s %-35s %-8s %-20s %-30s",
		"Name", "Email", "Enabled", "Domain", "Description")
	b.WriteString(header.Render(headerRow) + "\n")

	visible := m.visibleRows()
	end := m.scroll + visible
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.scroll; i < end; i++ {
		u := m.items[i]
		prefix := "  "
		if i == m.cursor {
			prefix = "▶ "
		}

		enabledStr := "No"
		if u.Enabled {
			enabledStr = "Yes"
		}

		row := fmt.Sprintf("%s%-30s %-35s %-8s %-20s %-30s",
			prefix,
			truncate(u.Name, 29),
			truncate(u.Email, 34),
			enabledStr,
			truncate(u.DomainID, 19),
			truncate(u.Description, 29),
		)

		style := lipgloss.NewStyle()
		if i == m.cursor {
			style = style.Background(shared.ColorHighlight).Foreground(shared.ColorFg)
		} else if !u.Enabled {
			style = style.Foreground(shared.ColorMuted)
		}
		rowStr := style.Render(row)
		if m.width > 0 && ansi.StringWidth(rowStr) > m.width {
			rowStr = ansi.Truncate(rowStr, m.width-1, "")
		}
		b.WriteString(rowStr + "\n")
	}

	// Footer
	b.WriteString("\n")
	footer := fmt.Sprintf("%d users — %s toggle • %s delete • %s back • %s refresh",
		len(m.items), shared.Keys.Enter.Help().Key, shared.Keys.Delete.Help().Key,
		shared.Keys.Back.Help().Key, shared.Keys.Refresh.Help().Key)
	b.WriteString(shared.StyleHelp.Render(footer))

	return b.String()
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.pending != nil {
		return fmt.Sprintf("%s confirm • %s/%s cancel",
			shared.Keys.Confirm.Help().Key, shared.Keys.Deny.Help().Key, shared.Keys.Back.Help().Key)
	}
	return fmt.Sprintf("↑↓ select • %s toggle • %s delete • %s back • %s refresh • ? help",
		shared.Keys.Enter.Help().Key, shared.Keys.Delete.Help().Key,
		shared.Keys.Back.Help().Key, shared.Keys.Refresh.Help().Key)
}

// ForceRefresh triggers a reload.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetch())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) fetch() tea.Cmd {
	pc := m.providerClient
	eo := m.endpointOpts
	return func() tea.Msg {
		items, err := compute.ListUsers(context.Background(), pc, eo)
		if err != nil {
			return usersErrMsg{err: err}
		}
		return usersLoadedMsg{items: items}
	}
}

// doAction performs a confirmed toggle or delete, then reloads the list.
func (m Model) doAction(p pendingAction) tea.Cmd {
	pc := m.providerClient
	eo := m.endpointOpts
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		switch p.kind {
		case actionToggle:
			err = compute.SetUserEnabled(ctx, pc, eo, p.user.ID, !p.user.Enabled)
		case actionDelete:
			err = compute.DeleteUser(ctx, pc, eo, p.user.ID)
		}
		msg := userMutatedMsg{action: p.kind, user: p.user, err: err}
		if err != nil {
			return msg
		}
		msg.items, msg.refreshErr = compute.ListUsers(ctx, pc, eo)
		return msg
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	return s[:n-1] + "…"
}
