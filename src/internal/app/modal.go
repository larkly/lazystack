package app

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/imagecreate"
	"github.com/larkly/lazystack/internal/ui/imagedownload"
)

// overlay is the read side every modal/overlay shares: whether it is on
// screen, what it draws and which keys it offers in the status bar.
type overlay interface {
	IsActive() bool
	View() string
	Hints() string
}

// modalModel is the method set of a value-receiver overlay model.
type modalModel[T any] interface {
	Update(tea.Msg) (T, tea.Cmd)
	View() string
	Hints() string
}

// sizedModel is a pointer to a modal model, which is how SetSize is reached.
type sizedModel[T any] interface {
	*T
	SetSize(w, h int)
}

// modalAdapter adapts a modal model held in a Model field to overlay.
type modalAdapter[T modalModel[T], PT sizedModel[T]] struct {
	model  PT
	active func() bool
}

func (a modalAdapter[T, PT]) IsActive() bool { return a.active() }
func (a modalAdapter[T, PT]) View() string   { return (*a.model).View() }
func (a modalAdapter[T, PT]) Hints() string  { return (*a.model).Hints() }

// overlayRef is one entry of the overlay list: the overlay plus the hooks
// that write back to the Model field it lives in.
type overlayRef struct {
	name string
	overlay
	update  func(tea.Msg) tea.Cmd
	setSize func(w, h int)
	// keys reports whether updateAnyModal routes key presses here. The
	// confirm and error modals take keys through updateModal instead.
	keys bool
	// background reports whether updateAnyModalBackground routes non-key
	// messages here while the overlay is active.
	background bool
}

// newOverlayRef builds an overlay entry for the model behind p.
func newOverlayRef[T modalModel[T], PT sizedModel[T]](name string, p PT, active func() bool) overlayRef {
	return overlayRef{
		name:    name,
		overlay: modalAdapter[T, PT]{model: p, active: active},
		update: func(msg tea.Msg) tea.Cmd {
			var cmd tea.Cmd
			*p, cmd = (*p).Update(msg)
			return cmd
		},
		setSize:    p.SetSize,
		keys:       true,
		background: true,
	}
}

// flag returns an active check reading the given Active field.
func flag(active *bool) func() bool {
	return func() bool { return *active }
}

