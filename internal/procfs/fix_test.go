package procfs

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"hostdiary/internal/config"
)

func TestSourceUnavailableVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		missing    []string
		broken     map[string]string
		want       []string
		errorPaths []string
	}{
		{name: "tcp6 absent", missing: []string{"net/tcp6"}},
		{name: "tcp absent", missing: []string{"net/tcp"}},
		{name: "both tcp absent", missing: []string{"net/tcp", "net/tcp6"}, want: []string{"tcp:unsupported"}},
		{name: "both clocks absent", missing: []string{"uptime", "sys/kernel/random/boot_id"}, want: []string{"cpu:unsupported", "diskio:unsupported", "net:unsupported"}},
		{name: "source failures", broken: map[string]string{"net/dev": "garbage", "diskstats": "broken", "self/mountinfo": "broken"}, want: []string{"disk:error", "diskio:error", "net:error"}, errorPaths: []string{"/proc/net/dev:", "/proc/diskstats:", "/proc/self/mountinfo:"}},
		{name: "both tcp invalid", broken: map[string]string{"net/tcp": "broken", "net/tcp6": "broken"}, want: []string{"tcp:error"}, errorPaths: []string{"/proc/net/tcp:", "/proc/net/tcp6:"}},
		{name: "v4 absent v6 broken", missing: []string{"net/tcp"}, broken: map[string]string{"net/tcp6": "broken"}, want: []string{"tcp:error"}, errorPaths: []string{"/proc/net/tcp6:"}},
		{name: "v6 absent v4 broken", missing: []string{"net/tcp6"}, broken: map[string]string{"net/tcp": "broken"}, want: []string{"tcp:error"}, errorPaths: []string{"/proc/net/tcp:"}},
		{name: "clock errors", broken: map[string]string{"uptime": "broken", "sys/kernel/random/boot_id": ""}, want: []string{"cpu:error", "diskio:error", "net:error"}, errorPaths: []string{"/proc/uptime:", "/proc/sys/kernel/random/boot_id:"}},
		{name: "pid failures", broken: map[string]string{"101/stat": "broken", "102/stat": "broken"}, want: []string{"procs:error"}, errorPaths: []string{"/proc/101/stat:", "/proc/102/stat:"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := snapshotRoot(t)
			for _, p := range tc.missing {
				if e := os.Remove(filepath.Join(root, "proc", p)); e != nil {
					t.Fatal(e)
				}
			}
			for p, b := range tc.broken {
				if e := os.WriteFile(filepath.Join(root, "proc", p), []byte(b), 0644); e != nil {
					t.Fatal(e)
				}
			}
			s := ReadSnapshot(root, config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}})
			got := []string{}
			for _, u := range s.Unavailable {
				if u.Column != nil {
					t.Fatal(u)
				}
				got = append(got, u.Group+":"+u.Reason)
			}
			want := append([]string{}, tc.want...)
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
			if tc.name == "tcp6 absent" && (!s.TCPValid || s.TCP[0] != 1 || s.TCP[9] != 1) {
				t.Fatal(s.TCP, s.TCPValid)
			}
			if tc.name == "tcp absent" && (!s.TCPValid || s.TCP[5] != 1) {
				t.Fatal(s.TCP, s.TCPValid)
			}
			if tc.name == "both tcp absent" && (s.TCPValid || s.TCP != [12]uint64{}) {
				t.Fatal(s.TCP, s.TCPValid)
			}
			if len(s.Errors) != len(tc.errorPaths) {
				t.Fatal(s.Errors, tc.errorPaths)
			}
			for _, path := range tc.errorPaths {
				found := false
				for _, e := range s.Errors {
					found = found || strings.HasPrefix(e.Error(), path)
					if strings.Contains(e.Error(), root) {
						t.Fatal("root leaked into source diagnostic")
					}
				}
				if !found {
					t.Fatal(path, s.Errors)
				}
			}
		})
	}
}

func TestPIDDiagnosticNamesSource(t *testing.T) {
	root := snapshotRoot(t)
	os.WriteFile(filepath.Join(root, "proc/101/stat"), []byte("broken"), 0644)
	s := ReadSnapshot(root, config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}})
	if len(s.Errors) != 1 || !strings.HasPrefix(s.Errors[0].Error(), "/proc/101/stat:") {
		t.Fatal(s.Errors)
	}
}
