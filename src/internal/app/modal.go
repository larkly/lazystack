package app

import tea "charm.land/bubbletea/v2"

// isAnyModalActive checks all overlay/modals in priority order and returns
// true if any of them is active. This replaces the ~25 if-blocks duplicated
// between Update() and View().
func (m *Model) isAnyModalActive() bool {
	return m.activeModal != modalNone ||
		m.cloneProgress.Active ||
		m.serverRename.Active ||
		m.serverRebuild.Active ||
		m.serverSnapshot.Active ||
		m.serverResize.Active ||
		m.serverAdminAct.Active ||
		m.serverMetadata.Active ||
		m.sshPrompt.Active ||
		m.copyPicker.Active ||
		m.consoleURL.Active ||
		m.vmPassword.Active ||
		m.fipPicker.Active ||
		m.serverPicker.Active ||
		m.volumePicker.Active ||
		m.routerCreate.Active ||
		m.subnetPicker.Active ||
		m.networkCreate.Active ||
		m.subnetCreate.Active ||
		m.subnetEdit.Active ||
		m.portCreate.Active ||
		m.portEdit.Active ||
		m.sgCreate.Active ||
		m.sgRuleCreate.Active ||
		m.imageEdit.Active ||
		m.imageCreate.Active ||
		m.imageDownload.Active ||
		m.lbCreate.Active ||
		m.lbListenerCreate.Active ||
		m.lbPoolCreate.Active ||
		m.lbMemberCreate.Active ||
		m.lbMonitorCreate.Active ||
		m.projectPicker.Active ||
		m.columnPicker.Active
}

// activeModalView returns the view string of whichever modal/overlay is active.
// Returns ("", false) if no modal is active.
func (m *Model) activeModalView() (string, bool) {
	if m.activeModal == modalConfirm {
		return m.confirm.View(), true
	}
	if m.activeModal == modalError {
		return m.errModal.View(), true
	}
	if m.cloneProgress.Active {
		return m.cloneProgress.View(), true
	}
	if m.serverRename.Active {
		return m.serverRename.View(), true
	}
	if m.serverRebuild.Active {
		return m.serverRebuild.View(), true
	}
	if m.serverSnapshot.Active {
		return m.serverSnapshot.View(), true
	}
	if m.serverResize.Active {
		return m.serverResize.View(), true
	}
	if m.serverAdminAct.Active {
		return m.serverAdminAct.View(), true
	}
	if m.serverMetadata.Active {
		return m.serverMetadata.View(), true
	}
	if m.sshPrompt.Active {
		return m.sshPrompt.View(), true
	}
	if m.copyPicker.Active {
		return m.copyPicker.View(), true
	}
	if m.consoleURL.Active {
		return m.consoleURL.View(), true
	}
	if m.vmPassword.Active {
		return m.vmPassword.View(), true
	}
	if m.fipPicker.Active {
		return m.fipPicker.View(), true
	}
	if m.serverPicker.Active {
		return m.serverPicker.View(), true
	}
	if m.volumePicker.Active {
		return m.volumePicker.View(), true
	}
	if m.routerCreate.Active {
		return m.routerCreate.View(), true
	}
	if m.subnetPicker.Active {
		return m.subnetPicker.View(), true
	}
	if m.networkCreate.Active {
		return m.networkCreate.View(), true
	}
	if m.subnetCreate.Active {
		return m.subnetCreate.View(), true
	}
	if m.subnetEdit.Active {
		return m.subnetEdit.View(), true
	}
	if m.portCreate.Active {
		return m.portCreate.View(), true
	}
	if m.portEdit.Active {
		return m.portEdit.View(), true
	}
	if m.sgCreate.Active {
		return m.sgCreate.View(), true
	}
	if m.sgRuleCreate.Active {
		return m.sgRuleCreate.View(), true
	}
	if m.imageEdit.Active {
		return m.imageEdit.View(), true
	}
	if m.imageCreate.Active {
		return m.imageCreate.View(), true
	}
	if m.imageDownload.Active {
		return m.imageDownload.View(), true
	}
	if m.lbCreate.Active {
		return m.lbCreate.View(), true
	}
	if m.lbListenerCreate.Active {
		return m.lbListenerCreate.View(), true
	}
	if m.lbPoolCreate.Active {
		return m.lbPoolCreate.View(), true
	}
	if m.lbMemberCreate.Active {
		return m.lbMemberCreate.View(), true
	}
	if m.lbMonitorCreate.Active {
		return m.lbMonitorCreate.View(), true
	}
	if m.projectPicker.Active {
		return m.projectPicker.View(), true
	}
	if m.columnPicker.Active {
		return m.columnPicker.View(), true
	}
	return "", false
}

