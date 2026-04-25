package imagedownload

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const (
	fieldPath   = 0
	fieldSubmit = 1
	fieldCancel = 2
	numFields   = 3
)

type downloadDoneMsg struct{ name string }
type downloadErrMsg struct{ err error }
type progressTickMsg struct{}

// countingReader wraps a reader and atomically tracks bytes read.
type countingReader struct {
	reader  io.Reader
	counter *atomic.Int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.reader.Read(p)
	cr.counter.Add(int64(n))
	return n, err
}

type pickerDirEntry struct {
	name  string
	path  string
	isDir bool
}

func scheduleProgressTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return progressTickMsg{}
	})
}

// Model is the image download modal.
type Model struct {
	Active    bool
	client    *gophercloud.ServiceClient
	imageID   string
	imageName string

	pathInput    textinput.Model
	focusField   int
	defaultFile  string // default filename for save-here

	// Directory picker
	pickerOpen    bool
	pickerDir     string
	pickerEntries []pickerDirEntry
	pickerCursor  int

	downloading     bool
	sharedBytesRead *atomic.Int64
	sharedTotal     *atomic.Int64
	bytesRead       int64
	totalBytes      int64

	spinner spinner.Model
	width   int
	height  int
	err     string
}

// New creates an image download modal.
func New(client *gophercloud.ServiceClient, imageID, imageName, diskFormat string) Model {
	pi := textinput.New()
	pi.Prompt = ""

	// Default path: cwd/imagename.format
	ext := diskFormat
	if ext == "" {
		ext = "img"
	}
	cwd, _ := os.Getwd()
	defaultFile := fmt.Sprintf("%s.%s", sanitizeFilename(imageName), ext)
	defaultPath := filepath.Join(cwd, defaultFile)

	pi.SetValue(defaultPath)
	pi.CharLimit = 512
	pi.SetWidth(50)
	pi.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:      true,
		client:      client,
		imageID:     imageID,
		imageName:   imageName,
		defaultFile: defaultFile,
		pathInput:   pi,
		spinner:     s,
	}
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

// Init returns initial commands.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[imagedownload] Init() imageID=%s imageName=%q", m.imageID, m.imageName)
	return m.pathInput.Focus()
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressTickMsg:
		if m.sharedBytesRead != nil {
			m.bytesRead = m.sharedBytesRead.Load()
		}
		if m.sharedTotal != nil {
			m.totalBytes = m.sharedTotal.Load()
		}
		if m.downloading {
			return m, scheduleProgressTick()
		}
		return m, nil

	case downloadDoneMsg:
		m.Active = false
		m.downloading = false
		shared.Debugf("[imagedownload] download success name=%q", msg.name)
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Downloaded image", Name: msg.name}
		}

	case downloadErrMsg:
		m.downloading = false
		m.err = msg.err.Error()
		shared.Debugf("[imagedownload] error: %v", msg.err)
		return m, nil

	case spinner.TickMsg:
		if m.downloading {
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
		if m.downloading {
			if msg.String() == "b" {
				m.Active = false
				return m, nil
			}
			return m, nil
		}
		if m.pickerOpen {
			return m.handlePickerKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.focusField == fieldPath {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.focusField = fieldSubmit
			m.pathInput.Blur()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focusField = fieldCancel
			m.pathInput.Blur()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			// If value is a directory, open picker
			p := strings.TrimSpace(m.pathInput.Value())
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return m.openDirPicker(p)
			}
			return m.submit()
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			var cmd tea.Cmd
			m.pathInput, cmd = m.pathInput.Update(msg)
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.focusField = (m.focusField + 1) % numFields
		m.updateFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.focusField = (m.focusField - 1 + numFields) % numFields
		m.updateFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.Right):
		if m.focusField == fieldSubmit {
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		if m.focusField == fieldCancel {
			m.focusField = fieldSubmit
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		}
		return m, nil
	case msg.String() == "ctrl+s":
		return m.submit()
	}
	return m, nil
}

func (m *Model) updateFocus() {
	m.pathInput.Blur()
	if m.focusField == fieldPath {
		m.pathInput.Focus()
	}
}

// --- Directory picker ---

func (m Model) openDirPicker(dir string) (Model, tea.Cmd) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = "Cannot read directory: " + err.Error()
		return m, nil
	}

	var dirs []pickerDirEntry

	if dir != "/" {
		dirs = append(dirs, pickerDirEntry{name: "..", path: filepath.Dir(dir), isDir: true})
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, pickerDirEntry{
			name:  e.Name() + "/",
			path:  filepath.Join(dir, e.Name()),
			isDir: true,
		})
	}

	m.pickerEntries = dirs
	m.pickerDir = dir
	m.pickerOpen = true
	m.pickerCursor = 0
	m.err = ""
	return m, nil
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.pickerOpen = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Down):
		if m.pickerCursor < len(m.pickerEntries)-1 {
			m.pickerCursor++
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if m.pickerCursor < 0 || m.pickerCursor >= len(m.pickerEntries) {
			return m, nil
		}
		entry := m.pickerEntries[m.pickerCursor]
		if entry.name == ".." {
			return m.openDirPicker(entry.path)
		}
		// Selected a directory — save here
		m.pathInput.SetValue(filepath.Join(entry.path, m.defaultFile))
		m.pickerOpen = false
		return m, nil
	}
	return m, nil
}

