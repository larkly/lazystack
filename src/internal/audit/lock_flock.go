//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package audit

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f, blocking until it is
// available. flock locks belong to the open file description, so they also
// serialize independent Loggers inside one process.
func lockFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			return err
		}
	}
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
