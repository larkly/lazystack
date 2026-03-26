package volume

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

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
	AttachedServerID string
	AttachedDevice   string
	Created          time.Time
	Updated          time.Time
	Description      string
	Metadata         map[string]string
	SnapshotID       string
	SourceVolID      string
}

// ListVolumes fetches all volumes.
func ListVolumes(ctx context.Context, client *gophercloud.ServiceClient) ([]Volume, error) {
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
			if len(v.Attachments) > 0 {
				vol.AttachedServerID = v.Attachments[0].ServerID
				vol.AttachedDevice = v.Attachments[0].Device
			}
			result = append(result, vol)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}
	return result, nil
}

// GetVolume fetches a single volume by ID.
func GetVolume(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Volume, error) {
	v, err := volumes.Get(ctx, client, id).Extract()
	if err != nil {
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
	if len(v.Attachments) > 0 {
		vol.AttachedServerID = v.Attachments[0].ServerID
		vol.AttachedDevice = v.Attachments[0].Device
	}
	return vol, nil
}

// CreateVolume creates a new volume.
func CreateVolume(ctx context.Context, client *gophercloud.ServiceClient, opts volumes.CreateOpts) (*Volume, error) {
	r := volumes.Create(ctx, client, opts, nil)
	v, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}
	vol := &Volume{
		ID:         v.ID,
		Name:       v.Name,
		Status:     v.Status,
		Size:       v.Size,
		VolumeType: v.VolumeType,
	}
	return vol, nil
}

// VolumeType is a simplified block storage volume type.
type VolumeType struct {
	ID   string
	Name string
}

// ListVolumeTypes fetches all volume types.
func ListVolumeTypes(ctx context.Context, client *gophercloud.ServiceClient) ([]VolumeType, error) {
	url := client.ServiceURL("types")
	var body struct {
		VolumeTypes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"volume_types"`
	}
	resp, err := client.Get(ctx, url, &body, nil)
	if err != nil {
		return nil, fmt.Errorf("listing volume types: %w", err)
	}
	resp.Body.Close()

	result := make([]VolumeType, len(body.VolumeTypes))
	for i, vt := range body.VolumeTypes {
		result[i] = VolumeType{ID: vt.ID, Name: vt.Name}
	}
	return result, nil
}

// DeleteVolume deletes a volume.
func DeleteVolume(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := volumes.Delete(ctx, client, id, volumes.DeleteOpts{})
	if r.Err != nil {
		return fmt.Errorf("deleting volume %s: %w", id, r.Err)
	}
	return nil
}

// AttachVolume attaches a volume to a server using Nova's os-volume_attachments.
func AttachVolume(ctx context.Context, computeClient *gophercloud.ServiceClient, serverID, volumeID string) error {
	body := map[string]interface{}{
		"volumeAttachment": map[string]interface{}{
			"volumeId": volumeID,
		},
	}
	resp, err := computeClient.Post(ctx, computeClient.ServiceURL("servers", serverID, "os-volume_attachments"), body, nil, &gophercloud.RequestOpts{
		OkCodes: []int{200, 201, 202},
	})
	if err != nil {
		return fmt.Errorf("attaching volume %s to server %s: %w", volumeID, serverID, err)
	}
	resp.Body.Close()
	return nil
}

// DetachVolume detaches a volume from a server.
func DetachVolume(ctx context.Context, computeClient *gophercloud.ServiceClient, serverID, volumeID string) error {
	resp, err := computeClient.Delete(ctx, computeClient.ServiceURL("servers", serverID, "os-volume_attachments", volumeID), nil)
	if err != nil {
		return fmt.Errorf("detaching volume %s from server %s: %w", volumeID, serverID, err)
	}
	resp.Body.Close()
	return nil
}
