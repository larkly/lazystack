package imagecreate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
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

var imageExtensions = map[string]bool{
	".qcow2": true, ".raw": true, ".vmdk": true, ".vdi": true,
	".iso": true, ".img": true, ".ami": true,
	".gz": true, ".bz2": true, ".xz": true,
}

type pickerEntry struct {
	name  string
	path  string
	isDir bool
	size  int64
}

// Messages
type progressTickMsg struct{}
type uploadDoneMsg struct {
	shared.Audit
	name string
}
type uploadErrMsg struct {
	shared.Audit
	err error
}
type importStartedMsg struct {
	shared.Audit
	name string
}

// Model is the image upload modal.
type Model struct {
	Active bool
	client *gophercloud.ServiceClient

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

	// File picker
	pickerOpen    bool
	pickerDir     string
	pickerEntries []pickerEntry
	pickerCursor  int

	// Progress tracking: the upload goroutine publishes its reader, the
	// progress tick reads its byte count.
	sharedUpload *atomic.Pointer[image.UploadReader]
	sharedTotal  int64
	bytesRead    int64
	totalBytes   int64

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
	ni.Focus()

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "/path/to/image.qcow2 (enter to browse)"
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
		focusField:   fieldName,
	}
}

// Init returns initial commands. Focus is set up in New: Init has a value
// receiver, so state changed here would be lost.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[imagecreate] Init()")
	return textinput.Blink
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
		if m.sharedUpload != nil {
			if ur := m.sharedUpload.Load(); ur != nil {
				m.bytesRead = ur.BytesRead()
			}
			m.totalBytes = m.sharedTotal
		}
		if m.uploading {
			return m, scheduleProgressTick()
		}
		return m, nil

	case uploadDoneMsg:
		m.Active = false
		m.uploading = false
		shared.Debugf("[imagecreate] upload success name=%q", msg.name)
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Uploaded image", Name: msg.name}
		}

	case uploadErrMsg:
		m.uploading = false
		m.submitting = false
		m.err = msg.err.Error()
		shared.Debugf("[imagecreate] error: %v", msg.err)
		return m, nil

	case importStartedMsg:
		m.Active = false
		shared.Debugf("[imagecreate] import started name=%q", msg.name)
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
		if m.pickerOpen {
			return m.handlePickerKey(msg)
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
			// On path field, open file picker for empty/directory values
			if m.focusField == fieldPath && m.source == 0 {
				p := expandHome(strings.TrimSpace(m.pathInput.Value()))
				if p == "" {
					if cwd, err := os.Getwd(); err == nil {
						return m.openPicker(cwd)
					}
					if home, err := os.UserHomeDir(); err == nil {
						return m.openPicker(home)
					}
				} else if info, err := os.Stat(p); err == nil && info.IsDir() {
					return m.openPicker(p)
				}
			}
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
				m.pathInput.Placeholder = "/path/to/image.qcow2 (enter to browse)"
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
				m.pathInput.Placeholder = "/path/to/image.qcow2 (enter to browse)"
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
	path := strings.TrimSpace(m.pathInput.Value())
	var importURL *url.URL
	if m.source != 0 && path != "" {
		u, err := validateImportURL(path)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		importURL = u
	}
	// Auto-fill name from filename if empty
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" && path != "" {
		if importURL != nil {
			name = urlImageName(importURL)
		} else {
			name = baseImageName(filepath.Base(path))
		}
		m.nameInput.SetValue(name)
	}
	if name == "" {
		m.err = "Name is required"
		return m, nil
	}
	if path == "" {
		if m.source == 0 {
			m.err = "File path is required"
		} else {
			m.err = "URL is required"
		}
		return m, nil
	}
	if _, _, err := m.minimums(); err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.imageName = name
	m.err = ""
	shared.Debugf("[imagecreate] submit name=%q format=%s", name, diskFormatOpts[m.diskFormat])

	if m.source == 0 {
		// Local file: check existence and size
		path = expandHome(path)
		m.pathInput.SetValue(path)
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

// --- File picker ---

func (m Model) openPicker(dir string) (Model, tea.Cmd) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = "Cannot read directory: " + err.Error()
		return m, nil
	}

	var dirs, files []pickerEntry

	// Add parent directory unless at root
	if dir != "/" {
		dirs = append(dirs, pickerEntry{name: "..", path: filepath.Dir(dir), isDir: true})
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // skip hidden files
		}
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			dirs = append(dirs, pickerEntry{name: e.Name() + "/", path: full, isDir: true})
		} else {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			// Also check double extension like .qcow2.gz
			nameNoExt := strings.TrimSuffix(e.Name(), ext)
			ext2 := strings.ToLower(filepath.Ext(nameNoExt))
			if imageExtensions[ext] || imageExtensions[ext2] {
				info, _ := e.Info()
				var size int64
				if info != nil {
					size = info.Size()
				}
				files = append(files, pickerEntry{name: e.Name(), path: full, size: size})
			}
		}
	}

	sort.Slice(dirs[1:], func(i, j int) bool { // skip ".." for sort
		return strings.ToLower(dirs[i+1].name) < strings.ToLower(dirs[j+1].name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].name) < strings.ToLower(files[j].name)
	})

	m.pickerEntries = append(dirs, files...)
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
	case key.Matches(msg, shared.Keys.PageUp):
		m.pickerCursor -= 10
		if m.pickerCursor < 0 {
			m.pickerCursor = 0
		}
		return m, nil
	case key.Matches(msg, shared.Keys.PageDown):
		m.pickerCursor += 10
		if m.pickerCursor >= len(m.pickerEntries) {
			m.pickerCursor = len(m.pickerEntries) - 1
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if m.pickerCursor < 0 || m.pickerCursor >= len(m.pickerEntries) {
			return m, nil
		}
		entry := m.pickerEntries[m.pickerCursor]
		if entry.isDir {
			return m.openPicker(entry.path)
		}
		// File selected
		m.pathInput.SetValue(entry.path)
		m.pickerOpen = false
		m.autoDetectDiskFormat(entry.name)
		// Auto-fill name if empty
		if strings.TrimSpace(m.nameInput.Value()) == "" {
			m.nameInput.SetValue(baseImageName(entry.name))
		}
		m.focusField = fieldDiskFormat
		m.updateFocus()
		return m, nil
	}
	return m, nil
}

