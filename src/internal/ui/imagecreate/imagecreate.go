package imagecreate

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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
	fieldSource     = 0
	fieldName       = 1
	fieldPath       = 2
	fieldDiskFormat = 3
	fieldVisibility = 4
	fieldMinDisk    = 5
	fieldMinRAM     = 6
	fieldSubmit     = 7
	fieldCancel     = 8
	numFields       = 9
)

var (
	sourceOpts     = []string{"Local File", "URL"}
	diskFormatOpts = []string{"qcow2", "raw", "vmdk", "vdi", "iso", "ami"}
	visibilityOpts = []string{"private", "public", "shared", "community"}
)

// Messages
type progressTickMsg struct{}
type uploadDoneMsg struct{ name string }
type uploadErrMsg struct{ err error }
type importStartedMsg struct{ name string }

// Model is the image upload modal.
type Model struct {
	Active   bool
	client   *gophercloud.ServiceClient

	source       int
	nameInput    textinput.Model
	pathInput    textinput.Model
	diskFormat   int
	visibility   int
	minDiskInput textinput.Model
	minRAMInput  textinput.Model

	focusField int
	submitting bool
	uploading  bool
	imageName  string

	// Progress tracking via shared atomics
	sharedBytesRead *atomic.Int64
	sharedTotal     int64
	bytesRead       int64
	totalBytes      int64

	// Large file warning
	warnLargeFile bool
	largeFileSize int64

	spinner spinner.Model
	width   int
	height  int
	err     string
}

// New creates an image upload modal.
func New(client *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "image name"
	ni.CharLimit = 128
	ni.SetWidth(40)

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "/path/to/image.qcow2"
	pi.CharLimit = 512
	pi.SetWidth(40)

	mdi := textinput.New()
	mdi.Prompt = ""
	mdi.Placeholder = "0"
	mdi.CharLimit = 6
	mdi.SetWidth(8)

	mri := textinput.New()
	mri.Prompt = ""
	mri.Placeholder = "0"
	mri.CharLimit = 8
	mri.SetWidth(8)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:       true,
		client:       client,
		nameInput:    ni,
		pathInput:    pi,
		minDiskInput: mdi,
		minRAMInput:  mri,
		spinner:      s,
	}
}

// Init returns initial commands.
func (m Model) Init() tea.Cmd {
	m.focusField = fieldName
	return m.nameInput.Focus()
}

func (m Model) isTextInput() bool {
	switch m.focusField {
	case fieldName, fieldPath, fieldMinDisk, fieldMinRAM:
		return true
	}
	return false
}

