//go:build !linux

package procfs

import "errors"

var errUnsupported = errors.New("statfs collection requires Linux")

func isUnsupportedFS(e error) bool   { return errors.Is(e, errUnsupported) }
func ReadFS(path string) (FS, error) { return FS{}, errUnsupported }
