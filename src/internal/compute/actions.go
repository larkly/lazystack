package compute

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/instanceactions"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Action is a simplified instance action.
type Action struct {
	Action    string
	RequestID string
	UserID    string
	StartTime time.Time
	Message   string
}

// ListActions fetches instance actions for a server.
func ListActions(ctx context.Context, client *gophercloud.ServiceClient, serverID string) ([]Action, error) {
	var result []Action
	err := instanceactions.List(client, serverID, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := instanceactions.ExtractInstanceActions(page)
		if err != nil {
			return false, err
		}
		for _, a := range extracted {
			result = append(result, Action{
				Action:    a.Action,
				RequestID: a.RequestID,
				UserID:    a.UserID,
				StartTime: a.StartTime,
				Message:   a.Message,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing actions for %s: %w", serverID, err)
	}
	return result, nil
}
