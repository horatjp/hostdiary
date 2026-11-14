//go:build linux || darwin

package dirs

import (
	"fmt"
	"os"
	"syscall"
)

func statEntry(fi os.FileInfo) (entry, error) {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || s.Blocks < 0 {
		return entry{}, fmt.Errorf("invalid file stat")
	}
	return entry{bytes: uint64(s.Blocks) * 512, dev: uint64(s.Dev), ino: uint64(s.Ino), nlink: uint64(s.Nlink), dir: fi.IsDir()}, nil
}
func setUmask() func()      { old := syscall.Umask(0022); return func() { syscall.Umask(old) } }
func lock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }
