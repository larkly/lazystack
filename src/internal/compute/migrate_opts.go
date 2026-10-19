package compute

import (
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// evacuateOpts builds the evacuate action body for the client's
// microversion. gophercloud's EvacuateOpts always sends onSharedStorage,
// which Nova removed in 2.14 and rejects with HTTP 400 from then on.
type evacuateOpts struct {
	microversion    string
	host            string
	adminPass       string
	onSharedStorage bool // only sent below 2.14
}

// ToEvacuateMap implements servers.EvacuateOptsBuilder.
func (o evacuateOpts) ToEvacuateMap() (map[string]any, error) {
	if !microversionAtLeast(o.microversion, "2.14") {
		return servers.EvacuateOpts{
			Host:            o.host,
			OnSharedStorage: o.onSharedStorage,
			AdminPass:       o.adminPass,
		}.ToEvacuateMap()
	}
	body := map[string]any{}
	if o.host != "" {
		body["host"] = o.host
	}
	if o.adminPass != "" {
		body["adminPass"] = o.adminPass
	}
	return map[string]any{"evacuate": body}, nil
}

// liveMigrateOpts builds the os-migrateLive action body for the client's
// microversion. From 2.25 Nova accepts block_migration "auto", which picks
// block migration for local-storage instances; disk_over_commit is gone.
// Below 2.25 the schema requires both booleans.
type liveMigrateOpts struct {
	microversion string
	host         string // empty lets the scheduler choose
}

// ToLiveMigrateMap implements servers.LiveMigrateOptsBuilder.
func (o liveMigrateOpts) ToLiveMigrateMap() (map[string]any, error) {
	var host any
	if o.host != "" {
		host = o.host
	}
	body := map[string]any{"host": host}
	if microversionAtLeast(o.microversion, "2.25") {
		body["block_migration"] = "auto"
	} else {
		body["block_migration"] = false
		body["disk_over_commit"] = false
	}
	return map[string]any{"os-migrateLive": body}, nil
}
