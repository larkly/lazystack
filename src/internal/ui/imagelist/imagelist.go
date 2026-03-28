package imagelist

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type imagesLoadedMsg struct{ images []image.Image }
type imagesErrMsg struct{ err error }
type tickMsg struct{}
type sortClearMsg struct{}

// Column defines an image list column.
type Column struct {
	Title    string
	MinWidth int
	Flex     int
	Priority int
	Key      string
	width    int
	hidden   bool
}

func defaultColumns() []Column {
	return []Column{
		{Title: "Name", MinWidth: 10, Flex: 3, Priority: 0, Key: "name"},
		{Title: "Status", MinWidth: 12, Flex: 0, Priority: 0, Key: "status"},
		{Title: "Size", MinWidth: 8, Flex: 0, Priority: 0, Key: "size"},
		{Title: "Min Disk", MinWidth: 8, Flex: 0, Priority: 2, Key: "min_disk"},
		{Title: "Min RAM", MinWidth: 8, Flex: 0, Priority: 2, Key: "min_ram"},
		{Title: "Visibility", MinWidth: 10, Flex: 0, Priority: 1, Key: "visibility"},
		{Title: "Created", MinWidth: 12, Flex: 0, Priority: 1, Key: "created"},
	}
}

func computeWidths(columns []Column, totalWidth int) []Column {
	for i := range columns {
		columns[i].hidden = false
		columns[i].width = columns[i].MinWidth
	}

	maxPrio := 0
	for _, c := range columns {
		if c.Priority > maxPrio {
			maxPrio = c.Priority
		}
	}

	for prio := maxPrio; prio >= 0; prio-- {
		if fitsWidth(columns, totalWidth) {
			break
		}
		for i := range columns {
			if columns[i].Priority == prio {
				columns[i].hidden = true
			}
		}
	}

	gaps := -1
	totalMin := 0
	totalFlex := 0
	for _, c := range columns {
		if c.hidden {
			continue
		}
		gaps++
		totalMin += c.MinWidth
		totalFlex += c.Flex
	}
	if gaps < 0 {
		gaps = 0
	}

	available := totalWidth - 2 - gaps
	if available < 0 {
		available = 0
	}

	remaining := available - totalMin
	if remaining > 0 && totalFlex > 0 {
		for i := range columns {
			if columns[i].hidden || columns[i].Flex == 0 {
				continue
			}
			extra := remaining * columns[i].Flex / totalFlex
			columns[i].width += extra
		}
	}

	return columns
}

func fitsWidth(columns []Column, totalWidth int) bool {
	needed := 2
	first := true
	for _, c := range columns {
		if c.hidden {
			continue
		}
		if !first {
			needed++
		}
		first = false
		needed += c.MinWidth
	}
	return needed <= totalWidth
}

// Model is the image list view.
type Model struct {
	client          *gophercloud.ServiceClient
	images          []image.Image
	columns         []Column
	cursor          int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	scrollOff       int
	refreshInterval time.Duration
	selected        map[string]bool // selected image IDs for bulk actions
	sortCol         int
	sortAsc         bool
	sortHighlight   bool
	sortClearAt     time.Time
}

// New creates an image list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		columns:         defaultColumns(),
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
		selected:        make(map[string]bool),
		sortAsc:         true,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchImages(), m.tickCmd())
}

// SelectedImage returns the image under the cursor.
func (m Model) SelectedImage() *image.Image {
	if m.cursor >= 0 && m.cursor < len(m.images) {
		img := m.images[m.cursor]
		return &img
	}
	return nil
}

// SelectedImages returns all selected images, or the cursor image if none selected.
func (m Model) SelectedImages() []image.Image {
	if len(m.selected) > 0 {
		var result []image.Image
		for _, img := range m.images {
			if m.selected[img.ID] {
				result = append(result, img)
			}
		}
		return result
	}
	if img := m.SelectedImage(); img != nil {
		return []image.Image{*img}
	}
	return nil
}

// ClearSelection clears all selected images.
func (m *Model) ClearSelection() {
	m.selected = make(map[string]bool)
}

// SelectionCount returns the number of selected images.
func (m Model) SelectionCount() int {
	return len(m.selected)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imagesLoadedMsg:
		m.loading = false
		m.images = msg.images
		m.err = ""
		m.sortImages()
		return m, nil

	case imagesErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchImages(), m.tickCmd())

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
		m.columns = computeWidths(m.columns, m.width)
		return m, nil

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Sort):
			visibleCount := m.visibleColCount()
			if visibleCount > 0 {
				m.sortCol = (m.sortCol + 1) % visibleCount
				m.sortAsc = true
				m.sortHighlight = true
				m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
				m.sortImages()
				return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
					return sortClearMsg{}
				})
			}
		case key.Matches(msg, shared.Keys.ReverseSort):
			if m.visibleColCount() > 0 {
				m.sortAsc = !m.sortAsc
				m.sortHighlight = true
				m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
				m.sortImages()
				return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
					return sortClearMsg{}
				})
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.images)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			if m.cursor >= len(m.images) {
				m.cursor = len(m.images) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.tableHeight()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.Select):
			if img := m.SelectedImage(); img != nil {
				if m.selected[img.ID] {
					delete(m.selected, img.ID)
				} else {
					m.selected[img.ID] = true
				}
				if m.cursor < len(m.images)-1 {
					m.cursor++
					m.ensureVisible()
				}
			}
		case key.Matches(msg, shared.Keys.Back):
			if len(m.selected) > 0 {
				m.selected = make(map[string]bool)
			}
		}
	}
	return m, nil
}

