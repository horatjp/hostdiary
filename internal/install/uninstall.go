package install

import (
	"fmt"
	"os"
)

// Uninstall removes only the executable and four units. It never stops an ongoing
// service invocation or removes configuration, data, drop-ins, or leftover links.
func Uninstall(env Env) int {
	env = env.writers()
	if err := env.check(false); err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	ok := true
	for _, timer := range []struct{ path, name string }{{timerPath, timerName}, {dirsTimerPath, dirsTimerName}} {
		if _, err := os.Lstat(env.path(timer.path)); err == nil {
			if !env.command("disable", "--now", timer.name) {
				ok = false
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(env.Stderr, err)
			ok = false
		}
	}
	for _, path := range []string{servicePath, timerPath, dirsServicePath, dirsTimerPath, binaryPath} {
		fi, err := os.Lstat(env.path(path))
		if err == nil && !fi.Mode().IsRegular() {
			fmt.Fprintf(env.Stderr, "%s: not a regular file; left in place\n", path)
			ok = false
			continue
		}
		if err == nil {
			err = os.Remove(env.path(path))
		}
		switch {
		case os.IsNotExist(err):
			fmt.Fprintf(env.Stdout, "%s: absent\n", path)
		case err != nil:
			fmt.Fprintf(env.Stderr, "%s: %v\n", path, err)
			ok = false
		default:
			fmt.Fprintf(env.Stdout, "%s: removed\n", path)
		}
	}
	if !env.command("daemon-reload") {
		ok = false
	}
	active, _, activeErr := timerState(env, "is-active", "active", timerName)
	if activeErr != nil {
		ok = false
	}
	fmt.Fprintf(env.Stdout, "timer: active=%s\n", active)
	if active == "active" {
		ok = false
		fmt.Fprintf(env.Stdout, "the timer is still active. Stop it manually with: systemctl disable --now %s\n", timerName)
	}
	dirsActive, _, dirsErr := timerState(env, "is-active", "active", dirsTimerName)
	if dirsErr != nil {
		ok = false
	}
	fmt.Fprintf(env.Stdout, "dirs timer: active=%s\n", dirsActive)
	if dirsActive == "active" {
		ok = false
		fmt.Fprintf(env.Stdout, "the dirs timer is still active. Stop it manually with: systemctl disable --now %s\n", dirsTimerName)
	}
	serviceActive, _, _ := timerState(env, "is-active", "active", dirsServiceName)
	fmt.Fprintf(env.Stdout, "dirs service: active=%s\n", serviceActive)
	if serviceActive == "activating" || serviceActive == "active" || serviceActive == "deactivating" {
		fmt.Fprintf(env.Stdout, "A directory measurement may run for up to 30 minutes. Stop it with: systemctl stop %s\n", dirsServiceName)
	}
	fmt.Fprintln(env.Stdout, "Data and configuration were kept. To remove them manually: sudo rm -rf /var/lib/hostdiary /etc/hostdiary")
	for _, path := range []string{servicePath + ".d", timerPath + ".d", dirsServicePath + ".d", dirsTimerPath + ".d", "/etc/systemd/system/timers.target.wants/" + timerName, "/etc/systemd/system/timers.target.wants/" + dirsTimerName} {
		if _, err := os.Lstat(env.path(path)); err == nil {
			fmt.Fprintf(env.Stdout, "Kept: %s (remove manually if no longer needed)\n", path)
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(env.Stderr, err)
			ok = false
		}
	}
	if !ok {
		return 1
	}
	return 0
}
