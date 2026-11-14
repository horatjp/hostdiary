//go:build linux

package procfs

import "syscall"

func isUnsupportedFS(error) bool { return false }

func ReadFS(path string) (FS, error) {
	var s syscall.Statfs_t
	if e := syscall.Statfs(path, &s); e != nil {
		return FS{}, e
	}
	size := s.Frsize
	if size == 0 {
		size = s.Bsize
	}
	return FS{(s.Blocks - s.Bfree) * uint64(size), s.Bavail * uint64(size), s.Files - s.Ffree, s.Files}, nil
}
