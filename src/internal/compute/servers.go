package compute

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Server is a simplified representation of a Nova server.
type Server struct {
	ID        string
	Name      string
	Status     string
	PowerState string
	FlavorName string
	FlavorID   string
	ImageID    string
	ImageName  string
	IPv4       []string
	IPv6       []string
	FloatingIP []string
	Locked     bool
	KeyName   string
	Created   time.Time
	TenantID  string
	AZ        string
	VolAttach []string
	SecGroups []string
	Networks  map[string][]string // network name → IPs
}

// ListServers fetches all servers from Nova.
func ListServers(ctx context.Context, client *gophercloud.ServiceClient) ([]Server, error) {
	opts := servers.ListOpts{
		AllTenants: false,
	}

	var result []Server
	err := servers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := servers.ExtractServers(page)
		if err != nil {
			return false, err
		}
		for _, s := range extracted {
			result = append(result, mapServer(s))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	return result, nil
}

// GetServer fetches a single server by ID.
func GetServer(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Server, error) {
	r := servers.Get(ctx, client, id)
	s, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("getting server %s: %w", id, err)
	}
	srv := mapServer(*s)
	return &srv, nil
}

// CreateServer creates a new server.
func CreateServer(ctx context.Context, client *gophercloud.ServiceClient, opts servers.CreateOpts) (*Server, error) {
	return CreateServerWithOpts(ctx, client, opts)
}

// CreateServerWithOpts creates a new server with arbitrary CreateOptsBuilder (e.g. keypairs.CreateOptsExt).
func CreateServerWithOpts(ctx context.Context, client *gophercloud.ServiceClient, opts servers.CreateOptsBuilder) (*Server, error) {
	r := servers.Create(ctx, client, opts, nil)
	s, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("creating server: %w", err)
	}
	srv := mapServer(*s)
	return &srv, nil
}

// DeleteServer deletes a server by ID.
func DeleteServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting server %s: %w", id, r.Err)
	}
	return nil
}

// RebootServer reboots a server. Use servers.SoftReboot or servers.HardReboot.
func RebootServer(ctx context.Context, client *gophercloud.ServiceClient, id string, how servers.RebootMethod) error {
	r := servers.Reboot(ctx, client, id, servers.RebootOpts{Type: how})
	if r.Err != nil {
		return fmt.Errorf("rebooting server %s: %w", id, r.Err)
	}
	return nil
}

// PauseServer pauses a server.
func PauseServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Pause(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("pausing server %s: %w", id, r.Err)
	}
	return nil
}

// UnpauseServer unpauses a server.
func UnpauseServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Unpause(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("unpausing server %s: %w", id, r.Err)
	}
	return nil
}

// SuspendServer suspends a server.
func SuspendServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Suspend(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("suspending server %s: %w", id, r.Err)
	}
	return nil
}

// ResumeServer resumes a suspended server.
func ResumeServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Resume(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("resuming server %s: %w", id, r.Err)
	}
	return nil
}

// ShelveServer shelves a server.
func ShelveServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Shelve(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("shelving server %s: %w", id, r.Err)
	}
	return nil
}

// UnshelveServer unshelves a server.
func UnshelveServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Unshelve(ctx, client, id, servers.UnshelveOpts{})
	if r.Err != nil {
		return fmt.Errorf("unshelving server %s: %w", id, r.Err)
	}
	return nil
}

// StopServer stops (shuts down) a server.
func StopServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Stop(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("stopping server %s: %w", id, r.Err)
	}
	return nil
}

// StartServer starts a stopped server.
func StartServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Start(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("starting server %s: %w", id, r.Err)
	}
	return nil
}

// LockServer prevents modifications to a server.
func LockServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Lock(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("locking server %s: %w", id, r.Err)
	}
	return nil
}

// UnlockServer removes the lock on a server.
func UnlockServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Unlock(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("unlocking server %s: %w", id, r.Err)
	}
	return nil
}

// ResizeServer resizes a server to a new flavor.
func ResizeServer(ctx context.Context, client *gophercloud.ServiceClient, id, flavorRef string) error {
	r := servers.Resize(ctx, client, id, servers.ResizeOpts{FlavorRef: flavorRef})
	if r.Err != nil {
		return fmt.Errorf("resizing server %s: %w", id, r.Err)
	}
	return nil
}

// ConfirmResize confirms a server resize.
func ConfirmResize(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.ConfirmResize(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("confirming resize for server %s: %w", id, r.Err)
	}
	return nil
}

// RevertResize reverts a server resize.
func RevertResize(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.RevertResize(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("reverting resize for server %s: %w", id, r.Err)
	}
	return nil
}

// CreateSnapshot creates an image snapshot of a server.
func CreateSnapshot(ctx context.Context, client *gophercloud.ServiceClient, id, snapshotName string) error {
	r := servers.CreateImage(ctx, client, id, servers.CreateImageOpts{Name: snapshotName})
	if r.Err != nil {
		if gophercloud.ResponseCodeIs(r.Err, 409) {
			return fmt.Errorf("server already has a snapshot in progress")
		}
		return fmt.Errorf("creating snapshot of server %s: %w", id, r.Err)
	}
	return nil
}