func (m Model) pathLabel() string {
	if m.source == 0 {
		return "File Path"
	}
	return "URL"
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressTickMsg:
		if m.sharedBytesRead != nil {
			m.bytesRead = m.sharedBytesRead.Load()
			m.totalBytes = m.sharedTotal
		}
		if m.uploading {
			return m, scheduleProgressTick()
		}
		return m, nil

	case uploadDoneMsg:
		m.Active = false
		m.uploading = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Uploaded image", Name: msg.name}
		}

	case uploadErrMsg:
		m.uploading = false
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case importStartedMsg:
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Import started for image", Name: msg.name}
		}

	case spinner.TickMsg:
		if m.submitting || m.uploading {
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
		if m.uploading {
			// During upload, only allow background/cancel
			if msg.String() == "b" {
				// Send to background
				m.Active = false
				return m, nil
			}
			return m, nil
		}
		if m.submitting {
			return m, nil
		}
		if m.warnLargeFile {
			return m.handleLargeFileWarning(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleLargeFileWarning(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.warnLargeFile = false
		return m.doUpload()
	case "n", "esc":
		m.warnLargeFile = false
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focusField = (m.focusField - 1 + numFields) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			var cmd tea.Cmd
			switch m.focusField {
			case fieldName:
				m.nameInput, cmd = m.nameInput.Update(msg)
			case fieldPath:
				m.pathInput, cmd = m.pathInput.Update(msg)
			case fieldMinDisk:
				m.minDiskInput, cmd = m.minDiskInput.Update(msg)
			case fieldMinRAM:
				m.minRAMInput, cmd = m.minRAMInput.Update(msg)
			}
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
		switch m.focusField {
		case fieldSource:
			m.source = (m.source + 1) % len(sourceOpts)
			if m.source == 0 {
				m.pathInput.Placeholder = "/path/to/image.qcow2"
			} else {
				m.pathInput.Placeholder = "https://example.com/image.qcow2"
			}
		case fieldDiskFormat:
			m.diskFormat = (m.diskFormat + 1) % len(diskFormatOpts)
		case fieldVisibility:
			m.visibility = (m.visibility + 1) % len(visibilityOpts)
		case fieldSubmit:
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldSource:
			m.source = (m.source - 1 + len(sourceOpts)) % len(sourceOpts)
			if m.source == 0 {
				m.pathInput.Placeholder = "/path/to/image.qcow2"
			} else {
				m.pathInput.Placeholder = "https://example.com/image.qcow2"
			}
		case fieldDiskFormat:
			m.diskFormat = (m.diskFormat - 1 + len(diskFormatOpts)) % len(diskFormatOpts)
		case fieldVisibility:
			m.visibility = (m.visibility - 1 + len(visibilityOpts)) % len(visibilityOpts)
		case fieldCancel:
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
		default:
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
		}
		return m, nil
	case msg.String() == "ctrl+s":
		return m.submit()
	}
	return m, nil
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.pathInput.Blur()
	m.minDiskInput.Blur()
	m.minRAMInput.Blur()

	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldPath:
		m.pathInput.Focus()
	case fieldMinDisk:
		m.minDiskInput.Focus()
	case fieldMinRAM:
		m.minRAMInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Name is required"
		return m, nil
	}
	path := strings.TrimSpace(m.pathInput.Value())
	if path == "" {
		if m.source == 0 {
			m.err = "File path is required"
		} else {
			m.err = "URL is required"
		}
		return m, nil
	}

	m.imageName = name
	m.err = ""

	if m.source == 0 {
		// Local file: check existence and size
		info, err := os.Stat(path)
		if err != nil {
			m.err = "File not found: " + path
			return m, nil
		}
		if info.IsDir() {
			m.err = "Path is a directory, not a file"
			return m, nil
		}
		// Warn if > 10GB
		if info.Size() > 10*1024*1024*1024 {
			m.warnLargeFile = true
			m.largeFileSize = info.Size()
			return m, nil
		}
		return m.doUpload()
	}

	// URL import
	return m.doURLImport()
}

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

func (m Model) doUpload() (Model, tea.Cmd) {
	m.submitting = true
	m.uploading = true
	m.bytesRead = 0

	name := strings.TrimSpace(m.nameInput.Value())
	path := strings.TrimSpace(m.pathInput.Value())
	diskFmt := diskFormatOpts[m.diskFormat]
	vis := visibilityOpts[m.visibility]
	minDisk := parseIntOr(m.minDiskInput.Value(), 0)
	minRAM := parseIntOr(m.minRAMInput.Value(), 0)

	// Get file size for progress tracking
	info, _ := os.Stat(path)
	m.totalBytes = info.Size()
	m.sharedTotal = info.Size()

	// Shared atomic counter — goroutine increments, tick reads
	sharedBytes := &atomic.Int64{}
	m.sharedBytesRead = sharedBytes

	client := m.client
	return m, tea.Batch(m.spinner.Tick, scheduleProgressTick(), func() tea.Msg {
		ctx := context.Background()

		img, err := image.CreateImage(ctx, client, image.CreateImageOpts{
			Name:       name,
			DiskFormat: diskFmt,
			Visibility: vis,
			MinDisk:    minDisk,
			MinRAM:     minRAM,
		})
		if err != nil {
			return uploadErrMsg{err: err}
		}

		f, err := os.Open(path)
		if err != nil {
			_ = image.DeleteImage(ctx, client, img.ID)
			return uploadErrMsg{err: fmt.Errorf("opening file: %w", err)}
		}
		defer f.Close()

		pr := &countingReader{reader: f, counter: sharedBytes}

		err = image.UploadImageData(ctx, client, img.ID, pr)
		if err != nil {
			_ = image.DeleteImage(ctx, client, img.ID)
			return uploadErrMsg{err: err}
		}

		return uploadDoneMsg{name: name}
	})
}

func (m Model) doURLImport() (Model, tea.Cmd) {
	m.submitting = true

	name := strings.TrimSpace(m.nameInput.Value())
	url := strings.TrimSpace(m.pathInput.Value())
	diskFmt := diskFormatOpts[m.diskFormat]
	vis := visibilityOpts[m.visibility]
	minDisk := parseIntOr(m.minDiskInput.Value(), 0)
	minRAM := parseIntOr(m.minRAMInput.Value(), 0)

	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx := context.Background()

		img, err := image.CreateImage(ctx, client, image.CreateImageOpts{
			Name:       name,
			DiskFormat: diskFmt,
			Visibility: vis,
			MinDisk:    minDisk,
			MinRAM:     minRAM,
		})
		if err != nil {
			return uploadErrMsg{err: err}
		}

		err = image.ImportImageURL(ctx, client, img.ID, url)
		if err != nil {
			_ = image.DeleteImage(ctx, client, img.ID)
			return uploadErrMsg{err: err}
		}

		return importStartedMsg{name: name}
	})
}

