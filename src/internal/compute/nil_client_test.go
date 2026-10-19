package compute

import (
	"context"
	"strings"
	"testing"
)

// gophercloud panics on a nil *ServiceClient; clouds without Nova must get
// an error instead.
func TestNilComputeClientIsAnError(t *testing.T) {
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"ListActions":     func() error { _, err := ListActions(ctx, nil, "vm"); return err },
		"ListFlavors":     func() error { _, err := ListFlavors(ctx, nil); return err },
		"ListHypervisors": func() error { _, err := ListHypervisors(ctx, nil); return err },
		"ListKeyPairs":    func() error { _, err := ListKeyPairs(ctx, nil); return err },
		"GetKeyPair":      func() error { _, err := GetKeyPair(ctx, nil, "k"); return err },
		"ListServers":     func() error { _, err := ListServers(ctx, nil); return err },
		"GetServer":       func() error { _, err := GetServer(ctx, nil, "vm"); return err },
		"GetRemoteConsole": func() error {
			_, err := GetRemoteConsole(ctx, nil, "vm")
			return err
		},
		"GetPassword":      func() error { _, _, err := GetPassword(ctx, nil, "vm", ""); return err },
		"GetConsoleOutput": func() error { _, err := GetConsoleOutput(ctx, nil, "vm", 10); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "compute (Nova) service is not available") {
				t.Errorf("err=%v", err)
			}
		})
	}
}
