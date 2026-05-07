package volume

import (
	"context"
	"fmt"
	"time"

	"github.com/larkly/lazystack/internal/shared"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// VolumeAttachment represents a single volume-to-server attachment.
type VolumeAttachment struct {
	ServerID string
	Device   string
}

// Volume is a simplified block storage volume.
type Volume struct {
	ID               string
	Name             string
	Status           string
	Size             int // GB
	VolumeType       string
	AZ               string
	Bootable         string
	Encrypted        bool
	Multiattach      bool
	AttachedServerID string             // DEPRECATED: use Attachments + ServerID() instead
	AttachedDevice   string             // DEPRECATED: use Attachments + Device() instead
	Attachments      []VolumeAttachment // all current attachments (multiattach-aware)
	Created          time.Time
	Updated          time.Time
	Description      string
	Metadata         map[string]string
	SnapshotID       string
	SourceVolID      string
}

// ServerID returns the first attachment's server ID for backward compatibility.
func (v Volume) ServerID() string {
	if len(v.Attachments) > 0 {
		return v.Attachments[0].ServerID
	}
	return ""
}

// Device returns the first attachment's device path for backward compatibility.
func (v Volume) Device() string {
	if len(v.Attachments) > 0 {
		return v.Attachments[0].Device
	}
	return ""
}

// IsAttached returns whether the volume has any attachments.
func (v Volume) IsAttached() bool {
	return len(v.Attachments) > 0
}

// IsMultiAttached returns whether the volume is attached to multiple servers.
func (v Volume) IsMultiAttached() bool {
	return len(v.Attachments) > 1
}

// ListVolumes fetches all volumes.
func ListVolumes(ctx context.Context, client *gophercloud.ServiceClient) ([]Volume, error) {
	shared.Debugf("[volume] ListVolumes: starting")
	var result []Volume
	err := volumes.List(client, volumes.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := volumes.ExtractVolumes(page)
		if err != nil {
			return false, err
		}
		for _, v := range extracted {
			vol := Volume{
				ID:          v.ID,
				Name:        v.Name,
				Status:      v.Status,
				Size:        v.Size,
				VolumeType:  v.VolumeType,
				AZ:          v.AvailabilityZone,
				Bootable:    v.Bootable,
				Encrypted:   v.Encrypted,
				Multiattach: v.Multiattach,
				Created:     v.CreatedAt,
				Updated:     v.UpdatedAt,
				Description: v.Description,
				Metadata:    v.Metadata,
				SnapshotID:  v.SnapshotID,
				SourceVolID: v.SourceVolID,
			}
			for _, att := range v.Attachments {
				vol.Attachments = append(vol.Attachments, VolumeAttachment{ServerID: att.ServerID, Device: att.Device})
			}
			vol.AttachedServerID = vol.ServerID()
			vol.AttachedDevice = vol.Device()
			result = append(result, vol)
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[volume] ListVolumes: error: %v", err)
		return nil, fmt.Errorf("listing volumes: %w", err)
	}
	shared.Debugf("[volume] ListVolumes: success, count=%d", len(result))
	return result, nil
}

// GetVolume fetches a single volume by ID.
func GetVolume(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Volume, error) {
	shared.Debugf("[volume] GetVolume: starting, id=%s", id)
	v, err := volumes.Get(ctx, client, id).Extract()
	if err != nil {
		shared.Debugf("[volume] GetVolume: error: %v", err)
		return nil, fmt.Errorf("getting volume %s: %w", id, err)
	}
	vol := &Volume{
		ID:          v.ID,
		Name:        v.Name,
		Status:      v.Status,
		Size:        v.Size,
		VolumeType:  v.VolumeType,
		AZ:          v.AvailabilityZone,
		Bootable:    v.Bootable,
		Encrypted:   v.Encrypted,
		Multiattach: v.Multiattach,
		Created:     v.CreatedAt,
		Updated:     v.UpdatedAt,
		Description: v.Description,
		Metadata:    v.Metadata,
		SnapshotID:  v.SnapshotID,
		SourceVolID: v.SourceVolID,
	}
	for _, att := range v.Attachments {
		vol.Attachments = append(vol.Attachments, VolumeAttachment{ServerID: att.ServerID, Device: att.Device})
	}
	vol.AttachedServerID = vol.ServerID()
	vol.AttachedDevice = vol.Device()
	shared.Debugf("[volume] GetVolume: success, id=%s name=%s", vol.ID, vol.Name)
	return vol, nil
}

// CreateVolume creates a new volume.
func CreateVolume(ctx context.Context, client *gophercloud.ServiceClient, opts volumes.CreateOpts) (*Volume, error) {
	shared.Debugf("[volume] CreateVolume: starting, name=%s size=%d", opts.Name, opts.Size)
	r := volumes.Create(ctx, client, opts, nil)
	v, err := r.Extract()
	if err != nil {
		shared.Debugf("[volume] CreateVolume: error: %v", err)
		return nil, fmt.Errorf("creating volume: %w", err)
	}
	vol := &Volume{
		ID:         v.ID,
		Name:       v.Name,
		Status:     v.Status,
		Size:       v.Size,
		VolumeType: v.VolumeType,
	}
	shared.Debugf("[volume] CreateVolume: success, id=%s name=%s", vol.ID, vol.Name)
	return vol, nil
}

// VolumeType is a simplified block storage volume type.
type VolumeType struct {
	ID   string
	Name string
}

// ListVolumeTypes fetches all volume types.
func ListVolumeTypes(ctx context.Context, client *gophercloud.ServiceClient) ([]VolumeType, error) {
	shared.Debugf("[volume] ListVolumeTypes: starting")
	url := client.ServiceURL("types")
	var body struct {
		VolumeTypes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"volume_types"`
	}
	resp, err := client.Get(ctx, url, &body, nil)
	if err != nil {
		shared.Debugf("[volume] ListVolumeTypes: error: %v", err)
		return nil, fmt.Errorf("listing volume types: %w", err)
	}
	resp.Body.Close()

	result := make([]VolumeType, len(body.VolumeTypes))
	for i, vt := range body.VolumeTypes {
		result[i] = VolumeType{ID: vt.ID, Name: vt.Name}
	}
	shared.Debugf("[volume] ListVolumeTypes: success, count=%d", len(result))
	return result, nil
}

// DeleteVolume deletes a volume.
func DeleteVolume(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[volume] DeleteVolume: starting, id=%s", id)
	r := volumes.Delete(ctx, client, id, volumes.DeleteOpts{})
	if r.Err != nil {
		shared.Debugf("[volume] DeleteVolume: error: %v", r.Err)
		return fmt.Errorf("deleting volume %s: %w", id, r.Err)
	}
	shared.Debugf("[volume] DeleteVolume: success, id=%s", id)
	return nil
}

// AttachVolume attaches a volume to a server using Nova's os-volume_attachments.
func AttachVolume(ctx context.Context, computeClient *gophercloud.ServiceClient, serverID, volumeID string) error {
	shared.Debugf("[volume] AttachVolume: starting, serverID=%s volumeID=%s", serverID, volumeID)
	body := map[string]interface{}{
		"volumeAttachment": map[string]interface{}{
			"volumeId": volumeID,
		},
	}
	resp, err := computeClient.Post(ctx, computeClient.ServiceURL("servers", serverID, "os-volume_attachments"), body, nil, &gophercloud.RequestOpts{
		OkCodes: []int{200, 201, 202},
	})
	if err != nil {
		shared.Debugf("[volume] AttachVolume: error: %v", err)
		return fmt.Errorf("attaching volume %s to server %s: %w", volumeID, serverID, err)
	}
	resp.Body.Close()
	shared.Debugf("[volume] AttachVolume: success, serverID=%s volumeID=%s", serverID, volumeID)
	return nil
}

// DetachVolume detaches a volume from a server.
func DetachVolume(ctx context.Context, computeClient *gophercloud.ServiceClient, serverID, volumeID string) error {
	shared.Debugf("[volume] DetachVolume: starting, serverID=%s volumeID=%s", serverID, volumeID)
	resp, err := computeClient.Delete(ctx, computeClient.ServiceURL("servers", serverID, "os-volume_attachments", volumeID), nil)
	if err != nil {
		shared.Debugf("[volume] DetachVolume: error: %v", err)
		return fmt.Errorf("detaching volume %s from server %s: %w", volumeID, serverID, err)
	}
	resp.Body.Close()
	shared.Debugf("[volume] DetachVolume: success, serverID=%s volumeID=%s", serverID, volumeID)
	return nil
}