func (m Model) renderDirPicker() string {
	title := shared.StyleModalTitle.Render("Download Image \u2014 Choose Directory")

	dirStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	var rows []string
	rows = append(rows, dirStyle.Render("Save to: "+m.pickerDir+"/"+m.defaultFile))
	rows = append(rows, "")

	maxShow := 15
	start := 0
	if m.pickerCursor >= maxShow {
		start = m.pickerCursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(m.pickerEntries) {
		end = len(m.pickerEntries)
	}

	for i := start; i < end; i++ {
		e := m.pickerEntries[i]
		cursor := "  "
		if i == m.pickerCursor {
			cursor = "\u25b8 "
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorCyan)
		if i == m.pickerCursor {
			nameStyle = nameStyle.Bold(true).Foreground(shared.ColorHighlight)
		}

		rows = append(rows, cursor+nameStyle.Render(e.name))
	}

	if len(m.pickerEntries) == 0 {
		rows = append(rows, shared.StyleHelp.Render("  No subdirectories"))
	}

	rows = append(rows, "")
	rows = append(rows, shared.StyleHelp.Render("\u2191\u2193 navigate \u2022 enter save here \u2022 esc back"))

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) submit() (Model, tea.Cmd) {
	path := strings.TrimSpace(m.pathInput.Value())
	if path == "" {
		m.err = "File path is required"
		return m, nil
	}

	if _, err := os.Stat(path); err == nil {
		m.err = "File already exists: " + path
		return m, nil
	}

	m.downloading = true
	m.err = ""
	shared.Debugf("[imagedownload] download start imageID=%s path=%s", m.imageID, path)

	sharedBytes := &atomic.Int64{}
	sharedTotal := &atomic.Int64{}
	m.sharedBytesRead = sharedBytes
	m.sharedTotal = sharedTotal
	m.bytesRead = 0
	m.totalBytes = 0

	client := m.client
	imageID := m.imageID
	name := m.imageName
	return m, tea.Batch(m.spinner.Tick, scheduleProgressTick(), func() tea.Msg {
		ctx := context.Background()

		body, contentLength, err := image.DownloadImageData(ctx, client, imageID)
		if err != nil {
			return downloadErrMsg{err: err}
		}
		defer body.Close()
		if contentLength > 0 {
			sharedTotal.Store(contentLength)
		}

		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return downloadErrMsg{err: fmt.Errorf("creating file: %w", err)}
		}
		defer f.Close()

		reader := &countingReader{reader: body, counter: sharedBytes}

		_, err = io.Copy(f, reader)
		if err != nil {
			os.Remove(path)
			return downloadErrMsg{err: fmt.Errorf("writing file: %w", err)}
		}

		return downloadDoneMsg{name: name}
	})
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.downloading {
		return "b background \u2022 downloading..."
	}
	return "ctrl+s download \u2022 esc cancel"
}

// View renders the modal.
func (m Model) View() string {
	if m.pickerOpen {
		return m.renderDirPicker()
	}
	if m.downloading {
		return m.renderProgress()
	}

	title := shared.StyleModalTitle.Render("Download Image")

	labelW := 12
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(labelW)

	var rows []string
	rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorFg).Render(
		fmt.Sprintf("Image: %s", m.imageName)))
	rows = append(rows, "")

	pathLabel := labelStyle.Render("Save to")
	if m.focusField == fieldPath {
		pathLabel = focusStyle.Render("Save to")
	}
	rows = append(rows, pathLabel+m.pathInput.View())

	if m.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err))
	}

	rows = append(rows, "")
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
	}
	rows = append(rows, submitStyle.Render("[ctrl+s] Download")+"  "+cancelStyle.Render("[esc] Cancel"))

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderProgress() string {
	title := shared.StyleModalTitle.Render("Downloading Image")

	var rows []string
	rows = append(rows, fmt.Sprintf("Image: %s", m.imageName))
	rows = append(rows, "")

	barWidth := m.formWidth() - 12
	if barWidth < 20 {
		barWidth = 20
	}

	if m.totalBytes > 0 {
		pct := int(float64(m.bytesRead) * 100 / float64(m.totalBytes))
		if pct > 100 {
			pct = 100
		}
		filled := barWidth * pct / 100
		bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)
		barStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		rows = append(rows, barStyle.Render(bar))
		rows = append(rows, fmt.Sprintf("%d%%  %s / %s",
			pct, shared.FormatSize(m.bytesRead), shared.FormatSize(m.totalBytes)))
	} else {
		rows = append(rows, m.spinner.View()+" "+shared.FormatSize(m.bytesRead)+" downloaded...")
	}

	rows = append(rows, "")
	rows = append(rows, shared.StyleHelp.Render("b send to background"))

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) formWidth() int {
	if m.width <= 0 {
		return 60
	}
	w := m.width - 6
	if w > 72 {
		w = 72
	}
	if w < 48 {
		w = 48
	}
	return w
}
