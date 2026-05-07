package compute

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// hypervisorsFixture is a minimal nova API paginated hypervisor list response.
const hypervisorsFixture = `{
  "hypervisors": [
    {
      "id": 1,
      "hypervisor_hostname": "compute01.example.com",
      "hypervisor_type": "QEMU",
      "hypervisor_version": 6004000,
      "status": "enabled",
      "state": "up",
      "vcpus": 16,
      "vcpus_used": 8,
      "memory_mb": 65536,
      "memory_mb_used": 32768,
      "local_gb": 512,
      "local_gb_used": 200,
      "free_disk_gb": 312,
      "free_ram_mb": 32768,
      "current_workload": 5,
      "disk_available_least": 300,
      "running_vms": 5,
      "host_ip": "192.168.1.10",
      "cpu_info": {
        "vendor": "Intel",
        "arch": "x86_64",
        "model": "Haswell",
        "features": ["mmx", "sse"],
        "topology": {"cells": 1, "sockets": 2, "cores": 8, "threads": 2}
      },
      "service": {"host": "compute01", "id": "s1", "disabled_reason": null}
    },
    {
      "id": 2,
      "hypervisor_hostname": "compute02.example.com",
      "hypervisor_type": "QEMU",
      "hypervisor_version": 6004000,
      "status": "enabled",
      "state": "up",
      "vcpus": 24,
      "vcpus_used": 18,
      "memory_mb": 131072,
      "memory_mb_used": 98304,
      "local_gb": 1024,
      "local_gb_used": 450,
      "free_disk_gb": 574,
      "free_ram_mb": 32768,
      "current_workload": 12,
      "disk_available_least": 550,
      "running_vms": 12,
      "host_ip": "192.168.1.11",
      "cpu_info": {
        "vendor": "AMD",
        "arch": "x86_64",
        "model": "EPYC",
        "features": ["mmx", "sse", "avx"],
        "topology": {"cells": 1, "sockets": 2, "cores": 12, "threads": 2}
      },
      "service": {"host": "compute02", "id": "s2", "disabled_reason": null}
    },
    {
      "id": 3,
      "hypervisor_hostname": "compute03.example.com",
      "hypervisor_type": "QEMU",
      "hypervisor_version": 6004000,
      "status": "disabled",
      "state": "down",
      "vcpus": 12,
      "vcpus_used": 0,
      "memory_mb": 32768,
      "memory_mb_used": 0,
      "local_gb": 256,
      "local_gb_used": 0,
      "free_disk_gb": 256,
      "free_ram_mb": 32768,
      "current_workload": 0,
      "disk_available_least": 255,
      "running_vms": 0,
      "host_ip": "192.168.1.12",
      "cpu_info": {
        "vendor": "Intel",
        "arch": "x86_64",
        "model": "Skylake",
        "features": ["mmx"],
        "topology": {"cells": 1, "sockets": 1, "cores": 6, "threads": 2}
      },
      "service": {"host": "compute03", "id": "s3", "disabled_reason": "maintenance"}
    }
  ]
}`

func TestListHypervisors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/os-hypervisors/detail") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(hypervisorsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	hypervisors, err := ListHypervisors(ctx, client)
	if err != nil {
		t.Fatalf("ListHypervisors() error: %v", err)
	}
	if len(hypervisors) != 3 {
		t.Fatalf("expected 3 hypervisors, got %d", len(hypervisors))
	}

	// Verify compute01
	h1 := hypervisors[0]
	if h1.ID != "1" {
		t.Errorf("unexpected ID: %s", h1.ID)
	}
	if h1.Name != "compute01.example.com" {
		t.Errorf("unexpected Name: %s", h1.Name)
	}
	if h1.Type != "QEMU" {
		t.Errorf("unexpected Type: %s", h1.Type)
	}
	if h1.Status != "enabled" {
		t.Errorf("unexpected Status: %s", h1.Status)
	}
	if h1.State != "up" {
		t.Errorf("unexpected State: %s", h1.State)
	}
	if h1.VCPUs != 16 {
		t.Errorf("unexpected VCPUs: %d", h1.VCPUs)
	}
	if h1.VCPUsUsed != 8 {
		t.Errorf("unexpected VCPUsUsed: %d", h1.VCPUsUsed)
	}
	if h1.MemoryMB != 65536 {
		t.Errorf("unexpected MemoryMB: %d", h1.MemoryMB)
	}
	if h1.MemoryMBUsed != 32768 {
		t.Errorf("unexpected MemoryMBUsed: %d", h1.MemoryMBUsed)
	}
	if h1.LocalGB != 512 {
		t.Errorf("unexpected LocalGB: %d", h1.LocalGB)
	}
	if h1.LocalGBUsed != 200 {
		t.Errorf("unexpected LocalGBUsed: %d", h1.LocalGBUsed)
	}
	if h1.RunningVMs != 5 {
		t.Errorf("unexpected RunningVMs: %d", h1.RunningVMs)
	}
	if h1.HostIP != "192.168.1.10" {
		t.Errorf("unexpected HostIP: %s", h1.HostIP)
	}

	// Verify disabled/down hypervisor
	h3 := hypervisors[2]
	if h3.Status != "disabled" {
		t.Errorf("unexpected third Status: %s", h3.Status)
	}
	if h3.State != "down" {
		t.Errorf("unexpected third State: %s", h3.State)
	}
	if h3.RunningVMs != 0 {
		t.Errorf("expected 0 RunningVMs for down hypervisor, got %d", h3.RunningVMs)
	}
}