// RebuildServer rebuilds a server with a new image.
func RebuildServer(ctx context.Context, client *gophercloud.ServiceClient, id, imageRef string) error {
	_, err := servers.Rebuild(ctx, client, id, servers.RebuildOpts{ImageRef: imageRef}).Extract()
	if err != nil {
		return fmt.Errorf("rebuilding server %s: %w", id, err)
	}
	return nil
}

// RescueServer places a server into RESCUE mode and returns the admin password.
// If imageRef is non-empty, the server is rescued with the specified image.
func RescueServer(ctx context.Context, client *gophercloud.ServiceClient, id, imageRef string) (string, error) {
	opts := servers.RescueOpts{RescueImageRef: imageRef}
	r := servers.Rescue(ctx, client, id, opts)
	adminPass, err := r.Extract()
	if err != nil {
		return "", fmt.Errorf("rescuing server %s: %w", id, err)
	}
	return adminPass, nil
}

// UnrescueServer returns a server from RESCUE mode.
func UnrescueServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := servers.Unrescue(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("unrescuing server %s: %w", id, r.Err)
	}
	return nil
}

// RenameServer updates a server's name.
func RenameServer(ctx context.Context, client *gophercloud.ServiceClient, id, newName string) error {
	_, err := servers.Update(ctx, client, id, servers.UpdateOpts{Name: newName}).Extract()
	if err != nil {
		return fmt.Errorf("renaming server %s: %w", id, err)
	}
	return nil
}

// GetConsoleOutput retrieves console output for a server.
func GetConsoleOutput(ctx context.Context, client *gophercloud.ServiceClient, id string, lines int) (string, error) {
	r := servers.ShowConsoleOutput(ctx, client, id, servers.ShowConsoleOutputOpts{Length: lines})
	output, err := r.Extract()
	if err != nil {
		return "", fmt.Errorf("getting console output for %s: %w", id, err)
	}
	return output, nil
}

func mapServer(s servers.Server) Server {
	srv := Server{
		ID:         s.ID,
		Name:       s.Name,
		Status:     s.Status,
		PowerState: s.PowerState.String(),
		KeyName:    s.KeyName,
		Created:    s.Created,
		TenantID:   s.TenantID,
	}

	// Flavor — microversion 2.47+ embeds full flavor with original_name
	if name, ok := s.Flavor["original_name"].(string); ok {
		srv.FlavorName = name
	}
	if id, ok := s.Flavor["id"].(string); ok {
		srv.FlavorID = id
	}

	// Image — can be empty for boot-from-volume
	if imgMap := s.Image; len(imgMap) > 0 {
		if id, ok := imgMap["id"].(string); ok {
			srv.ImageID = id
		}
		if name, ok := imgMap["name"].(string); ok {
			srv.ImageName = name
		}
	}

	// Extract IPs by type
	srv.IPv4, srv.IPv6, srv.FloatingIP = classifyIPs(s.Addresses)
	srv.Networks = ExtractAllIPs(s.Addresses)

	// Locked
	if s.Locked != nil {
		srv.Locked = *s.Locked
	}

	// Security groups
	for _, sg := range s.SecurityGroups {
		if name, ok := sg["name"].(string); ok {
			srv.SecGroups = append(srv.SecGroups, name)
		}
	}

	// Availability zone
	srv.AZ = s.AvailabilityZone

	// Attached volumes
	for _, v := range s.AttachedVolumes {
		srv.VolAttach = append(srv.VolAttach, v.ID)
	}

	return srv
}

func classifyIPs(addresses map[string]interface{}) (ipv4, ipv6, floating []string) {
	for _, netAddrs := range addresses {
		addrs, ok := netAddrs.([]interface{})
		if !ok {
			continue
		}
		for _, a := range addrs {
			addrMap, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			addr, ok := addrMap["addr"].(string)
			if !ok {
				continue
			}

			ipType, _ := addrMap["OS-EXT-IPS:type"].(string)
			version, _ := addrMap["version"].(float64)

			if ipType == "floating" {
				floating = append(floating, addr)
			} else if version == 6 {
				ipv6 = append(ipv6, addr)
			} else {
				ipv4 = append(ipv4, addr)
			}
		}
	}
	return
}

// ExtractAllIPs returns all IPs grouped by network name.
func ExtractAllIPs(addresses map[string]interface{}) map[string][]string {
	result := make(map[string][]string)
	for netName, netAddrs := range addresses {
		addrs, ok := netAddrs.([]interface{})
		if !ok {
			continue
		}
		for _, a := range addrs {
			addrMap, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			if addr, ok := addrMap["addr"].(string); ok {
				version := ""
				if v, ok := addrMap["version"].(float64); ok {
					version = fmt.Sprintf("v%d", int(v))
				}
				typ := ""
				if t, ok := addrMap["OS-EXT-IPS:type"].(string); ok {
					typ = t
				}
				entry := addr
				if version != "" || typ != "" {
					parts := []string{addr}
					if typ != "" {
						parts = append(parts, typ)
					}
					if version != "" {
						parts = append(parts, version)
					}
					entry = strings.Join(parts, " ")
				}
				result[netName] = append(result[netName], entry)
			}
		}
	}
	return result
}
