package volume

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
)

// volumesFixture is a minimal Cinder v3 paginated volume list response with
// multi-attachment support and deprecated fields.
const volumesFixture = `{
  "volumes": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "name": "vol-multiattach",
      "status": "in-use",
      "size": 100,
      "volume_type": "ssd",
      "availability_zone": "nova",
      "bootable": "true",
      "encrypted": true,
      "multiattach": true,
      "attachments": [
        {"server_id": "srv-001", "device": "/dev/vdb"},
        {"server_id": "srv-002", "device": "/dev/vdc"}
      ],
      "description": "Shared storage volume",
      "metadata": {"env": "prod"},
      "snapshot_id": "snap-001",
      "source_volid": "vol-orig-001"
    },
    {
      "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "name": "vol-single-attach",
      "status": "available",
      "size": 50,
      "volume_type": "hdd",
      "availability_zone": "nova",
      "bootable": "false",
      "encrypted": false,
      "multiattach": false,
      "attachments": [],
      "description": "",
      "metadata": {},
      "snapshot_id": "",
      "source_volid": ""
    }
  ]
}`

// getVolumeFixture is a single-volume detail response (non-paginated GET).
const getVolumeFixture = `{
  "volume": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "name": "vol-multiattach",
    "status": "in-use",
    "size": 100,
    "volume_type": "ssd",
    "availability_zone": "nova",
    "bootable": "true",
    "encrypted": true,
    "multiattach": true,
    "attachments": [
      {"server_id": "srv-001", "device": "/dev/vdb"},
      {"server_id": "srv-002", "device": "/dev/vdc"},
      {"server_id": "srv-003", "device": "/dev/vdd"}
    ],
    "description": "Shared storage volume",
    "metadata": {"env": "prod"},
    "snapshot_id": "snap-001",
    "source_volid": "vol-orig-001"
  }
}`

func fakeBlockStorageClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListVolumes(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "volumes") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(volumesFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeBlockStorageClient(handler)
	ctx := context.Background()

	vols, err := ListVolumes(ctx, client)
	if err != nil {
		t.Fatalf("ListVolumes() error: %v", err)
	}
	if len(vols) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(vols))
	}

	// Volume 1: multi-attach, in-use, encrypted, bootable
	v1 := vols[0]
	if v1.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("unexpected ID: %s", v1.ID)
	}
	if v1.Name != "vol-multiattach" {
		t.Errorf("unexpected Name: %s", v1.Name)
	}
	if v1.Status != "in-use" {
		t.Errorf("unexpected Status: %s", v1.Status)
	}
	if v1.Size != 100 {
		t.Errorf("unexpected Size: %d", v1.Size)
	}
	if v1.VolumeType != "ssd" {
		t.Errorf("unexpected VolumeType: %s", v1.VolumeType)
	}
	if v1.Bootable != "true" {
		t.Errorf("unexpected Bootable: %s", v1.Bootable)
	}
	if !v1.Encrypted {
		t.Errorf("expected Encrypted=true")
	}
	if !v1.Multiattach {
		t.Errorf("expected Multiattach=true")
	}
	if v1.Description != "Shared storage volume" {
		t.Errorf("unexpected Description: %s", v1.Description)
	}
	if v1.SnapshotID != "snap-001" {
		t.Errorf("unexpected SnapshotID: %s", v1.SnapshotID)
	}
	if v1.SourceVolID != "vol-orig-001" {
		t.Errorf("unexpected SourceVolID: %s", v1.SourceVolID)
	}

	// Verify multi-attachments
	if len(v1.Attachments) != 2 {
		t.Fatalf("expected 2 Attachments, got %d", len(v1.Attachments))
	}
	if v1.Attachments[0].ServerID != "srv-001" {
		t.Errorf("unexpected attachment[0] ServerID: %s", v1.Attachments[0].ServerID)
	}
	if v1.Attachments[0].Device != "/dev/vdb" {
		t.Errorf("unexpected attachment[0] Device: %s", v1.Attachments[0].Device)
	}
	if v1.Attachments[1].ServerID != "srv-002" {
		t.Errorf("unexpected attachment[1] ServerID: %s", v1.Attachments[1].ServerID)
	}
	if v1.Attachments[1].Device != "/dev/vdc" {
		t.Errorf("unexpected attachment[1] Device: %s", v1.Attachments[1].Device)
	}

	// Verify IsAttached and IsMultiAttached helpers
	if !v1.IsAttached() {
		t.Errorf("expected IsAttached()=true")
	}
	if !v1.IsMultiAttached() {
		t.Errorf("expected IsMultiAttached()=true")
	}

	// Verify deprecated backward-compat fields
	if v1.AttachedServerID != "srv-001" {
		t.Errorf("unexpected deprecated AttachedServerID: %s", v1.AttachedServerID)
	}
	if v1.AttachedDevice != "/dev/vdb" {
		t.Errorf("unexpected deprecated AttachedDevice: %s", v1.AttachedDevice)
	}

	// Verify ServerID() and Device() helpers
	if v1.ServerID() != "srv-001" {
		t.Errorf("unexpected ServerID(): %s", v1.ServerID())
	}
	if v1.Device() != "/dev/vdb" {
		t.Errorf("unexpected Device(): %s", v1.Device())
	}

	// Volume 2: no attachments, available, unencrypted
	v2 := vols[1]
	if v2.ID != "b2c3d4e5-f6a7-8901-bcde-f12345678901" {
		t.Errorf("unexpected ID: %s", v2.ID)
	}
	if v2.Name != "vol-single-attach" {
		t.Errorf("unexpected Name: %s", v2.Name)
	}
	if v2.Status != "available" {
		t.Errorf("unexpected Status: %s", v2.Status)
	}
	if v2.Encrypted {
		t.Errorf("expected Encrypted=false")
	}
	if v2.Multiattach {
		t.Errorf("expected Multiattach=false")
	}
	if v2.IsAttached() {
		t.Errorf("expected IsAttached()=false")
	}
	if v2.IsMultiAttached() {
		t.Errorf("expected IsMultiAttached()=false")
	}
	if v2.ServerID() != "" {
		t.Errorf("expected empty ServerID(): %s", v2.ServerID())
	}
	if v2.Device() != "" {
		t.Errorf("expected empty Device(): %s", v2.Device())
	}
	if len(v2.Attachments) != 0 {
		t.Errorf("expected no Attachments, got %d", len(v2.Attachments))
	}
}

