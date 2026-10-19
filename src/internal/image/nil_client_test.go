package image

import (
	"context"
	"strings"
	"testing"
)

// gophercloud panics on a nil *ServiceClient; clouds without Glance must get
// an error instead.
func TestNilImageClientIsAnError(t *testing.T) {
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"ListImages": func() error { _, err := ListImages(ctx, nil); return err },
		"GetImage":   func() error { _, err := GetImage(ctx, nil, "img"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "image (Glance) service is not available") {
				t.Errorf("err=%v", err)
			}
		})
	}
}