func parseIntOr(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.uploading {
		return "b background • esc cancel"
	}
	return "tab/↑↓ navigate • ←→ cycle • ctrl+s submit • esc cancel"
}

// View renders the modal.
func (m Model) View() string {
	if m.uploading {
		return m.renderProgress()
	}

	title := shared.StyleModalTitle.Render("Upload Image")

	labelW := 12
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(labelW)

	label := func(name string, field int) string {
		if m.focusField == field {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	cycleDisplay := func(opts []string, idx int) string {
		var parts []string
		for i, o := range opts {
			if i == idx {
				parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render(o))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(o))
			}
		}
		return strings.Join(parts, " / ")
	}

	var rows []string
	rows = append(rows, label("Source", fieldSource)+cycleDisplay(sourceOpts, m.source))
	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	rows = append(rows, label(m.pathLabel(), fieldPath)+m.pathInput.View())
	rows = append(rows, label("Disk Format", fieldDiskFormat)+cycleDisplay(diskFormatOpts, m.diskFormat))
	rows = append(rows, label("Visibility", fieldVisibility)+cycleDisplay(visibilityOpts, m.visibility))
	rows = append(rows, label("Min Disk", fieldMinDisk)+m.minDiskInput.View()+" GB")
	rows = append(rows, label("Min RAM", fieldMinRAM)+m.minRAMInput.View()+" MB")

	if m.warnLargeFile {
		rows = append(rows, "")
		warnStyle := lipgloss.NewStyle().Foreground(shared.ColorWarning).Bold(true)
		rows = append(rows, warnStyle.Render(fmt.Sprintf("\u26a0 File is %s \u2014 continue? (y/n)",
			shared.FormatSize(m.largeFileSize))))
	}

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

	if m.submitting {
		if m.source == 0 {
			rows = append(rows, m.spinner.View()+" Creating image and uploading...")
		} else {
			rows = append(rows, m.spinner.View()+" Creating image and importing...")
		}
	} else {
		rows = append(rows, submitStyle.Render("[ctrl+s] Submit")+"  "+cancelStyle.Render("[esc] Cancel"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderProgress() string {
	title := shared.StyleModalTitle.Render("Uploading Image")

	var rows []string
	rows = append(rows, fmt.Sprintf("Image: %s", m.imageName))
	rows = append(rows, "")

	barWidth := m.formWidth() - 12
	if barWidth < 20 {
		barWidth = 20
	}

	var pct int
	if m.totalBytes > 0 {
		pct = int(float64(m.bytesRead) * 100 / float64(m.totalBytes))
		if pct > 100 {
			pct = 100
		}
	}
	filled := barWidth * pct / 100

	bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)
	barStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	rows = append(rows, barStyle.Render(bar))
	rows = append(rows, fmt.Sprintf("%d%%  %s / %s",
		pct, shared.FormatSize(m.bytesRead), shared.FormatSize(m.totalBytes)))

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