// updateAnyModal routes a key message to the active modal/overlay.
// Returns true if the message was consumed by a modal.
func (m *Model) updateAnyModal(msg tea.Msg) (bool, tea.Cmd) {
	switch {
	case m.cloneProgress.Active:
		var cmd tea.Cmd
		m.cloneProgress, cmd = m.cloneProgress.Update(msg)
		return true, cmd
	case m.serverRename.Active:
		var cmd tea.Cmd
		m.serverRename, cmd = m.serverRename.Update(msg)
		return true, cmd
	case m.serverRebuild.Active:
		var cmd tea.Cmd
		m.serverRebuild, cmd = m.serverRebuild.Update(msg)
		return true, cmd
	case m.serverSnapshot.Active:
		var cmd tea.Cmd
		m.serverSnapshot, cmd = m.serverSnapshot.Update(msg)
		return true, cmd
	case m.serverResize.Active:
		var cmd tea.Cmd
		m.serverResize, cmd = m.serverResize.Update(msg)
		return true, cmd
	case m.serverAdminAct.Active:
		var cmd tea.Cmd
		m.serverAdminAct, cmd = m.serverAdminAct.Update(msg)
		return true, cmd
	case m.serverMetadata.Active:
		var cmd tea.Cmd
		m.serverMetadata, cmd = m.serverMetadata.Update(msg)
		return true, cmd
	case m.sshPrompt.Active:
		var cmd tea.Cmd
		m.sshPrompt, cmd = m.sshPrompt.Update(msg)
		return true, cmd
	case m.copyPicker.Active:
		var cmd tea.Cmd
		m.copyPicker, cmd = m.copyPicker.Update(msg)
		return true, cmd
	case m.consoleURL.Active:
		var cmd tea.Cmd
		m.consoleURL, cmd = m.consoleURL.Update(msg)
		return true, cmd
	case m.vmPassword.Active:
		var cmd tea.Cmd
		m.vmPassword, cmd = m.vmPassword.Update(msg)
		return true, cmd
	case m.fipPicker.Active:
		var cmd tea.Cmd
		m.fipPicker, cmd = m.fipPicker.Update(msg)
		return true, cmd
	case m.serverPicker.Active:
		var cmd tea.Cmd
		m.serverPicker, cmd = m.serverPicker.Update(msg)
		return true, cmd
	case m.volumePicker.Active:
		var cmd tea.Cmd
		m.volumePicker, cmd = m.volumePicker.Update(msg)
		return true, cmd
	case m.routerCreate.Active:
		var cmd tea.Cmd
		m.routerCreate, cmd = m.routerCreate.Update(msg)
		return true, cmd
	case m.subnetPicker.Active:
		var cmd tea.Cmd
		m.subnetPicker, cmd = m.subnetPicker.Update(msg)
		return true, cmd
	case m.networkCreate.Active:
		var cmd tea.Cmd
		m.networkCreate, cmd = m.networkCreate.Update(msg)
		return true, cmd
	case m.subnetCreate.Active:
		var cmd tea.Cmd
		m.subnetCreate, cmd = m.subnetCreate.Update(msg)
		return true, cmd
	case m.subnetEdit.Active:
		var cmd tea.Cmd
		m.subnetEdit, cmd = m.subnetEdit.Update(msg)
		return true, cmd
	case m.portCreate.Active:
		var cmd tea.Cmd
		m.portCreate, cmd = m.portCreate.Update(msg)
		return true, cmd
	case m.portEdit.Active:
		var cmd tea.Cmd
		m.portEdit, cmd = m.portEdit.Update(msg)
		return true, cmd
	case m.sgCreate.Active:
		var cmd tea.Cmd
		m.sgCreate, cmd = m.sgCreate.Update(msg)
		return true, cmd
	case m.sgRuleCreate.Active:
		var cmd tea.Cmd
		m.sgRuleCreate, cmd = m.sgRuleCreate.Update(msg)
		return true, cmd
	case m.imageEdit.Active:
		var cmd tea.Cmd
		m.imageEdit, cmd = m.imageEdit.Update(msg)
		return true, cmd
	case m.imageCreate.Active:
		var cmd tea.Cmd
		m.imageCreate, cmd = m.imageCreate.Update(msg)
		return true, cmd
	case m.imageDownload.Active:
		var cmd tea.Cmd
		m.imageDownload, cmd = m.imageDownload.Update(msg)
		return true, cmd
	case m.lbCreate.Active:
		var cmd tea.Cmd
		m.lbCreate, cmd = m.lbCreate.Update(msg)
		return true, cmd
	case m.lbListenerCreate.Active:
		var cmd tea.Cmd
		m.lbListenerCreate, cmd = m.lbListenerCreate.Update(msg)
		return true, cmd
	case m.lbPoolCreate.Active:
		var cmd tea.Cmd
		m.lbPoolCreate, cmd = m.lbPoolCreate.Update(msg)
		return true, cmd
	case m.lbMemberCreate.Active:
		var cmd tea.Cmd
		m.lbMemberCreate, cmd = m.lbMemberCreate.Update(msg)
		return true, cmd
	case m.lbMonitorCreate.Active:
		var cmd tea.Cmd
		m.lbMonitorCreate, cmd = m.lbMonitorCreate.Update(msg)
		return true, cmd
	case m.projectPicker.Active:
		var cmd tea.Cmd
		m.projectPicker, cmd = m.projectPicker.Update(msg)
		return true, cmd
	case m.columnPicker.Active:
		var cmd tea.Cmd
		m.columnPicker, cmd = m.columnPicker.Update(msg)
		return true, cmd
	}
	return false, nil
}