// overlays lists every modal/overlay in precedence order: when several are
// active, the first one is drawn, receives keys and background messages,
// and supplies the status-bar hints.
func (m *Model) overlays() []overlayRef {
	confirm := newOverlayRef("confirm", &m.confirm, func() bool { return m.activeModal == modalConfirm })
	confirm.keys, confirm.background = false, false
	errModal := newOverlayRef("errModal", &m.errModal, func() bool { return m.activeModal == modalError })
	errModal.keys, errModal.background = false, false
	// Image transfers take background messages through their own routes,
	// which run whether or not the modal is open.
	imageCreate := newOverlayRef("imageCreate", &m.imageCreate, flag(&m.imageCreate.Active))
	imageCreate.background = false
	imageDownload := newOverlayRef("imageDownload", &m.imageDownload, flag(&m.imageDownload.Active))
	imageDownload.background = false

	return []overlayRef{
		confirm,
		errModal,
		newOverlayRef("cloneProgress", &m.cloneProgress, flag(&m.cloneProgress.Active)),
		newOverlayRef("serverRename", &m.serverRename, flag(&m.serverRename.Active)),
		newOverlayRef("serverRebuild", &m.serverRebuild, flag(&m.serverRebuild.Active)),
		newOverlayRef("serverSnapshot", &m.serverSnapshot, flag(&m.serverSnapshot.Active)),
		newOverlayRef("serverResize", &m.serverResize, flag(&m.serverResize.Active)),
		newOverlayRef("serverAdminAct", &m.serverAdminAct, flag(&m.serverAdminAct.Active)),
		newOverlayRef("serverMetadata", &m.serverMetadata, flag(&m.serverMetadata.Active)),
		newOverlayRef("sshPrompt", &m.sshPrompt, flag(&m.sshPrompt.Active)),
		newOverlayRef("copyPicker", &m.copyPicker, flag(&m.copyPicker.Active)),
		newOverlayRef("consoleURL", &m.consoleURL, flag(&m.consoleURL.Active)),
		newOverlayRef("vmPassword", &m.vmPassword, flag(&m.vmPassword.Active)),
		newOverlayRef("fipPicker", &m.fipPicker, flag(&m.fipPicker.Active)),
		newOverlayRef("serverPicker", &m.serverPicker, flag(&m.serverPicker.Active)),
		newOverlayRef("volumePicker", &m.volumePicker, flag(&m.volumePicker.Active)),
		newOverlayRef("routerCreate", &m.routerCreate, flag(&m.routerCreate.Active)),
		newOverlayRef("subnetPicker", &m.subnetPicker, flag(&m.subnetPicker.Active)),
		newOverlayRef("networkCreate", &m.networkCreate, flag(&m.networkCreate.Active)),
		newOverlayRef("subnetCreate", &m.subnetCreate, flag(&m.subnetCreate.Active)),
		newOverlayRef("subnetEdit", &m.subnetEdit, flag(&m.subnetEdit.Active)),
		newOverlayRef("portCreate", &m.portCreate, flag(&m.portCreate.Active)),
		newOverlayRef("portEdit", &m.portEdit, flag(&m.portEdit.Active)),
		newOverlayRef("sgCreate", &m.sgCreate, flag(&m.sgCreate.Active)),
		newOverlayRef("sgRuleCreate", &m.sgRuleCreate, flag(&m.sgRuleCreate.Active)),
		newOverlayRef("imageEdit", &m.imageEdit, flag(&m.imageEdit.Active)),
		imageCreate,
		imageDownload,
		newOverlayRef("lbCreate", &m.lbCreate, flag(&m.lbCreate.Active)),
		newOverlayRef("lbListenerCreate", &m.lbListenerCreate, flag(&m.lbListenerCreate.Active)),
		newOverlayRef("lbPoolCreate", &m.lbPoolCreate, flag(&m.lbPoolCreate.Active)),
		newOverlayRef("lbMemberCreate", &m.lbMemberCreate, flag(&m.lbMemberCreate.Active)),
		newOverlayRef("lbMonitorCreate", &m.lbMonitorCreate, flag(&m.lbMonitorCreate.Active)),
		newOverlayRef("projectPicker", &m.projectPicker, flag(&m.projectPicker.Active)),
		newOverlayRef("columnPicker", &m.columnPicker, flag(&m.columnPicker.Active)),
	}
}

// firstOverlay returns the highest-precedence active overlay accepted by
// want.
func (m *Model) firstOverlay(want func(overlayRef) bool) (overlayRef, bool) {
	for _, o := range m.overlays() {
		if want(o) && o.IsActive() {
			return o, true
		}
	}
	return overlayRef{}, false
}

// topOverlay returns the overlay on screen, if any.
func (m *Model) topOverlay() (overlayRef, bool) {
	return m.firstOverlay(func(overlayRef) bool { return true })
}

// keyOverlay returns the overlay that updateAnyModal routes keys to.
func (m *Model) keyOverlay() (overlayRef, bool) {
	return m.firstOverlay(func(o overlayRef) bool { return o.keys })
}

// backgroundOverlay returns the overlay that updateAnyModalBackground routes
// non-key messages to.
func (m *Model) backgroundOverlay() (overlayRef, bool) {
	return m.firstOverlay(func(o overlayRef) bool { return o.background })
}

// cloneTakesBackground reports whether a dismissed clone that is still
// running gets background messages (its spinner ticks): only when no other
// overlay would take them.
func (m *Model) cloneTakesBackground() bool {
	if _, ok := m.backgroundOverlay(); ok {
		return false
	}
	return m.cloneProgress.Running()
}

// updateAnyModal routes a key message to the active modal/overlay.
// Returns true if the message was consumed by a modal.
func (m *Model) updateAnyModal(msg tea.Msg) (bool, tea.Cmd) {
	if o, ok := m.keyOverlay(); ok {
		return true, o.update(msg)
	}
	return false, nil
}

