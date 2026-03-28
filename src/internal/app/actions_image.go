package app

import (
	"context"
	"fmt"
	"strings"

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
	// Bulk delete
	if m.view == viewImageList && m.imageList.SelectionCount() > 0 {
		imgs := m.imageList.SelectedImages()
		refs := make([]modal.ServerRef, len(imgs))
		for i, img := range imgs {
			name := img.Name
			if name == "" {
				name = img.ID
			}
			refs[i] = modal.ServerRef{ID: img.ID, Name: name}
		}
		m.confirm = modal.NewBulkConfirm("delete_image", refs)
		m.confirm.Title = "Delete Images"
		m.confirm.Body = fmt.Sprintf("Are you sure you want to delete %d images?", len(imgs))
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil
	}

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
	// Bulk deactivate/reactivate
	if m.view == viewImageList && m.imageList.SelectionCount() > 0 {
		imgs := m.imageList.SelectedImages()
		// Determine action from first image's status
		action := "deactivate_image"
		title := "Deactivate Images"
		verb := "deactivate"
		if len(imgs) > 0 && imgs[0].Status == "deactivated" {
			action = "reactivate_image"
			title = "Reactivate Images"
			verb = "reactivate"
		}
		refs := make([]modal.ServerRef, len(imgs))
		for i, img := range imgs {
			name := img.Name
			if name == "" {
				name = img.ID
			}
			refs[i] = modal.ServerRef{ID: img.ID, Name: name}
		}
		m.confirm = modal.NewBulkConfirm(action, refs)
		m.confirm.Title = title
		m.confirm.Body = fmt.Sprintf("Are you sure you want to %s %d images?", verb, len(imgs))
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil
	}

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

func (m Model) executeBulkImageAction(action modal.ConfirmAction) tea.Cmd {
	targets := action.Servers
	act := action.Action
	imgClient := m.client.Image
	return func() tea.Msg {
		var errs []string
		for _, s := range targets {
			var err error
			switch act {
			case "delete_image":
				err = image.DeleteImage(context.Background(), imgClient, s.ID)
			case "deactivate_image":
				err = image.DeactivateImage(context.Background(), imgClient, s.ID)
			case "reactivate_image":
				err = image.ReactivateImage(context.Background(), imgClient, s.ID)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
			}
		}
		if len(errs) > 0 {
			return shared.ResourceActionErrMsg{
				Action: act,
				Name:   fmt.Sprintf("%d images", len(targets)),
				Err:    fmt.Errorf("%s", strings.Join(errs, "; ")),
			}
		}
		return shared.ResourceActionMsg{
			Action: act,
			Name:   fmt.Sprintf("%d images", len(targets)),
		}
	}
}
