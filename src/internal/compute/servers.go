package compute

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/remoteconsoles"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// Server is a simplified representation of a Nova server.
type Server struct {
	ID          string
	Name        string
	Description string
	Status      string
	PowerState  string
	FlavorName  string
	FlavorID    string
	FlavorVCPUs int
	FlavorRAM   int // MB
	FlavorDisk  int // GB
	ImageID     string
	ImageName   string
	IPv4        []string
	IPv6        []string
	FloatingIP  []string
	Locked      bool
	KeyName     string
	Created     time.Time
	TenantID    string
	AZ          string
	VolAttach   []VolumeAttachment
	SecGroups   []string
	Networks    map[string][]string // network name → IPs
	Metadata    map[string]string
}

// VolumeAttachment holds a volume ID and its device path on the server.
type VolumeAttachment struct {
	ID     string
	Device string
}

// VolumeAttachmentIDs returns just the IDs from a slice of VolumeAttachments.
func VolumeAttachmentIDs(attachments []VolumeAttachment) []string {
	ids := make([]string, len(attachments))
	for i, a := range attachments {
		ids[i] = a.ID
	}
	return ids
}

// ListServers fetches all servers from Nova.
func ListServers(ctx context.Context, client *gophercloud.ServiceClient) ([]Server, error) {
	shared.Debugf("[compute] listing servers")
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
		shared.Debugf("[compute] list servers: %v", err)
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	shared.Debugf("[compute] listed %d servers", len(result))
	return result, nil
}

// GetServer fetches a single server by ID.
func GetServer(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Server, error) {
	shared.Debugf("[compute] getting server %s", id)
	r := servers.Get(ctx, client, id)
	s, err := r.Extract()
	if err != nil {
		shared.Debugf("[compute] get server %s: %v", id, err)
		return nil, fmt.Errorf("getting server %s: %w", id, err)
	}
	srv := mapServer(*s)
	shared.Debugf("[compute] got server %s (%s)", srv.Name, id)
	return &srv, nil
}

// CreateServer creates a new server.
func CreateServer(ctx context.Context, client *gophercloud.ServiceClient, opts servers.CreateOpts) (*Server, error) {
	shared.Debugf("[compute] creating server %q", opts.Name)
	return CreateServerWithOpts(ctx, client, opts)
}

// CreateServerWithOpts creates a new server with arbitrary CreateOptsBuilder (e.g. keypairs.CreateOptsExt).
func CreateServerWithOpts(ctx context.Context, client *gophercloud.ServiceClient, opts servers.CreateOptsBuilder) (*Server, error) {
	shared.Debugf("[compute] creating server with opts")
	r := servers.Create(ctx, client, opts, nil)
	s, err := r.Extract()
	if err != nil {
		shared.Debugf("[compute] create server: %v", err)
		return nil, fmt.Errorf("creating server: %w", err)
	}
	srv := mapServer(*s)
	shared.Debugf("[compute] created server %s (%s)", srv.Name, srv.ID)
	return &srv, nil
}

// DeleteServer deletes a server by ID.
func DeleteServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] deleting server %s", id)
	r := servers.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] delete server %s: %v", id, r.Err)
		return fmt.Errorf("deleting server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] deleted server %s", id)
	return nil
}

// RebootServer reboots a server. Use servers.SoftReboot or servers.HardReboot.
func RebootServer(ctx context.Context, client *gophercloud.ServiceClient, id string, how servers.RebootMethod) error {
	shared.Debugf("[compute] rebooting server %s (method=%s)", id, how)
	r := servers.Reboot(ctx, client, id, servers.RebootOpts{Type: how})
	if r.Err != nil {
		shared.Debugf("[compute] reboot server %s: %v", id, r.Err)
		return fmt.Errorf("rebooting server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] rebooted server %s", id)
	return nil
}

// PauseServer pauses a server.
func PauseServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] pausing server %s", id)
	r := servers.Pause(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] pause server %s: %v", id, r.Err)
		return fmt.Errorf("pausing server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] paused server %s", id)
	return nil
}

// UnpauseServer unpauses a server.
func UnpauseServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] unpausing server %s", id)
	r := servers.Unpause(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] unpause server %s: %v", id, r.Err)
		return fmt.Errorf("unpausing server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] unpaused server %s", id)
	return nil
}

// SuspendServer suspends a server.
func SuspendServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] suspending server %s", id)
	r := servers.Suspend(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] suspend server %s: %v", id, r.Err)
		return fmt.Errorf("suspending server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] suspended server %s", id)
	return nil
}

// ResumeServer resumes a suspended server.
func ResumeServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] resuming server %s", id)
	r := servers.Resume(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] resume server %s: %v", id, r.Err)
		return fmt.Errorf("resuming server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] resumed server %s", id)
	return nil
}

// ShelveServer shelves a server.
func ShelveServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] shelving server %s", id)
	r := servers.Shelve(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] shelve server %s: %v", id, r.Err)
		return fmt.Errorf("shelving server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] shelved server %s", id)
	return nil
}

