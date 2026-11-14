package dirs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
)

// Run reads only selected mounts, writes only dirs data, and uses its own lock.
func Run(root string, stderr io.Writer, now func() time.Time) (code int) {
	restore := setUmask()
	defer restore()
	defer func() { fmt.Fprintf(stderr, "hostdiary dirs: exit=%d\n", code) }()
	started := now().Unix()
	c, err := config.Load(procfs.RootPath(root, "/etc/hostdiary/config.json"))
	if err != nil {
		fmt.Fprintln(stderr, "config:", err)
		return 2
	}
	base := procfs.RootPath(root, "/var/lib/hostdiary")
	if err = os.MkdirAll(base, 0755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	f, err := os.OpenFile(filepath.Join(base, "dirs.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer f.Close()
	if err = lock(f); err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return 75
		}
		return 1
	}
	dir := filepath.Join(base, "public/data/dirs")
	if !c.Dirs.IsEnabled() {
		if err = pruneFiles(dir, true, ""); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	b, err := os.ReadFile(procfs.RootPath(root, "/proc/self/mountinfo"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	mounts, err := procfs.ParseMounts(string(b))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	mounts = procfs.SelectMounts(mounts)
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Point < mounts[j].Point })
	doc := Document{Schema: 1, Kind: "dirs", StartedAt: started, Depth: depth, Top: top, Filesystems: []Filesystem{}}
	log := walkErrors{stderr: stderr}
	defer log.summary()
	for _, m := range mounts {
		if !c.Disk.Select(m.Point, false) {
			continue
		}
		result := walkWithLog(procfs.RootPath(root, m.Point), m.Point, os.Lstat, &log)
		doc.Filesystems = append(doc.Filesystems, result)
		if !result.Complete {
			code = 1
		}
	}
	if len(doc.Filesystems) == 0 {
		fmt.Fprintln(stderr, "no filesystem selected")
		return 0
	}
	doc.FinishedAt = now().Unix()
	if err = writeDocument(base, dir, doc); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err = pruneFiles(dir, false, time.Unix(started, 0).UTC().Format("2006-01-02")); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return code
}
