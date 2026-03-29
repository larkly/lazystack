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
	LoadBalancerClient *gophercloud.ServiceClient
	ProviderClient     *gophercloud.ProviderClient
	EndpointOpts       gophercloud.EndpointOpts
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

// ProjectInfo is a simplified project reference for UI use.
type ProjectInfo struct {
	ID   string
	Name string
}

// ProjectsLoadedMsg is sent after accessible projects are fetched.
type ProjectsLoadedMsg struct {
	Projects  []ProjectInfo
	CurrentID string
}

// ProjectSelectedMsg is sent when user picks a project.
type ProjectSelectedMsg struct {
	ProjectID   string
	ProjectName string
}

// NavigateToResourceMsg requests navigation to a resource tab with the cursor
// positioned on a specific resource. Used for cross-resource navigation
// (e.g. server detail → attached volumes).
type NavigateToResourceMsg struct {
	Tab       string   // tab key: "volumes", "secgroups", "networks"
	Highlight []string // resource IDs or names to scroll to
}

// SSHFinishedMsg is sent when the SSH process exits.
type SSHFinishedMsg struct {
	Err error
}

// ConsoleURLMsg is sent when a noVNC console URL has been fetched.
type ConsoleURLMsg struct {
	URL        string
	ServerName string
}

// ConsoleURLErrMsg is sent when fetching a console URL fails.
type ConsoleURLErrMsg struct {
	Err        error
	ServerName string
}

// ConfigChangedMsg is sent after the config is modified and saved at runtime.
type ConfigChangedMsg struct{}
