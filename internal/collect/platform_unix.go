//go:build linux || darwin

package collect

import (
	"errors"
	"os"
	"syscall"
)

func setUmask() func() { old := syscall.Umask(0022); return func() { syscall.Umask(old) } }
func lock(f *os.File) error {
	e := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(e, syscall.EWOULDBLOCK) || errors.Is(e, syscall.EAGAIN) {
		return errRunning
	}
	return e
}