func TestGetVolume(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "volumes") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(getVolumeFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeBlockStorageClient(handler)
	ctx := context.Background()

	vol, err := GetVolume(ctx, client, "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("GetVolume() error: %v", err)
	}
	if vol == nil {
		t.Fatal("expected non-nil volume")
	}

	if vol.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("unexpected ID: %s", vol.ID)
	}
	if vol.Name != "vol-multiattach" {
		t.Errorf("unexpected Name: %s", vol.Name)
	}
	if vol.Status != "in-use" {
		t.Errorf("unexpected Status: %s", vol.Status)
	}
	if vol.Size != 100 {
		t.Errorf("unexpected Size: %d", vol.Size)
	}
	if vol.VolumeType != "ssd" {
		t.Errorf("unexpected VolumeType: %s", vol.VolumeType)
	}
	if !vol.Encrypted {
		t.Errorf("expected Encrypted=true")
	}
	if !vol.Multiattach {
		t.Errorf("expected Multiattach=true")
	}

	// GetVolume detail has 3 attachments vs list's 2
	if len(vol.Attachments) != 3 {
		t.Fatalf("expected 3 Attachments in detail view, got %d", len(vol.Attachments))
	}
	if !vol.IsAttached() {
		t.Errorf("expected IsAttached()=true")
	}
	if !vol.IsMultiAttached() {
		t.Errorf("expected IsMultiAttached()=true")
	}
	if vol.ServerID() != "srv-001" {
		t.Errorf("unexpected ServerID(): %s", vol.ServerID())
	}
	if vol.Attachments[2].ServerID != "srv-003" {
		t.Errorf("unexpected attachment[2] ServerID: %s", vol.Attachments[2].ServerID)
	}
	if vol.Attachments[2].Device != "/dev/vdd" {
		t.Errorf("unexpected attachment[2] Device: %s", vol.Attachments[2].Device)
	}

	// Verify deprecated compat fields consistent with first attachment
	if vol.AttachedServerID != "srv-001" {
		t.Errorf("unexpected deprecated AttachedServerID: %s", vol.AttachedServerID)
	}
	if vol.AttachedDevice != "/dev/vdb" {
		t.Errorf("unexpected deprecated AttachedDevice: %s", vol.AttachedDevice)
	}
}

// volumeTypesPage1/2 simulate two pages of a paginated Cinder volume type
// listing; page 1 carries a next link the paginator must follow.
const volumeTypesPage1 = `{
  "volume_types": [
    {"id": "vt-ssd-0001", "name": "ssd"},
    {"id": "vt-hdd-0002", "name": "hdd"}
  ],
  "volume_type_links": [
    {"rel": "next", "href": "PLACEHOLDER?page=2"}
  ]
}`

