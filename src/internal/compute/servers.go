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
	Status    string
	FlavorID  string
	ImageID   string
	IP        string
	KeyName   string
	Created   time.Time
	TenantID  string
	AZ        string
	VolAttach []string
	SecGroups []string
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

func mapServer(s servers.Server) Server {
	srv := Server{
		ID:       s.ID,
		Name:     s.Name,
		Status:   s.Status,
		KeyName:  s.KeyName,
		Created:  s.Created,
		TenantID: s.TenantID,
	}

	// Flavor
	if id, ok := s.Flavor["id"].(string); ok {
		srv.FlavorID = id
	}

	// Image — can be empty for boot-from-volume
	if imgMap := s.Image; len(imgMap) > 0 {
		if id, ok := imgMap["id"].(string); ok {
			srv.ImageID = id
		}
	}

	// Extract first IP
	srv.IP = extractFirstIP(s.Addresses)

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

func extractFirstIP(addresses map[string]interface{}) string {
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
			if addr, ok := addrMap["addr"].(string); ok {
				return addr
			}
		}
	}
	return ""
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
