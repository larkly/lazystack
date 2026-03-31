package app

import (
	"context"
	"fmt"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/imagecreate"
	"github.com/larkly/lazystack/internal/ui/imagedownload"
	"github.com/larkly/lazystack/internal/ui/imageedit"
	"github.com/larkly/lazystack/internal/ui/modal"
	"charm.land/bubbletea/v2"
)

func (m Model) openImageEdit() (Model, tea.Cmd) {
	im := m.imageView.SelectedImage()
	if im == nil {
		return m, nil
	}
	m.imageEdit = imageedit.New(m.client.Image, im.ID, im.Name, im.Visibility,
		im.MinDisk, im.MinRAM, im.Tags, im.Protected)
	m.imageEdit.SetSize(m.width, m.height)
	return m, m.imageEdit.Init()
}

func (m Model) openImageUpload() (Model, tea.Cmd) {
	m.imageCreate = imagecreate.New(m.client.Image)
	m.imageCreate.SetSize(m.width, m.height)
	return m, m.imageCreate.Init()
}

func (m Model) openImageDownload() (Model, tea.Cmd) {
	im := m.imageView.SelectedImage()
	if im == nil {
		return m, nil
	}
	m.imageDownload = imagedownload.New(m.client.Image, im.ID, im.Name, im.DiskFormat)
	m.imageDownload.SetSize(m.width, m.height)
	return m, m.imageDownload.Init()
}

func (m Model) openImageDeleteConfirm() (Model, tea.Cmd) {
	im := m.imageView.SelectedImage()
	if im == nil {
		return m, nil
	}
	if im.Protected {
		m.statusBar.Error = "Image is protected \u2014 edit to unprotect first"
		return m, nil
	}
	id, name := im.ID, im.Name
	if name == "" {
		name = id
	}
	m.confirm = modal.NewConfirm("delete_image", id, name)
	m.confirm.Title = "Delete Image"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete image %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openImageDeactivateConfirm() (Model, tea.Cmd) {
	im := m.imageView.SelectedImage()
	if im == nil {
		return m, nil
	}
	id, name, status := im.ID, im.Name, im.Status
	if name == "" {
		name = id
	}

	action := "deactivate_image"
	title := "Deactivate Image"
	body := fmt.Sprintf("Are you sure you want to deactivate image %q?", name)
	if status == "deactivated" {
		action = "reactivate_image"
		title = "Reactivate Image"
		body = fmt.Sprintf("Are you sure you want to reactivate image %q?", name)
	}

	m.confirm = modal.NewConfirm(action, id, name)
	m.confirm.Title = title
	m.confirm.Body = body
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) doDeactivateImage(id, name string) tea.Cmd {
	imgClient := m.client.Image
	return func() tea.Msg {
		shared.Debugf("[action] deactivating image %s", name)
		err := image.DeactivateImage(context.Background(), imgClient, id)
		if err != nil {
			shared.Debugf("[action] deactivate image %s failed: %s", name, err)
			return shared.ResourceActionErrMsg{Action: "Deactivate image", Name: name, Err: err}
		}
		shared.Debugf("[action] deactivated image %s", name)
		return shared.ResourceActionMsg{Action: "Deactivated image", Name: name}
	}
}

func (m Model) doReactivateImage(id, name string) tea.Cmd {
	imgClient := m.client.Image
	return func() tea.Msg {
		shared.Debugf("[action] reactivating image %s", name)
		err := image.ReactivateImage(context.Background(), imgClient, id)
		if err != nil {
			shared.Debugf("[action] reactivate image %s failed: %s", name, err)
			return shared.ResourceActionErrMsg{Action: "Reactivate image", Name: name, Err: err}
		}
		shared.Debugf("[action] reactivated image %s", name)
		return shared.ResourceActionMsg{Action: "Reactivated image", Name: name}
	}
}
