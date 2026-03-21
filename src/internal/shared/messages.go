package shared

import (
	"github.com/gophercloud/gophercloud/v2"
)

// CloudSelectedMsg is sent when a cloud is selected from the picker.
type CloudSelectedMsg struct {
	CloudName string
	Region    string
}

// CloudConnectedMsg is sent after successful authentication.
type CloudConnectedMsg struct {
	ComputeClient      *gophercloud.ServiceClient
	ImageClient        *gophercloud.ServiceClient
	NetworkClient      *gophercloud.ServiceClient
	BlockStorageClient *gophercloud.ServiceClient
	Region             string
}

// CloudConnectErrMsg is sent when authentication fails.
type CloudConnectErrMsg struct {
	Err error
}

// ErrMsg is a generic error message.
type ErrMsg struct {
	Err     error
	Context string
}

// ServerActionMsg is sent after a server action completes.
type ServerActionMsg struct {
	Action string
	Name   string
}

// ServerActionErrMsg is sent when a server action fails.
type ServerActionErrMsg struct {
	Action string
	Name   string
	Err    error
}

// RefreshServersMsg triggers a server list refresh.
type RefreshServersMsg struct{}

// TickMsg is sent by the auto-refresh ticker.
type TickMsg struct{}

// ViewChangeMsg requests a view change in the root model.
type ViewChangeMsg struct {
	View string
}

// RestartMsg signals the app should re-exec itself.
type RestartMsg struct{}

// ResourceActionMsg is sent after a non-server resource action completes.
type ResourceActionMsg struct {
	Action string
	Name   string
}

// ResourceActionErrMsg is sent when a non-server resource action fails.
type ResourceActionErrMsg struct {
	Action string
	Name   string
	Err    error
}

// RefreshResourceMsg triggers a resource list refresh for the current tab.
type RefreshResourceMsg struct{}
