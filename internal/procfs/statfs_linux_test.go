//go:build linux

package procfs

import "testing"

func TestReadFS(t *testing.T) {
	v, e := ReadFS(t.TempDir())
	if e != nil || v.Avail == 0 {
		t.Fatal(v, e)
	}
}
