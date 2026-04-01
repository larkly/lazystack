package keypairdetail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type keypairLoadedMsg struct{ kp *compute.KeyPairFull }
type keypairErrMsg struct{ err error }

// Model is the keypair detail view.
type Model struct {
	client  *gophercloud.ServiceClient
	name    string
	kp      *compute.KeyPairFull
	scroll  int
	loading bool
	spinner spinner.Model
	err     string
	width   int
	height  int
}

// New creates a keypair detail view.
func New(client *gophercloud.ServiceClient, name string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:  client,
		name:    name,
		loading: true,
		spinner: s,
	}
}

// Init fetches the keypair.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[keypairdetail] Init() name=%q", m.name)
	return tea.Batch(m.spinner.Tick, m.fetchKeypair())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case keypairLoadedMsg:
		m.loading = false
		m.kp = msg.kp
		shared.Debugf("[keypairdetail] loaded keypair %q", m.name)
		return m, nil
	case keypairErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		shared.Debugf("[keypairdetail] error: %v", msg.err)
		return m, nil
	case spinner.TickMsg:
		if m.loading {
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
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "keypairlist"}
			}
		case key.Matches(msg, shared.Keys.Down):
			m.scroll++
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		}
	}
	return m, nil
}

// View renders the keypair detail.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Key Pair: " + m.name)
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.kp == nil {
		return b.String()
	}

	labelStyle := lipgloss.NewStyle().Width(14).Foreground(shared.ColorSecondary)
	valStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	props := []struct{ label, value string }{
		{"Name", m.kp.Name},
		{"Type", m.kp.Type},
	}

	for _, p := range props {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(p.label), valStyle.Render(p.value)))
	}

	b.WriteString("\n")
	b.WriteString("  " + labelStyle.Render("Public Key") + "\n")

	lines := strings.Split(m.kp.PublicKey, "\n")
	viewHeight := m.height - 12
	if viewHeight < 3 {
		viewHeight = 3
	}
	maxScroll := len(lines) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	end := m.scroll + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	keyStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	for _, line := range lines[m.scroll:end] {
		b.WriteString("  " + keyStyle.Render(line) + "\n")
	}

	return b.String()
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ scroll • ^d delete • esc back • ? help"
}

// KeyPairName returns the name of the displayed keypair.
func (m Model) KeyPairName() string {
	return m.name
}

func (m Model) fetchKeypair() tea.Cmd {
	client := m.client
	name := m.name
	return func() tea.Msg {
		kp, err := compute.GetKeyPair(context.Background(), client, name)
		if err != nil {
			return keypairErrMsg{err: err}
		}
		return keypairLoadedMsg{kp: kp}
	}
}
