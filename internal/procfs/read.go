package procfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"hostdiary/internal/config"
	"hostdiary/internal/store"
)

type FS struct{ Used, Avail, InodesUsed, InodesTotal uint64 }
type Snapshot struct {
	BootID         string
	Uptime         float64
	UptimeValid    bool
	CPU            [8]uint64
	CPUValid       bool
	CPUs           int
	Load           [3]float64
	LoadValid      bool
	Mem            map[string]uint64
	Net, DiskIO    map[string][]uint64
	PSI, VMStat    map[string]uint64
	TCPSegs        []uint64
	Disks          map[string]FS
	Processes      []Process
	ProcessesValid bool
	TCP            [12]uint64
	TCPValid       bool
	Unavailable    []store.Unavailable
	Errors         []error
}

func RootPath(root, path string) string { return filepath.Join(root, strings.TrimPrefix(path, "/")) }
func (s *Snapshot) unavailable(kind, name, reason string, err error) {
	if reason == "error" && err != nil {
		s.Errors = append(s.Errors, err)
	}
	group := kind
	if name != "" {
		group = store.GroupID(kind, name)
	}
	for _, u := range s.Unavailable {
		if u.Group == group && u.Column == nil && u.Reason == reason {
			return
		}
	}
	s.Unavailable = append(s.Unavailable, store.Unavailable{Group: group, Reason: reason})
}

// Diagnostics use the Linux source path, never the development root prefix.
func sourceError(path string, err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("%s: %w", path, err)
}
func reason(err error) string {
	if os.IsNotExist(err) {
		return "unsupported"
	}
	return "error"
}

