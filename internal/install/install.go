package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"hostdiary/internal/collect"
	"hostdiary/packaging/systemd"
)

// Install checks all prerequisites before writing. A failed trial never enables
// the timer; an existing timer is left running and its actual state is reported.
func Install(env Env) int {
	env = env.writers()
	if err := env.check(true); err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	content, err := os.ReadFile(env.Self)
	if err != nil {
		fmt.Fprintln(env.Stderr, "cannot read executable:", err)
		return 2
	}
	warnWritableDirectory(env)
	statuses := []string{"not attempted", "not attempted", "not attempted", "not attempted", "not attempted"}
	files := []struct {
		path    string
		content []byte
		mode    os.FileMode
	}{
		{binaryPath, content, 0755},
		{servicePath, []byte(systemd.Service), 0644},
		{timerPath, []byte(systemd.Timer), 0644},
		{dirsServicePath, []byte(systemd.DirsService), 0644},
		{dirsTimerPath, []byte(systemd.DirsTimer), 0644},
	}
	ok := true
	for i, file := range files {
		status, err := placeFile(env, env.path(file.path), file.content, file.mode)
		if err != nil {
			statuses[i] = "failed"
			fmt.Fprintf(env.Stderr, "%s: %v\n", file.path, err)
			ok = false
			break
		}
		if i == 0 && status == "unchanged" {
			source, sourceErr := os.Stat(env.Self)
			dest, destErr := os.Stat(env.path(binaryPath))
			if sourceErr == nil && destErr == nil && os.SameFile(source, dest) {
				status = "same"
			}
		}
		// Only units suggest systemctl edit; a binary replacement is an installation.
		if i == 0 && status == replaced {
			status = "installed"
		}
		statuses[i] = status
	}
	if ok {
		if err := createConfigDirectory(env); err != nil {
			fmt.Fprintln(env.Stderr, err)
			ok = false
		}
	}
	if ok {
		ok = env.command("daemon-reload")
	}
	if ok {
		ok = env.command("start", serviceName)
		if !ok {
			trialDiagnostics(env)
		}
	}
	if ok {
		ok = env.command("enable", "--now", timerName)
	}
	if ok {
		ok = env.command("enable", "--now", dirsTimerName)
	}
	enabled, enabledOK, _ := timerState(env, "is-enabled", "enabled", timerName)
	active, activeOK, _ := timerState(env, "is-active", "active", timerName)
	ok = ok && enabledOK && activeOK
	dirsEnabled, dirsEnabledOK, _ := timerState(env, "is-enabled", "enabled", dirsTimerName)
	dirsActive, dirsActiveOK, _ := timerState(env, "is-active", "active", dirsTimerName)
	ok = ok && dirsEnabledOK && dirsActiveOK
	for i, file := range files {
		fmt.Fprintf(env.Stdout, "%s: %s\n", file.path, statuses[i])
	}
	fmt.Fprintf(env.Stdout, "installer version: %s\ntimer: enabled=%s, active=%s\n", collect.Version, enabled, active)
	fmt.Fprintf(env.Stdout, "dirs timer: enabled=%s, active=%s\nMeasure now: systemctl start %s\n", dirsEnabled, dirsActive, dirsServiceName)
	if !ok {
		if active == "active" {
			version := ""
			if statuses[0] != "failed" && statuses[0] != "not attempted" {
				version = fmt.Sprintf(" (version %s)", collect.Version)
			}
			fmt.Fprintf(env.Stdout, "the timer is still active and runs the installed executable%s. Fix the cause and run install again, or stop it with: systemctl disable --now %s\n", version, timerName)
		}
		if dirsActive == "active" {
			fmt.Fprintf(env.Stdout, "the dirs timer is still active. Stop it with: systemctl disable --now %s\n", dirsTimerName)
		}
		return 1
	}
	fmt.Fprintf(env.Stdout, "Next: systemctl list-timers 'hostdiary-*.timer'\nDiagnostics: journalctl -u %s\nFor nginx authentication, see the README section 'Serve behind nginx authentication'.\n", serviceName)
	return 0
}

func timerState(env Env, query, want, name string) (string, bool, error) {
	output, diagnostics, code, err := env.Run(env.Systemctl, query, name)
	state := lastNonemptyLine(output)
	var exit *exec.ExitError
	normalExit := code >= 0 && (err == nil || errors.As(err, &exit))
	if state == "" {
		state = "unknown"
	}
	queryErr := err
	// Nonzero status exits are values (disabled/inactive), not command failures.
	if normalExit {
		queryErr = nil
	}
	if state == "unknown" && queryErr == nil {
		queryErr = fmt.Errorf("timer state is unknown")
	}
	if len(diagnostics) > 0 {
		fmt.Fprintf(env.Stderr, "%s\n", diagnostics)
	}
	if queryErr != nil {
		fmt.Fprintf(env.Stderr, "systemctl %s failed (exit %d): %v\n%s\n", query, code, queryErr, output)
	}
	return state, state == want && code == 0 && err == nil, queryErr
}

// lastNonemptyLine extracts a scalar from stdout only; diagnostics never become state.
func lastNonemptyLine(output []byte) string {
	lines := strings.Split(string(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

func trialDiagnostics(env Env) {
	output, diagnostics, _, _ := env.Run(env.Systemctl, "status", "--no-pager", "-n", "30", serviceName)
	fmt.Fprintf(env.Stderr, "%s\n%s\n", output, diagnostics)
	output, diagnostics, code, err := env.Run(env.Systemctl, "show", "-p", "ExecMainStatus", serviceName)
	if len(diagnostics) > 0 {
		fmt.Fprintf(env.Stderr, "%s\n", diagnostics)
	}
	if code == 0 && err == nil && lastNonemptyLine(output) == "ExecMainStatus=1" {
		fmt.Fprintf(env.Stderr, "collect exit 1 can include a single item failure; data may have been written. If acceptable, run: systemctl enable --now %s\n", timerName)
	}
	fmt.Fprintln(env.Stderr, "For clock_backward, wait a little and run install again.")
}

func warnWritableDirectory(env Env) {
	fi, err := os.Stat(env.path("/usr/local/bin"))
	if err != nil || fi.Mode().Perm()&0022 == 0 {
		return
	}
	_, gid := env.Owner(fi)
	group := strconv.Itoa(gid)
	if entry, err := user.LookupGroupId(group); err == nil {
		group = entry.Name
	}
	who := fmt.Sprintf("group '%s'", group)
	if fi.Mode().Perm()&0020 == 0 {
		who = fmt.Sprintf("other users (directory group '%s')", group)
	} else if fi.Mode().Perm()&0002 != 0 {
		who += " and other users"
	}
	fmt.Fprintf(env.Stderr, "warning: %s can write to /usr/local/bin and could replace the executable that runs as root\n", who)
}
