package systemd

import (
	"strings"
	"testing"
)

func TestDirsUnits(t *testing.T) {
	for _, line := range []string{"Type=oneshot", "User=root", "ExecStart=/usr/local/bin/hostdiary dirs", "StateDirectory=hostdiary", "StateDirectoryMode=0755", "TimeoutStartSec=30min", "SuccessExitStatus=75", "ProtectSystem=strict", "NoNewPrivileges=yes", "IPAddressDeny=any", "RestrictAddressFamilies=AF_UNIX", "ProtectKernelTunables=yes", "ProtectControlGroups=yes", "LockPersonality=yes", "CapabilityBoundingSet=CAP_DAC_READ_SEARCH", "Nice=19", "IOSchedulingClass=best-effort", "IOSchedulingPriority=7", "MemoryMax=256M", "Environment=GOMEMLIMIT=200MiB"} {
		if !strings.Contains("\n"+DirsService, "\n"+line+"\n") {
			t.Error("missing", line)
		}
	}
	for _, key := range []string{"PrivateTmp=", "ProtectKernelModules=", "CAP_SYS_PTRACE"} {
		if strings.Contains(DirsService, key) {
			t.Error(key)
		}
	}
	for _, line := range []string{"OnCalendar=*-*-* 04:30:00", "Persistent=true", "RandomizedDelaySec=10min", "Unit=hostdiary-dirs.service", "WantedBy=timers.target"} {
		if !strings.Contains(DirsTimer, line+"\n") {
			t.Error(line)
		}
	}
}