// ReadSnapshot reads only safe mounts selected from mountinfo, never all mounts.
func ReadSnapshot(root string, c config.Config) Snapshot {
	s := Snapshot{Net: map[string][]uint64{}, DiskIO: map[string][]uint64{}, Disks: map[string]FS{}}
	s.readExtra(root)
	read := func(path string) (string, error) { b, e := os.ReadFile(RootPath(root, path)); return string(b), e }
	parse := func(path, kind string, fn func(string) error) {
		text, e := read(path)
		if e == nil {
			e = fn(text)
		}
		if e != nil {
			if kind == "rates" {
				for _, k := range []string{"cpu", "net", "diskio"} {
					s.unavailable(k, "", reason(e), nil)
				}
				if reason(e) == "error" {
					s.Errors = append(s.Errors, sourceError(path, e))
				}
			} else {
				s.unavailable(kind, "", reason(e), sourceError(path, e))
			}
		}
	}
	parse("/proc/sys/kernel/random/boot_id", "rates", func(text string) error {
		s.BootID = strings.TrimSpace(text)
		if s.BootID == "" {
			return errors.New("empty boot_id")
		}
		return nil
	})
	parse("/proc/uptime", "rates", func(text string) error { v, e := ParseUptime(text); s.Uptime = v; s.UptimeValid = e == nil; return e })
	parse("/proc/stat", "cpu", func(text string) error {
		v, n, e := ParseStat(text)
		s.CPU = v
		s.CPUs = n
		s.CPUValid = e == nil
		return e
	})
	parse("/proc/loadavg", "load", func(text string) error { v, e := ParseLoad(text); s.Load = v; s.LoadValid = e == nil; return e })
	parse("/proc/meminfo", "mem", func(text string) error { v, e := ParseMem(text); s.Mem = v; return e })
	parse("/proc/net/dev", "net", func(text string) error {
		v, e := ParseNet(text)
		if e != nil {
			return e
		}
		for name, values := range v {
			excluded := name == "lo"
			for _, prefix := range []string{"veth", "docker", "br-", "virbr"} {
				excluded = excluded || strings.HasPrefix(name, prefix)
			}
			if c.Net.Select(name, excluded) {
				s.Net[name] = values
			}
		}
		return nil
	})
	parse("/proc/diskstats", "diskio", func(text string) error {
		v, e := ParseDiskStats(text)
		if e != nil {
			return e
		}
		for name, values := range v {
			excluded := false
			for _, prefix := range []string{"loop", "ram", "sr", "nbd", "zram"} {
				excluded = excluded || strings.HasPrefix(name, prefix)
			}
			if _, e := os.Stat(RootPath(root, "/sys/block/"+name)); e != nil {
				if !os.IsNotExist(e) {
					s.unavailable("diskio", name, "error", sourceError("/sys/block/"+name, e))
				}
				continue
			}
			if c.DiskIO.Select(name, excluded) {
				s.DiskIO[name] = values
			}
		}
		return nil
	})
	parse("/proc/self/mountinfo", "disk", func(text string) error {
		v, e := ParseMounts(text)
		if e != nil {
			return e
		}
		for _, m := range SelectMounts(v) {
			if !c.Disk.Select(m.Point, false) {
				continue
			}
			fs, e := ReadFS(RootPath(root, m.Point))
			if e != nil {
				r := reason(e)
				if errors.Is(e, errors.ErrUnsupported) {
					r = "unsupported"
				} // non-Linux builds intentionally provide no fake statfs.
				if isUnsupportedFS(e) {
					r = "unsupported"
				}
				s.unavailable("disk", m.Point, r, sourceError(m.Point, e))
				continue
			}
			s.Disks[m.Point] = fs
		}
		return nil
	})
	for _, pair := range []struct {
		kind    string
		f       config.Filter
		present func(string) bool
	}{{"net", c.Net, func(n string) bool { _, ok := s.Net[n]; return ok }}, {"diskio", c.DiskIO, func(n string) bool { _, ok := s.DiskIO[n]; return ok }}, {"disk", c.Disk, func(n string) bool { _, ok := s.Disks[n]; return ok }}} {
		for _, n := range pair.f.Include {
			if pair.f.Select(n, false) && !pair.present(n) {
				id := store.GroupID(pair.kind, n)
				reported := false
				for _, u := range s.Unavailable {
					reported = reported || u.Group == id
				}
				if !reported {
					s.unavailable(pair.kind, n, "not_found", nil)
				}
			}
		}
	}
	// Either TCP family may be absent; report unsupported only if both are absent.
	s.TCPValid = true
	seenTCP := false
	missingTCP := 0
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		text, e := read(path)
		if os.IsNotExist(e) {
			missingTCP++
			continue
		}
		if e == nil {
			var v [12]uint64
			v, e = ParseTCP(text)
			if e == nil {
				seenTCP = true
				for i := range v {
					s.TCP[i] += v[i]
				}
			}
		}
		if e != nil {
			s.TCPValid = false
			s.unavailable("tcp", "", "error", sourceError(path, e))
		}
	}
	if missingTCP == 2 {
		s.unavailable("tcp", "", "unsupported", nil)
	}
	s.TCPValid = s.TCPValid && seenTCP
	entries, e := os.ReadDir(RootPath(root, "/proc"))
	if e != nil {
		s.unavailable("procs", "", reason(e), sourceError("/proc", e))
		return s
	}
	s.ProcessesValid = true
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, e := strconv.ParseUint(entry.Name(), 10, 32); e != nil {
			continue
		}
		text, e := read("/proc/" + entry.Name() + "/stat")
		if os.IsNotExist(e) || errors.Is(e, syscall.ESRCH) {
			continue
		}
		if e == nil {
			var p Process
			p, e = ParsePIDStat(text, uint64(os.Getpagesize()))
			if e == nil {
				s.Processes = append(s.Processes, p)
			}
		}
		if e != nil {
			s.ProcessesValid = false
			s.unavailable("procs", "", "error", sourceError("/proc/"+entry.Name()+"/stat", e))
		}
	}
	sort.Slice(s.Unavailable, func(i, j int) bool {
		a, b := s.Unavailable[i], s.Unavailable[j]
		return a.Group+a.Reason < b.Group+b.Reason
	})
	return s
}
