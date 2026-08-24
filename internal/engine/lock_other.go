//go:build !(windows || darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package engine

import "os"

// lockFile has no OS lock to use on this platform; it only creates the
// file, so concurrent opens are not detected.
func lockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