// baseImageName returns the filename as the default image name.
func baseImageName(filename string) string {
	return filename
}

func (m *Model) autoDetectDiskFormat(filename string) {
	name := strings.ToLower(filename)
	// Strip compression extension first
	for _, ext := range []string{".gz", ".bz2", ".xz"} {
		name = strings.TrimSuffix(name, ext)
	}
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	for i, fmt := range diskFormatOpts {
		if fmt == ext {
			m.diskFormat = i
			return
		}
	}
}

func (m Model) renderPicker() string {
	title := shared.StyleModalTitle.Render("Upload Image")

	dirStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	var rows []string
	rows = append(rows, dirStyle.Render("Browsing: "+m.pickerDir))
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

	nameW := 30
	formW := m.formWidth() - 8
	if formW > 20 {
		nameW = formW - 12 // room for size column
	}

	for i := start; i < end; i++ {
		e := m.pickerEntries[i]
		cursor := "  "
		if i == m.pickerCursor {
			cursor = "\u25b8 "
		}

		name := e.name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		var sizeStr string
		if e.isDir {
			sizeStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("<dir>")
		} else {
			sizeStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(shared.FormatSize(e.size))
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if e.isDir {
			nameStyle = nameStyle.Foreground(shared.ColorCyan)
		}
		if i == m.pickerCursor {
			nameStyle = nameStyle.Bold(true).Foreground(shared.ColorHighlight)
		}

		row := cursor + nameStyle.Width(nameW).Render(name) + "  " + sizeStr
		rows = append(rows, row)
	}

	if len(m.pickerEntries) == 0 {
		rows = append(rows, shared.StyleHelp.Render("  No image files found"))
	}

	rows = append(rows, "")
	rows = append(rows, shared.StyleHelp.Render("\u2191\u2193 navigate \u2022 enter select \u2022 esc back"))

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func scheduleProgressTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return progressTickMsg{}
	})
}

