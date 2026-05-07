package compute

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/users"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// User is a simplified Keystone user.
type User struct {
	ID          string
	Name        string
	Email       string
	Enabled     bool
	Description string
	DomainID    string
}

// ListUsers returns all users visible to the authenticated identity.
func ListUsers(ctx context.Context, providerClient *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) ([]User, error) {
	shared.Debugf("[compute] ListUsers: starting")

	identityClient, err := openstack.NewIdentityV3(providerClient, eo)
	if err != nil {
		shared.Debugf("[compute] ListUsers: identity client error: %v", err)
		return nil, fmt.Errorf("identity client: %w", err)
	}

	var result []User
	err = users.List(identityClient, users.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := users.ExtractUsers(page)
		if err != nil {
			return false, err
		}
		for _, u := range extracted {
			email := ""
			if u.Extra != nil {
				if e, ok := u.Extra["email"]; ok {
					email = fmt.Sprintf("%v", e)
				}
			}
			result = append(result, User{
				ID:          u.ID,
				Name:        u.Name,
				Email:       email,
				Enabled:     u.Enabled,
				Description: u.Description,
				DomainID:    u.DomainID,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[compute] ListUsers: error: %v", err)
		return nil, fmt.Errorf("listing users: %w", err)
	}

	shared.Debugf("[compute] ListUsers: success, count=%d", len(result))
	return result, nil
}

// SetUserEnabled enables or disables a Keystone user.
func SetUserEnabled(ctx context.Context, providerClient *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, userID string, enabled bool) error {
	shared.Debugf("[compute] SetUserEnabled: user=%s enabled=%v", userID, enabled)

	identityClient, err := openstack.NewIdentityV3(providerClient, eo)
	if err != nil {
		return fmt.Errorf("identity client: %w", err)
	}

	_, err = users.Update(ctx, identityClient, userID, users.UpdateOpts{
		Enabled: &enabled,
	}).Extract()
	if err != nil {
		shared.Debugf("[compute] SetUserEnabled: error: %v", err)
		return fmt.Errorf("updating user %s: %w", userID, err)
	}

	shared.Debugf("[compute] SetUserEnabled: success")
	return nil
}

// DeleteUser deletes a Keystone user.
func DeleteUser(ctx context.Context, providerClient *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, userID string) error {
	shared.Debugf("[compute] DeleteUser: user=%s", userID)

	identityClient, err := openstack.NewIdentityV3(providerClient, eo)
	if err != nil {
		return fmt.Errorf("identity client: %w", err)
	}

	err = users.Delete(ctx, identityClient, userID).ExtractErr()
	if err != nil {
		shared.Debugf("[compute] DeleteUser: error: %v", err)
		return fmt.Errorf("deleting user %s: %w", userID, err)
	}

	shared.Debugf("[compute] DeleteUser: success")
	return nil
}