// updateImageDownloadBackground routes image-download lifecycle messages
// (progress ticks, done, error) to the download modal even when it is
// inactive, so backgrounded downloads keep flowing, and surfaces completion
// results in the status bar. Returns the resulting command, if any.
func (m *Model) updateImageDownloadBackground(msg tea.Msg) tea.Cmd {
	if fin, ok := msg.(imagedownload.DownloadFinishedMsg); ok {
		if errors.Is(fin.Err, shared.ErrTransferCanceled) {
			m.statusBar.StickyHint = transferCancelledHint("download", fin.Name, fin.Err)
			return nil
		}
		if fin.Err != nil {
			m.statusBar.StickyHint = fmt.Sprintf("%s Image download failed: %v", shared.Glyph("✗", "FAIL"), fin.Err)
		} else {
			m.statusBar.StickyHint = fmt.Sprintf("%s Image %s downloaded", shared.Glyph("✓", "OK"), fin.Name)
		}
		return nil
	}
	var cmd tea.Cmd
	m.imageDownload, cmd = m.imageDownload.Update(msg)
	return cmd
}

// updateImageUploadBackground routes image-upload lifecycle messages
// (progress ticks, done, error) to the upload modal even when it is
// inactive, so backgrounded uploads keep flowing, and surfaces completion
// results in the status bar. Returns the resulting command, if any.
func (m *Model) updateImageUploadBackground(msg tea.Msg) tea.Cmd {
	if fin, ok := msg.(imagecreate.UploadFinishedMsg); ok {
		if errors.Is(fin.Err, shared.ErrTransferCanceled) {
			m.statusBar.StickyHint = transferCancelledHint("upload", fin.Name, fin.Err)
			return nil
		}
		if fin.Err != nil {
			m.statusBar.StickyHint = fmt.Sprintf("%s Image upload failed: %s", shared.Glyph("✗", "FAIL"), shared.ParseError(fin.Err).FriendlyMessage)
		} else {
			m.statusBar.StickyHint = fmt.Sprintf("%s Image %s uploaded", shared.Glyph("✓", "OK"), fin.Name)
		}
		return nil
	}
	var cmd tea.Cmd
	m.imageCreate, cmd = m.imageCreate.Update(msg)
	return cmd
}

// transferCancelledHint reports a cancelled image transfer, including any
// cleanup problem the cancel ran into (a partial file or image left over).
func transferCancelledHint(kind, name string, err error) string {
	what := "Image " + kind
	if name != "" {
		what = fmt.Sprintf("Image %s of %s", kind, name)
	}
	if err != shared.ErrTransferCanceled {
		return fmt.Sprintf("%s %s cancelled: %s", shared.Glyph("✗", "FAIL"), what, shared.ParseError(err).FriendlyMessage)
	}
	return shared.Glyph("–", "-") + " " + what + " cancelled"
}

// updateAnyModalBackground routes non-key messages to active modals.
func (m *Model) updateAnyModalBackground(msg tea.Msg) tea.Cmd {
	if cmd := m.updateImageDownloadBackground(msg); cmd != nil {
		return cmd
	}
	if cmd := m.updateImageUploadBackground(msg); cmd != nil {
		return cmd
	}
	if o, ok := m.backgroundOverlay(); ok {
		return o.update(msg)
	}
	if m.cloneTakesBackground() {
		var cmd tea.Cmd
		m.cloneProgress, cmd = m.cloneProgress.Update(msg)
		return cmd
	}
	return nil
}

// setSizeAllModals propagates window size to all modal/overlay models.
func (m *Model) setSizeAllModals(w, h int) {
	for _, o := range m.overlays() {
		o.setSize(w, h)
	}
}

// updateClone routes a clone progress message to the clone operation with
// the given ID. Messages for unknown operations are dropped.
func (m *Model) updateClone(op uint64, msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.cloneProgress.ID() == op {
		m.cloneProgress, cmd = m.cloneProgress.Update(msg)
		return cmd
	}
	for i := range m.cloneBackground {
		if m.cloneBackground[i].ID() == op {
			m.cloneBackground[i], cmd = m.cloneBackground[i].Update(msg)
			return cmd
		}
	}
	return nil
}

// finishClone stops tracking a finished clone operation. It reports false
// when no tracked clone has that ID (for example a duplicate completion).
func (m *Model) finishClone(op uint64) bool {
	if m.cloneProgress.ID() == op {
		m.cloneProgress.Active = false
		return true
	}
	for i := range m.cloneBackground {
		if m.cloneBackground[i].ID() == op {
			m.cloneBackground = append(m.cloneBackground[:i], m.cloneBackground[i+1:]...)
			return true
		}
	}
	return false
}
