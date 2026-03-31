package imagedownload

import (
	"context"
	"fmt"
	"io"
	"os"
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

	pathInput  textinput.Model
	focusField int

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
	defaultPath := fmt.Sprintf("%s/%s.%s", cwd, sanitizeFilename(imageName), ext)

	pi.SetValue(defaultPath)
	pi.CharLimit = 512
	pi.SetWidth(50)
	pi.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:    true,
		client:    client,
		imageID:   imageID,
		imageName: imageName,
		pathInput: pi,
		spinner:   s,
	}
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

// Init returns initial commands.
func (m Model) Init() tea.Cmd {
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
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Downloaded image", Name: msg.name}
		}

	case downloadErrMsg:
		m.downloading = false
		m.err = msg.err.Error()
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

		f, err := os.Create(path)
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