// updateAnyModalBackground routes non-key messages to active modals.
func (m *Model) updateAnyModalBackground(msg tea.Msg) tea.Cmd {
	switch {
	case m.serverRename.Active:
		var cmd tea.Cmd
		m.serverRename, cmd = m.serverRename.Update(msg)
		return cmd
	case m.serverRebuild.Active:
		var cmd tea.Cmd
		m.serverRebuild, cmd = m.serverRebuild.Update(msg)
		return cmd
	case m.serverSnapshot.Active:
		var cmd tea.Cmd
		m.serverSnapshot, cmd = m.serverSnapshot.Update(msg)
		return cmd
	case m.serverResize.Active:
		var cmd tea.Cmd
		m.serverResize, cmd = m.serverResize.Update(msg)
		return cmd
	case m.fipPicker.Active:
		var cmd tea.Cmd
		m.fipPicker, cmd = m.fipPicker.Update(msg)
		return cmd
	case m.serverPicker.Active:
		var cmd tea.Cmd
		m.serverPicker, cmd = m.serverPicker.Update(msg)
		return cmd
	case m.volumePicker.Active:
		var cmd tea.Cmd
		m.volumePicker, cmd = m.volumePicker.Update(msg)
		return cmd
	case m.routerCreate.Active:
		var cmd tea.Cmd
		m.routerCreate, cmd = m.routerCreate.Update(msg)
		return cmd
	case m.subnetPicker.Active:
		var cmd tea.Cmd
		m.subnetPicker, cmd = m.subnetPicker.Update(msg)
		return cmd
	case m.networkCreate.Active:
		var cmd tea.Cmd
		m.networkCreate, cmd = m.networkCreate.Update(msg)
		return cmd
	case m.subnetCreate.Active:
		var cmd tea.Cmd
		m.subnetCreate, cmd = m.subnetCreate.Update(msg)
		return cmd
	case m.subnetEdit.Active:
		var cmd tea.Cmd
		m.subnetEdit, cmd = m.subnetEdit.Update(msg)
		return cmd
	case m.portCreate.Active:
		var cmd tea.Cmd
		m.portCreate, cmd = m.portCreate.Update(msg)
		return cmd
	case m.portEdit.Active:
		var cmd tea.Cmd
		m.portEdit, cmd = m.portEdit.Update(msg)
		return cmd
	case m.sgCreate.Active:
		var cmd tea.Cmd
		m.sgCreate, cmd = m.sgCreate.Update(msg)
		return cmd
	case m.sgRuleCreate.Active:
		var cmd tea.Cmd
		m.sgRuleCreate, cmd = m.sgRuleCreate.Update(msg)
		return cmd
	case m.imageEdit.Active:
		var cmd tea.Cmd
		m.imageEdit, cmd = m.imageEdit.Update(msg)
		return cmd
	case m.imageCreate.Active:
		var cmd tea.Cmd
		m.imageCreate, cmd = m.imageCreate.Update(msg)
		return cmd
	case m.imageDownload.Active:
		var cmd tea.Cmd
		m.imageDownload, cmd = m.imageDownload.Update(msg)
		return cmd
	case m.lbCreate.Active:
		var cmd tea.Cmd
		m.lbCreate, cmd = m.lbCreate.Update(msg)
		return cmd
	case m.lbListenerCreate.Active:
		var cmd tea.Cmd
		m.lbListenerCreate, cmd = m.lbListenerCreate.Update(msg)
		return cmd
	case m.lbPoolCreate.Active:
		var cmd tea.Cmd
		m.lbPoolCreate, cmd = m.lbPoolCreate.Update(msg)
		return cmd
	case m.lbMemberCreate.Active:
		var cmd tea.Cmd
		m.lbMemberCreate, cmd = m.lbMemberCreate.Update(msg)
		return cmd
	case m.lbMonitorCreate.Active:
		var cmd tea.Cmd
		m.lbMonitorCreate, cmd = m.lbMonitorCreate.Update(msg)
		return cmd
	case m.projectPicker.Active:
		var cmd tea.Cmd
		m.projectPicker, cmd = m.projectPicker.Update(msg)
		return cmd
	case m.cloneProgress.Running():
		var cmd tea.Cmd
		m.cloneProgress, cmd = m.cloneProgress.Update(msg)
		return cmd
	}
	return nil
}

