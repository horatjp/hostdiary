package procfs

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"hostdiary/internal/store"
)

// Defensively classify unsupported-operation errors if returned by PSI reads.
// Keep this classification local to PSI rather than changing other source errors.
func psiReadReason(err error) string {
	if errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.ENOTSUP) {
		return "unsupported"
	}
	return reason(err)
}

func (s *Snapshot) unavailableColumns(group string, columns []string, why string) {
	for _, col := range columns {
		column := col
		s.Unavailable = append(s.Unavailable, store.Unavailable{Group: group, Column: &column, Reason: why})
	}
}

// readExtra reads optional sources without turning unsupported kernels into errors.
func (s *Snapshot) readExtra(root string) {
	s.PSI = map[string]uint64{}
	if _, e := os.Stat(RootPath(root, "/proc/pressure")); e != nil {
		s.unavailable("psi", "", psiReadReason(e), sourceError("/proc/pressure", e))
	} else {
		for _, resource := range []string{"cpu", "memory", "io"} {
			path := "/proc/pressure/" + resource
			cols := []string{resource + "_some"}
			if resource != "cpu" {
				cols = append(cols, resource+"_full")
			}
			b, e := os.ReadFile(RootPath(root, path))
			var some, full *uint64
			if e == nil {
				some, full, e = ParsePSI(string(b))
			}
			if e != nil {
				why := psiReadReason(e)
				s.unavailableColumns("psi", cols, why)
				if why == "error" {
					s.Errors = append(s.Errors, sourceError(path, e))
				}
				continue
			}
			s.PSI[cols[0]] = *some
			if len(cols) == 2 {
				if full == nil {
					s.unavailableColumns("psi", cols[1:], "unsupported")
				} else {
					s.PSI[cols[1]] = *full
				}
			}
		}
	}
	path := "/proc/vmstat"
	b, e := os.ReadFile(RootPath(root, path))
	bad := map[string]bool{}
	if e == nil {
		s.VMStat, bad = parseVMStat(string(b))
	}
	parseFailed := false
	for _, key := range []struct{ key, group, col string }{{"pswpin", "swap", "in"}, {"pswpout", "swap", "out"}, {"oom_kill", "mem", "oom_kills"}} {
		if _, ok := s.VMStat[key.key]; ok {
			continue
		}
		why := "unsupported"
		if e != nil {
			why = reason(e)
		} else if bad[key.key] {
			why, parseFailed = "error", true
		}
		s.unavailableColumns(key.group, []string{key.col}, why)
	}
	if e != nil && reason(e) == "error" {
		s.Errors = append(s.Errors, sourceError(path, e))
	} else if parseFailed {
		s.Errors = append(s.Errors, sourceError(path, fmt.Errorf("invalid vmstat counter")))
	}
	path = "/proc/net/snmp"
	b, e = os.ReadFile(RootPath(root, path))
	if e == nil {
		s.TCPSegs, e = ParseSNMPTCP(string(b))
	}
	if e != nil {
		s.unavailableColumns("tcp", []string{"out_segs", "retrans_segs"}, reason(e))
		if reason(e) == "error" {
			s.Errors = append(s.Errors, sourceError(path, e))
		}
	}
}