func (m *Model) ensureVisible() {
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) tableHeight() int {
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) visibleColCount() int {
	n := 0
	for _, col := range m.columns {
		if !col.hidden {
			n++
		}
	}
	return n
}

func (m Model) visibleColKey(idx int) string {
	n := 0
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		if n == idx {
			return col.Key
		}
		n++
	}
	return ""
}

func (m *Model) sortImages() {
	if len(m.images) == 0 {
		return
	}
	colKey := m.visibleColKey(m.sortCol)
	if colKey == "" {
		return
	}
	asc := m.sortAsc
	sort.SliceStable(m.images, func(i, j int) bool {
		a, b := m.images[i], m.images[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "status":
			less = a.Status < b.Status
		case "size":
			less = a.Size < b.Size
		case "min_disk":
			less = a.MinDisk < b.MinDisk
		case "min_ram":
			less = a.MinRAM < b.MinRAM
		case "visibility":
			less = a.Visibility < b.Visibility
		case "created":
			less = a.CreatedAt.Before(b.CreatedAt)
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

// View renders the image list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Images")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.images))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.images) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No images found.") + "\n")
		return b.String()
	}

	// Header
	visIdx := 0
	header := m.renderRow(func(col Column) string {
		title := col.Title
		idx := visIdx
		visIdx++
		indicator := ""
		if idx == m.sortCol {
			if m.sortAsc {
				indicator = " ▲"
			} else {
				indicator = " ▼"
			}
		}
		if idx == m.sortCol && m.sortHighlight {
			return lipgloss.NewStyle().
				Width(col.width).
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(title + indicator)
		}
		return shared.StyleHeader.Width(col.width).Render(title + indicator)
	})
	b.WriteString(header + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width)) + "\n")

	th := m.tableHeight()
	end := m.scrollOff + th
	if end > len(m.images) {
		end = len(m.images)
	}

	for i := m.scrollOff; i < end; i++ {
		img := m.images[i]
		cursor := i == m.cursor

		name := img.Name
		if name == "" && len(img.ID) > 8 {
			name = img.ID[:8] + "..."
		}

		values := map[string]string{
			"name":       name,
			"status":     img.Status,
			"size":       formatSize(img.Size),
			"min_disk":   fmt.Sprintf("%dGB", img.MinDisk),
			"min_ram":    fmt.Sprintf("%dMB", img.MinRAM),
			"visibility": img.Visibility,
			"created":    img.CreatedAt.Format("2006-01-02"),
		}

		row := m.renderDataRow(values, img.Status, cursor, m.selected[img.ID])
		b.WriteString(row + "\n")
	}

	return b.String()
}

func (m Model) renderRow(render func(Column) string) string {
	var parts []string
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		parts = append(parts, render(col))
	}
	return "  " + strings.Join(parts, " ")
}

func (m Model) renderDataRow(values map[string]string, status string, cursor bool, selected bool) string {
	var rowBg color.Color
	hasBg := false
	if cursor {
		rowBg = lipgloss.Color("#073642")
		hasBg = true
	} else if selected {
		rowBg = lipgloss.Color("#1a1a2e")
		hasBg = true
	}

	var parts []string
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		val := values[col.Key]
		w := col.width
		if len(val) > w && w > 1 {
			val = val[:w-1] + "…"
		}

		style := lipgloss.NewStyle().Width(w)
		if col.Key == "status" {
			style = imageStatusStyle(status).Width(w)
		}
		if cursor {
			style = style.Bold(true)
		}
		if hasBg {
			style = style.Background(rowBg)
		}

		parts = append(parts, style.Render(val))
	}

	prefix := "  "
	if selected {
		prefix = "● "
	}
	prefixStyle := lipgloss.NewStyle()
	gapStyle := lipgloss.NewStyle()
	if hasBg {
		prefixStyle = prefixStyle.Background(rowBg)
		gapStyle = gapStyle.Background(rowBg)
	}

	gap := gapStyle.Render(" ")
	row := prefixStyle.Render(prefix) + strings.Join(parts, gap)

	if hasBg {
		rowW := lipgloss.Width(row)
		if rowW < m.width {
			row += gapStyle.Render(strings.Repeat(" ", m.width-rowW))
		}
	}

	return row
}

func imageStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "active":
		fg = shared.ColorSuccess
	case "saving":
		fg = shared.ColorWarning
	case "queued", "importing":
		fg = shared.ColorCyan
	case "deactivated", "killed":
		fg = shared.ColorError
	case "deleted", "pending_delete":
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "-"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (m Model) fetchImages() tea.Cmd {
	client := m.client
	if client == nil {
		return func() tea.Msg {
			return imagesErrMsg{err: fmt.Errorf("image service not available")}
		}
	}
	return func() tea.Msg {
		imgs, err := image.ListImages(context.Background(), client)
		if err != nil {
			return imagesErrMsg{err: err}
		}
		return imagesLoadedMsg{images: imgs}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the image list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchImages())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.columns = computeWidths(m.columns, w)
}

// Hints returns key hints.
func (m Model) Hints() string {
	if len(m.selected) > 0 {
		return fmt.Sprintf("(%d selected) space toggle • ^d delete • d deactivate • esc clear • ? help", len(m.selected))
	}
	return "↑↓ navigate • space select • enter detail • ^d delete • d deactivate • R refresh • s/S sort • ? help"
}
