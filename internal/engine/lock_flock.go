//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package engine

import (
	"os"
	"syscall"
)

// lockFile takes a non-blocking exclusive flock on path. The lock is
// released when the file is closed or the process exits.
func lockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