func (m Model) doUpload() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	path := strings.TrimSpace(m.pathInput.Value())
	diskFmt := diskFormatOpts[m.diskFormat]
	vis := visibilityOpts[m.visibility]
	minDisk, minRAM, err := m.minimums()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}

	// The size seen here is the exact byte count the upload must deliver.
	info, err := os.Stat(path)
	if err != nil {
		m.err = "File not found: " + path
		return m, nil
	}
	size := info.Size()

	m.submitting = true
	m.uploading = true
	m.bytesRead = 0
	m.totalBytes = size
	m.sharedTotal = size

	// The goroutine publishes its reader; the progress tick reads from it.
	sharedUpload := &atomic.Pointer[image.UploadReader]{}
	m.sharedUpload = sharedUpload

	client := m.client
	return m, tea.Batch(m.spinner.Tick, scheduleProgressTick(), func() tea.Msg {
		createCtx, cancelCreate := shared.RequestCtx()
		img, err := image.CreateImage(createCtx, client, image.CreateImageOpts{
			Name:       name,
			DiskFormat: diskFmt,
			Visibility: vis,
			MinDisk:    minDisk,
			MinRAM:     minRAM,
		})
		cancelCreate()
		if err != nil {
			return uploadErrMsg{Audit: shared.NewAudit(audit.ActionUpload, "image", "", name, err), err: err}
		}
		// record describes the upload for the audit log.
		record := func(err error) shared.Audit {
			return shared.NewAudit(audit.ActionUpload, "image", img.ID, name, err).
				WithDetails(map[string]string{"disk_format": diskFmt, "visibility": vis})
		}

		f, err := os.Open(path)
		if err != nil {
			err = cleanupFailedImage(client, img.ID, fmt.Errorf("opening file: %w", err))
			return uploadErrMsg{Audit: record(err), err: err}
		}
		defer f.Close()

		ur := image.NewUploadReader(f, size)
		sharedUpload.Store(ur)

		// The data transfer may legitimately take hours, so it has no
		// overall deadline; it is abandoned only if it stops making
		// progress. (Waiting for the response once the body is sent is
		// bounded by the transport's response-header timeout.)
		ctx, cancel := shared.StallCtx(ur.BytesRead)
		defer cancel()
		err = image.UploadImageData(ctx, client, img.ID, ur)
		if err != nil {
			if errors.Is(context.Cause(ctx), shared.ErrTransferStalled) {
				err = fmt.Errorf("upload made no progress for %s: %w", shared.TransferStallTimeout, err)
			}
			err = cleanupFailedImage(client, img.ID, err)
			return uploadErrMsg{Audit: record(err), err: err}
		}

		return uploadDoneMsg{Audit: record(nil), name: name}
	})
}

func (m Model) doURLImport() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	url := strings.TrimSpace(m.pathInput.Value())
	diskFmt := diskFormatOpts[m.diskFormat]
	vis := visibilityOpts[m.visibility]
	minDisk, minRAM, err := m.minimums()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.submitting = true
	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()

		img, err := image.CreateImage(ctx, client, image.CreateImageOpts{
			Name:       name,
			DiskFormat: diskFmt,
			Visibility: vis,
			MinDisk:    minDisk,
			MinRAM:     minRAM,
		})
		if err != nil {
			return uploadErrMsg{Audit: shared.NewAudit(audit.ActionCreateImage, "image", "", name, err), err: err}
		}

		err = image.ImportImageURL(ctx, client, img.ID, url)
		if err != nil {
			err = cleanupFailedImage(client, img.ID, err)
		}
		rec := shared.NewAudit(audit.ActionCreateImage, "image", img.ID, name, err).
			WithDetails(map[string]string{"import_url": url, "disk_format": diskFmt, "visibility": vis})
		if err != nil {
			return uploadErrMsg{Audit: rec, err: err}
		}

		return importStartedMsg{Audit: rec, name: name}
	})
}

func expandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// cleanupFailedImage deletes an image whose data step failed. The primary
// error is always kept; a failed delete is appended with the image ID so the
// user knows which resource was left behind.
func cleanupFailedImage(client *gophercloud.ServiceClient, imageID string, primary error) error {
	// A fresh deadline: the failed step's context may already be done.
	ctx, cancel := shared.RequestCtx()
	defer cancel()
	if err := image.DeleteImage(ctx, client, imageID); err != nil {
		shared.Debugf("[imagecreate] cleanup of image %s failed: %v", imageID, err)
		return fmt.Errorf("%w (cleanup failed, image %s may be left behind: %v)", primary, imageID, err)
	}
	return primary
}

// minimums parses the Min Disk and Min RAM fields; blank means 0.
func (m Model) minimums() (disk, ram int, err error) {
	if disk, err = image.ParseMinimum("Min Disk", m.minDiskInput.Value()); err != nil {
		return 0, 0, err
	}
	if ram, err = image.ParseMinimum("Min RAM", m.minRAMInput.Value()); err != nil {
		return 0, 0, err
	}
	return disk, ram, nil
}

// validateImportURL checks a web-download source before any image is
// created. Glance's web-download method only accepts http and https URLs
// by default, and the URL is passed to it unchanged.
func validateImportURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" {
		return nil, fmt.Errorf("URL must be an absolute http:// or https:// address")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("URL scheme %q is not supported for import (use http or https)", u.Scheme)
	}
	return u, nil
}

// urlImageName derives a default image name from the last URL path segment,
// ignoring any query string or fragment.
func urlImageName(u *url.URL) string {
	base := path.Base(u.Path)
	if base == "/" || base == "." {
		return ""
	}
	return base
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
	if m.source == 0 && m.focusField == fieldPath {
		return "enter browse files • tab/↑↓ navigate • ctrl+s submit • esc cancel"
	}
	return "tab/↑↓ navigate • ←→ cycle • ctrl+s submit • esc cancel"
}

// View renders the modal.
func (m Model) View() string {
	if m.pickerOpen {
		return m.renderPicker()
	}
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
