package keypairlist

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/compute"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type keypairsLoadedMsg struct{ keypairs []compute.KeyPair }
type keypairsErrMsg struct{ err error }
type sortClearMsg struct{}
type tickMsg struct{}

var kpSortColumns = []string{"name", "type"}

// Model is the keypair list view.
type Model struct {
	client  *gophercloud.ServiceClient
	pairs   []compute.KeyPair
	cursor  int
	width   int
	height  int
	loading       bool
	spinner       spinner.Model
	err           string
	sortCol       int
	sortAsc       bool
	sortHighlight   bool
	sortClearAt     time.Time
	refreshInterval time.Duration
}

// New creates a keypair list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		sortAsc:         true,
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchKeypairs(), m.tickCmd())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case keypairsLoadedMsg:
		m.loading = false
		m.pairs = msg.keypairs
		m.err = ""
		m.sortPairs()
		return m, nil

	case keypairsErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchKeypairs(), m.tickCmd())

	case shared.TickMsg:
		return m, tea.Batch(m.fetchKeypairs(), m.tickCmd())

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

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Sort):
			m.sortCol = (m.sortCol + 1) % len(kpSortColumns)
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortPairs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortPairs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.pairs)-1 {
				m.cursor++
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.height - 5
			if m.cursor >= len(m.pairs) {
				m.cursor = len(m.pairs) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.height - 5
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
	}
	return m, nil
}

// View renders the keypair list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Key Pairs")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.pairs))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.pairs) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No key pairs found.") + "\n")
		return b.String()
	}

	headerTitles := []struct {
		title string
		width int
	}{
		{"Name", 30},
		{"Type", 10},
	}
	var headerParts []string
	for i, h := range headerTitles {
		title := h.title
		indicator := ""
		if i == m.sortCol {
			if m.sortAsc {
				indicator = " ▲"
			} else {
				indicator = " ▼"
			}
		}
		if i == m.sortCol && m.sortHighlight {
			headerParts = append(headerParts, lipgloss.NewStyle().
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		} else {
			headerParts = append(headerParts, shared.StyleHeader.Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		}
	}
	b.WriteString("  " + strings.Join(headerParts, " ") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width)) + "\n")

	for i, kp := range m.pairs {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			cursor = "▸ "
			style = style.Background(lipgloss.Color("#073642")).Bold(true)
		}

		line := fmt.Sprintf("%-30s %-10s", kp.Name, kp.Type)
		b.WriteString(cursor + style.Render(line) + "\n")
	}

	return b.String()
}

func (m *Model) sortPairs() {
	if len(m.pairs) == 0 {
		return
	}
	colKey := kpSortColumns[m.sortCol]
	asc := m.sortAsc
	sort.SliceStable(m.pairs, func(i, j int) bool {
		a, b := m.pairs[i], m.pairs[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "type":
			less = strings.ToLower(a.Type) < strings.ToLower(b.Type)
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

// SelectedKeyPair returns the keypair under the cursor.
func (m Model) SelectedKeyPair() *compute.KeyPair {
	if m.cursor >= 0 && m.cursor < len(m.pairs) {
		kp := m.pairs[m.cursor]
		return &kp
	}
	return nil
}

func (m Model) fetchKeypairs() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		kps, err := compute.ListKeyPairs(context.Background(), client)
		if err != nil {
			return keypairsErrMsg{err: err}
		}
		return keypairsLoadedMsg{keypairs: kps}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the keypair list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchKeypairs())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • enter detail • ^n create • ^d delete • R refresh • 1-5/←→ switch tab • ? help"
}