const volumeTypesPage2 = `{
  "volume_types": [
    {"id": "vt-nvme-0003", "name": "nvme"}
  ]
}`

func TestListVolumeTypesPagination(t *testing.T) {
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/types") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(volumeTypesPage2))
		default:
			// Rewrite the placeholder link to the absolute URL of this server.
			body := strings.Replace(volumeTypesPage1, "PLACEHOLDER",
				"http://"+r.Host+r.URL.Path, 1)
			w.Write([]byte(body))
		}
	})

	client := fakeBlockStorageClient(handler)
	ctx := context.Background()

	types, err := ListVolumeTypes(ctx, client)
	if err != nil {
		t.Fatalf("ListVolumeTypes() error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 paginated GETs, got %d", calls)
	}
	if len(types) != 3 {
		t.Fatalf("expected 3 volume types across 2 pages, got %d", len(types))
	}
	want := []VolumeType{
		{ID: "vt-ssd-0001", Name: "ssd"},
		{ID: "vt-hdd-0002", Name: "hdd"},
		{ID: "vt-nvme-0003", Name: "nvme"},
	}
	for i, exp := range want {
		if types[i] != exp {
			t.Errorf("types[%d] = %+v, want %+v", i, types[i], exp)
		}
	}
}

func TestNilClientGuards(t *testing.T) {
	const cinderMsg = "block storage (Cinder) service is not available in this cloud"
	const novaMsg = "compute (Nova) service is not available in this cloud"

	tests := []struct {
		name    string
		call    func() error
		wantMsg string
	}{
		{"ListVolumes", func() error {
			_, err := ListVolumes(context.Background(), nil)
			return err
		}, cinderMsg},
		{"GetVolume", func() error {
			_, err := GetVolume(context.Background(), nil, "vol-1")
			return err
		}, cinderMsg},
		{"CreateVolume", func() error {
			_, err := CreateVolume(context.Background(), nil, volumes.CreateOpts{})
			return err
		}, cinderMsg},
		{"ListVolumeTypes", func() error {
			_, err := ListVolumeTypes(context.Background(), nil)
			return err
		}, cinderMsg},
		{"DeleteVolume", func() error {
			return DeleteVolume(context.Background(), nil, "vol-1")
		}, cinderMsg},
		{"AttachVolume", func() error {
			_, err := AttachVolume(context.Background(), nil, "srv-1", "vol-1")
			return err
		}, novaMsg},
		{"DetachVolume", func() error {
			return DetachVolume(context.Background(), nil, "srv-1", "vol-1")
		}, novaMsg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected error for nil client, got nil")
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestAttachVolumeReturnsAttachmentID(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "os-volume_attachments") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"volumeAttachment": {"id": "att-42", "volumeId": "vol-1", "serverId": "srv-1", "device": "/dev/vdb"}}`))
	})

	client := fakeBlockStorageClient(handler)
	attID, err := AttachVolume(context.Background(), client, "srv-1", "vol-1")
	if err != nil {
		t.Fatalf("AttachVolume() error: %v", err)
	}
	if attID != "att-42" {
		t.Errorf("attachment ID = %q, want %q", attID, "att-42")
	}
	if !strings.HasSuffix(gotPath, "/servers/srv-1/os-volume_attachments") {
		t.Errorf("unexpected request path: %s", gotPath)
	}
}

func TestDetachVolumeResolvesAttachmentID(t *testing.T) {
	var deletePath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "os-volume_attachments"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"volumeAttachments": [{"id": "att-42", "volumeId": "vol-1", "serverId": "srv-1", "device": "/dev/vdb"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "os-volume_attachments/att-42"):
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	client := fakeBlockStorageClient(handler)
	if err := DetachVolume(context.Background(), client, "srv-1", "vol-1"); err != nil {
		t.Fatalf("DetachVolume() error: %v", err)
	}
	if deletePath == "" {
		t.Error("expected DELETE by attachment ID att-42, request never seen")
	}
}

func TestDetachVolumeFallsBackToVolumeID(t *testing.T) {
	var deletePath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "os-volume_attachments"):
			w.Header().Set("Content-Type", "application/json")
			// List contains a different volume only; no match for vol-1.
			w.Write([]byte(`{"volumeAttachments": [{"id": "att-99", "volumeId": "vol-other", "serverId": "srv-1"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "os-volume_attachments/vol-1"):
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	client := fakeBlockStorageClient(handler)
	if err := DetachVolume(context.Background(), client, "srv-1", "vol-1"); err != nil {
		t.Fatalf("DetachVolume() error: %v", err)
	}
	if deletePath == "" {
		t.Error("expected fallback DELETE by volume ID vol-1, request never seen")
	}
}