// UnshelveServer unshelves a server.
func UnshelveServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] unshelving server %s", id)
	r := servers.Unshelve(ctx, client, id, servers.UnshelveOpts{})
	if r.Err != nil {
		shared.Debugf("[compute] unshelve server %s: %v", id, r.Err)
		return fmt.Errorf("unshelving server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] unshelved server %s", id)
	return nil
}

// StopServer stops (shuts down) a server.
func StopServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] stopping server %s", id)
	r := servers.Stop(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] stop server %s: %v", id, r.Err)
		return fmt.Errorf("stopping server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] stopped server %s", id)
	return nil
}

// StartServer starts a stopped server.
func StartServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] starting server %s", id)
	r := servers.Start(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] start server %s: %v", id, r.Err)
		return fmt.Errorf("starting server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] started server %s", id)
	return nil
}

// LockServer prevents modifications to a server.
func LockServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] locking server %s", id)
	r := servers.Lock(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] lock server %s: %v", id, r.Err)
		return fmt.Errorf("locking server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] locked server %s", id)
	return nil
}

// UnlockServer removes the lock on a server.
func UnlockServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] unlocking server %s", id)
	r := servers.Unlock(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] unlock server %s: %v", id, r.Err)
		return fmt.Errorf("unlocking server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] unlocked server %s", id)
	return nil
}

// ResizeServer resizes a server to a new flavor.
func ResizeServer(ctx context.Context, client *gophercloud.ServiceClient, id, flavorRef string) error {
	shared.Debugf("[compute] resizing server %s to flavor %s", id, flavorRef)
	r := servers.Resize(ctx, client, id, servers.ResizeOpts{FlavorRef: flavorRef})
	if r.Err != nil {
		shared.Debugf("[compute] resize server %s: %v", id, r.Err)
		return fmt.Errorf("resizing server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] resized server %s", id)
	return nil
}

// ConfirmResize confirms a server resize.
func ConfirmResize(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] confirming resize for server %s", id)
	r := servers.ConfirmResize(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] confirm resize server %s: %v", id, r.Err)
		return fmt.Errorf("confirming resize for server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] confirmed resize for server %s", id)
	return nil
}

// RevertResize reverts a server resize.
func RevertResize(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] reverting resize for server %s", id)
	r := servers.RevertResize(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] revert resize server %s: %v", id, r.Err)
		return fmt.Errorf("reverting resize for server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] reverted resize for server %s", id)
	return nil
}

// CreateSnapshot creates an image snapshot of a server.
func CreateSnapshot(ctx context.Context, client *gophercloud.ServiceClient, id, snapshotName string) error {
	shared.Debugf("[compute] creating snapshot %q of server %s", snapshotName, id)
	r := servers.CreateImage(ctx, client, id, servers.CreateImageOpts{Name: snapshotName})
	if r.Err != nil {
		if gophercloud.ResponseCodeIs(r.Err, 409) {
			shared.Debugf("[compute] create snapshot server %s: snapshot already in progress", id)
			return fmt.Errorf("server already has a snapshot in progress")
		}
		shared.Debugf("[compute] create snapshot server %s: %v", id, r.Err)
		return fmt.Errorf("creating snapshot of server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] created snapshot %q of server %s", snapshotName, id)
	return nil
}

// RebuildServer rebuilds a server with a new image.
func RebuildServer(ctx context.Context, client *gophercloud.ServiceClient, id, imageRef string) error {
	shared.Debugf("[compute] rebuilding server %s with image %s", id, imageRef)
	_, err := servers.Rebuild(ctx, client, id, servers.RebuildOpts{ImageRef: imageRef}).Extract()
	if err != nil {
		shared.Debugf("[compute] rebuild server %s: %v", id, err)
		return fmt.Errorf("rebuilding server %s: %w", id, err)
	}
	shared.Debugf("[compute] rebuilt server %s", id)
	return nil
}

// RescueServer places a server into RESCUE mode and returns the admin password.
func RescueServer(ctx context.Context, client *gophercloud.ServiceClient, id string) (string, error) {
	shared.Debugf("[compute] rescuing server %s", id)
	r := servers.Rescue(ctx, client, id, servers.RescueOpts{})
	adminPass, err := r.Extract()
	if err != nil {
		shared.Debugf("[compute] rescue server %s: %v", id, err)
		return "", fmt.Errorf("rescuing server %s: %w", id, err)
	}
	shared.Debugf("[compute] rescued server %s", id)
	return adminPass, nil
}

// UnrescueServer returns a server from RESCUE mode.
func UnrescueServer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[compute] unrescuing server %s", id)
	r := servers.Unrescue(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[compute] unrescue server %s: %v", id, r.Err)
		return fmt.Errorf("unrescuing server %s: %w", id, r.Err)
	}
	shared.Debugf("[compute] unrescued server %s", id)
	return nil
}

