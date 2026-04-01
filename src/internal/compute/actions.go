package compute

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/instanceactions"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
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
	shared.Debugf("[compute] listing actions for server %s", serverID)
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
		shared.Debugf("[compute] list actions for server %s: %v", serverID, err)
		return nil, fmt.Errorf("listing actions for %s: %w", serverID, err)
	}
	shared.Debugf("[compute] listed %d actions for server %s", len(result), serverID)
	return result, nil
}
