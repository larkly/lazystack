package app

import (
	"context"
	"fmt"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/imagedetail"
	"github.com/larkly/lazystack/internal/ui/modal"
	"charm.land/bubbletea/v2"
)

func (m Model) openImageDetail() (Model, tea.Cmd) {
	img := m.imageList.SelectedImage()
	if img == nil {
		return m, nil
	}
	m.imageDetail = imagedetail.New(m.client.Image, img.ID, m.refreshInterval)
	m.imageDetail.SetSize(m.width, m.height)
	m.view = viewImageDetail
	m.statusBar.CurrentView = "imagedetail"
	m.statusBar.Hint = m.imageDetail.Hints()
	return m, m.imageDetail.Init()
}

func (m Model) openImageDeleteConfirm() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewImageList:
		if img := m.imageList.SelectedImage(); img != nil {
			id, name = img.ID, img.Name
			if name == "" {
				name = id
			}
		}
	case viewImageDetail:
		id = m.imageDetail.ImageID()
		name = m.imageDetail.ImageName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_image", id, name)
	m.confirm.Title = "Delete Image"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete image %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openImageDeactivateConfirm() (Model, tea.Cmd) {
	var id, name, status string
	switch m.view {
	case viewImageList:
		if img := m.imageList.SelectedImage(); img != nil {
			id, name, status = img.ID, img.Name, img.Status
			if name == "" {
				name = id
			}
		}
	case viewImageDetail:
		id = m.imageDetail.ImageID()
		name = m.imageDetail.ImageName()
		status = m.imageDetail.ImageStatus()
	}
	if id == "" {
		return m, nil
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
		err := image.DeactivateImage(context.Background(), imgClient, id)
		if err != nil {
			return shared.ResourceActionErrMsg{Action: "Deactivate image", Name: name, Err: err}
		}
		return shared.ResourceActionMsg{Action: "Deactivated image", Name: name}
	}
}

func (m Model) doReactivateImage(id, name string) tea.Cmd {
	imgClient := m.client.Image
	return func() tea.Msg {
		err := image.ReactivateImage(context.Background(), imgClient, id)
		if err != nil {
			return shared.ResourceActionErrMsg{Action: "Reactivate image", Name: name, Err: err}
		}
		return shared.ResourceActionMsg{Action: "Reactivated image", Name: name}
	}
}