// RenameServer updates a server's name.
func RenameServer(ctx context.Context, client *gophercloud.ServiceClient, id, newName string) error {
	shared.Debugf("[compute] renaming server %s to %q", id, newName)
	_, err := servers.Update(ctx, client, id, servers.UpdateOpts{Name: newName}).Extract()
	if err != nil {
		shared.Debugf("[compute] rename server %s: %v", id, err)
		return fmt.Errorf("renaming server %s: %w", id, err)
	}
	shared.Debugf("[compute] renamed server %s to %q", id, newName)
	return nil
}

// GetRemoteConsole retrieves a noVNC console URL for a server.
func GetRemoteConsole(ctx context.Context, client *gophercloud.ServiceClient, id string) (string, error) {
	shared.Debugf("[compute] getting remote console for server %s", id)
	result := remoteconsoles.Create(ctx, client, id, remoteconsoles.CreateOpts{
		Protocol: remoteconsoles.ConsoleProtocolVNC,
		Type:     remoteconsoles.ConsoleTypeNoVNC,
	})
	rc, err := result.Extract()
	if err != nil {
		shared.Debugf("[compute] get remote console server %s: %v", id, err)
		return "", fmt.Errorf("getting remote console for %s: %w", id, err)
	}
	shared.Debugf("[compute] got remote console for server %s", id)
	return rc.URL, nil
}

// GetPassword fetches the Windows admin password for a server.
//
// encrypted is always the base64-encoded blob returned by Nova (empty if the
// server has no generated password — e.g. a Linux instance or one not yet
// booted). When privKeyPath points to an unencrypted RSA PEM key that matches
// the keypair used at launch, plain is the decrypted cleartext; otherwise
// plain is empty. Decryption failures are returned as the error along with a
// non-empty encrypted so callers can still surface the blob for manual
// decryption.
func GetPassword(ctx context.Context, client *gophercloud.ServiceClient, id, privKeyPath string) (plain, encrypted string, err error) {
	shared.Debugf("[compute] getting password for server %s", id)
	r := servers.GetPassword(ctx, client, id)
	encrypted, err = r.ExtractPassword(nil)
	if err != nil {
		shared.Debugf("[compute] get password server %s: %v", id, err)
		return "", "", fmt.Errorf("getting password for %s: %w", id, err)
	}
	if encrypted == "" {
		shared.Debugf("[compute] get password server %s: no password set", id)
		return "", "", nil
	}
	if privKeyPath == "" {
		return "", encrypted, nil
	}
	key, err := loadRSAPrivateKey(privKeyPath)
	if err != nil {
		shared.Debugf("[compute] load private key %s: %v", privKeyPath, err)
		return "", encrypted, err
	}
	plain, err = r.ExtractPassword(key)
	if err != nil {
		shared.Debugf("[compute] decrypt password server %s: %v", id, err)
		return "", encrypted, fmt.Errorf("decrypting password: %w", err)
	}
	shared.Debugf("[compute] got password for server %s", id)
	return plain, encrypted, nil
}

// loadRSAPrivateKey reads an unencrypted RSA private key from a PEM file.
func loadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found in key file")
	}
	// x509.IsEncryptedPEMBlock is deprecated but the header check still works.
	if _, ok := block.Headers["DEK-Info"]; ok {
		return nil, errors.New("encrypted PEM keys are not supported")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("key is not RSA")
		}
		return rk, nil
	case "OPENSSH PRIVATE KEY":
		return nil, errors.New("OpenSSH-format keys are not supported; convert to PEM with: ssh-keygen -p -m PEM -f <key>")
	default:
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}

// GetConsoleOutput retrieves console output for a server.
func GetConsoleOutput(ctx context.Context, client *gophercloud.ServiceClient, id string, lines int) (string, error) {
	shared.Debugf("[compute] getting console output for server %s (lines=%d)", id, lines)
	r := servers.ShowConsoleOutput(ctx, client, id, servers.ShowConsoleOutputOpts{Length: lines})
	output, err := r.Extract()
	if err != nil {
		shared.Debugf("[compute] get console output server %s: %v", id, err)
		return "", fmt.Errorf("getting console output for %s: %w", id, err)
	}
	shared.Debugf("[compute] got console output for server %s", id)
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
		Metadata:   s.Metadata,
	}

	// Flavor — microversion 2.47+ embeds full flavor with original_name
	if name, ok := s.Flavor["original_name"].(string); ok {
		srv.FlavorName = name
	}
	if id, ok := s.Flavor["id"].(string); ok {
		srv.FlavorID = id
	}
	if vcpus, ok := s.Flavor["vcpus"].(float64); ok {
		srv.FlavorVCPUs = int(vcpus)
	}
	if ram, ok := s.Flavor["ram"].(float64); ok {
		srv.FlavorRAM = int(ram)
	}
	if disk, ok := s.Flavor["disk"].(float64); ok {
		srv.FlavorDisk = int(disk)
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
		srv.VolAttach = append(srv.VolAttach, VolumeAttachment{ID: v.ID})
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
