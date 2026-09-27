//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package audit

import "os"

// lockFile is a no-op where flock is unavailable; writers are then only
// serialized within a Logger by its mutex.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) error { return nil }
