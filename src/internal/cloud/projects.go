package cloud

import (
	"context"
	"fmt"

	"github.com/larkly/lazystack/internal/shared"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Project is a simplified Keystone project.
type Project struct {
	ID   string
	Name string
}

// ListAccessibleProjects returns projects the current user can access.
func ListAccessibleProjects(ctx context.Context, providerClient *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) ([]Project, error) {
	shared.Debugf("[cloud] ListAccessibleProjects: starting")
	identityClient, err := openstack.NewIdentityV3(providerClient, eo)
	if err != nil {
		shared.Debugf("[cloud] ListAccessibleProjects: identity client error: %v", err)
		return nil, fmt.Errorf("identity client: %w", err)
	}

	var result []Project
	err = projects.ListAvailable(identityClient).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := projects.ExtractProjects(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			if p.Enabled {
				result = append(result, Project{ID: p.ID, Name: p.Name})
			}
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[cloud] ListAccessibleProjects: error: %v", err)
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	shared.Debugf("[cloud] ListAccessibleProjects: success, count=%d", len(result))
	return result, nil
}