// setSizeAllModals propagates window size to all modal/overlay models.
func (m *Model) setSizeAllModals(w, h int) {
	m.confirm.SetSize(w, h)
	m.errModal.SetSize(w, h)
	m.serverRename.SetSize(w, h)
	m.serverRebuild.SetSize(w, h)
	m.serverSnapshot.SetSize(w, h)
	m.serverResize.SetSize(w, h)
	m.sshPrompt.SetSize(w, h)
	m.copyPicker.SetSize(w, h)
	m.consoleURL.SetSize(w, h)
	m.vmPassword.SetSize(w, h)
	m.fipPicker.SetSize(w, h)
	m.serverPicker.SetSize(w, h)
	m.volumePicker.SetSize(w, h)
	m.sgCreate.SetSize(w, h)
	m.sgRuleCreate.SetSize(w, h)
	m.networkCreate.SetSize(w, h)
	m.subnetCreate.SetSize(w, h)
	m.subnetEdit.SetSize(w, h)
	m.portCreate.SetSize(w, h)
	m.portEdit.SetSize(w, h)
	m.routerCreate.SetSize(w, h)
	m.subnetPicker.SetSize(w, h)
	m.projectPicker.SetSize(w, h)
	m.columnPicker.SetSize(w, h)
	m.cloneProgress.SetSize(w, h)
}
