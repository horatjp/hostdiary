package procfs

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

// PSI disabled by the kernel is unsupported; unrelated failures remain errors.
// Inject errno values because fixture files cannot reproduce a procfs read errno.
func TestPSIReadReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"EOPNOTSUPP", syscall.EOPNOTSUPP, "unsupported"},
		{"ENOTSUP", syscall.ENOTSUP, "unsupported"},
		{"wrapped read", &os.PathError{Op: "read", Path: "/proc/pressure/cpu", Err: syscall.EOPNOTSUPP}, "unsupported"},
		{"wrapped ENOTSUP", fmt.Errorf("read PSI: %w", syscall.ENOTSUP), "unsupported"},
		{"absent", &os.PathError{Op: "open", Path: "/proc/pressure/io", Err: syscall.ENOENT}, "unsupported"},
		{"permission", syscall.EACCES, "error"},
		{"I/O", syscall.EIO, "error"},
		{"parse", fmt.Errorf("missing PSI some"), "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := psiReadReason(tc.err); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
