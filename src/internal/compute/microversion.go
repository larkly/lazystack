package compute

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
)

// novaBaseMicroversion is what Nova applies to a request without a
// microversion header.
const novaBaseMicroversion = "2.1"

// RequireMicroversion returns an error naming feature when the client's
// negotiated Nova microversion is below min ("X.Y"), so a gated request is
// not sent only to fail with an opaque 404 or 400.
func RequireMicroversion(client *gophercloud.ServiceClient, min, feature string) error {
	if microversionAtLeast(client.Microversion, min) {
		return nil
	}
	connected := client.Microversion
	if connected == "" {
		connected = novaBaseMicroversion
	}
	return fmt.Errorf("%s requires Nova microversion %s; connected with %s", feature, min, connected)
}

// microversionAtLeast reports whether the Nova microversion v is at least
// min, both "X.Y" and compared numerically ("2.10" is above "2.9"). An empty
// or malformed v counts as the 2.1 base, so callers fall back to the legacy
// request shape.
func microversionAtLeast(v, min string) bool {
	vMajor, vMinor, ok := splitMicroversion(v)
	if !ok {
		vMajor, vMinor, _ = splitMicroversion(novaBaseMicroversion)
	}
	minMajor, minMinor, ok := splitMicroversion(min)
	if !ok {
		return false
	}
	return vMajor > minMajor || (vMajor == minMajor && vMinor >= minMinor)
}

func splitMicroversion(v string) (major, minor int, ok bool) {
	ma, mi, found := strings.Cut(v, ".")
	if !found {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(ma)
	minor, err2 := strconv.Atoi(mi)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}
