// Package collect orchestrates a single locked collection. All paths honor root.
package collect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"hostdiary/internal/config"
	"hostdiary/internal/metrics"
	"hostdiary/internal/procfs"
	"hostdiary/internal/store"
	"hostdiary/internal/web"
)

const Version = "0.3.0"

var errRunning = errors.New("collection already running")

type State struct {
	LastCollected int64               `json:"last_collected"`
	Boots         []int64             `json:"boots"`
	Observation   metrics.Observation `json:"observation"`
}

func loadState(path string) (State, error) {
	var s State
	b, e := os.ReadFile(path)
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	if e != nil {
		return State{}, e
	}
	if s.LastCollected <= 0 || s.Observation.Uptime < 0 {
		return State{}, errors.New("invalid state")
	}
	return s, nil
}

// Run accepts a clock for deterministic fixture tests; production passes time.Now.
func Run(root string, stderr io.Writer, now func() time.Time) (code int) {
	restore := setUmask()
	defer restore()
	defer func() { fmt.Fprintf(stderr, "hostdiary collect: exit=%d\n", code) }()
	// Invalid configuration must not even create the state directory or lock file.
	c, e := config.Load(procfs.RootPath(root, "/etc/hostdiary/config.json"))
	if e != nil {
		fmt.Fprintf(stderr, "config: %v\n", e)
		return 2
	}
	base := procfs.RootPath(root, "/var/lib/hostdiary")
	if e = os.MkdirAll(base, 0755); e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	f, e := os.OpenFile(filepath.Join(base, "collect.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	defer f.Close()
	if e = lock(f); e != nil {
		fmt.Fprintln(stderr, e)
		if errors.Is(e, errRunning) {
			return 75
		}
		return 1
	}
	statePath := filepath.Join(base, "state.json")
	prev, e := loadState(statePath)
	if e != nil {
		fmt.Fprintf(stderr, "state ignored: %v\n", e)
		prev = State{}
	}
	at := now().Unix()
	if at <= prev.LastCollected {
		fmt.Fprintln(stderr, "clock_backward")
		return 1
	}
	s := procfs.ReadSnapshot(root, c)
	rows, missing := metrics.BuildRows(prev.Observation, s, c, at)
	for _, e := range s.Errors {
		fmt.Fprintln(stderr, e)
		code = 1
	}
	for _, u := range missing {
		fmt.Fprintf(stderr, "unavailable: %s %s\n", u.Group, u.Reason)
	}
	public := filepath.Join(base, "public")
	if e = os.MkdirAll(public, 0755); e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	if e = web.WriteAssets(public, Version); e != nil {
		fmt.Fprintf(stderr, "web assets: %v\n", e)
		code = 1
	}
	// AppendGroup repairs its own files; scan all retained days only once below.
	for _, row := range rows {
		if e = store.AppendGroup(public, row.Group, at, row.Values); e != nil {
			fmt.Fprintf(stderr, "append %s: %v\n", row.Group.ID, e)
			code = 1
		}
	}
	rollupErr := store.Rollup(public, time.Unix(at, 0))
	if rollupErr != nil {
		fmt.Fprintf(stderr, "rollup: %v\n", rollupErr)
		code = 1
	}
	if e = store.RepairAndPrune(public, rollupErr != nil); e != nil {
		fmt.Fprintln(stderr, e)
		code = 1
	}
	boots := bootHistory(prev, metrics.Observe(s), at)
	hostname := c.Hostname
	if hostname == "" {
		hostname, e = os.Hostname()
		if e != nil {
			fmt.Fprintln(stderr, e)
			code = 1
		}
	}
	groups := []store.Group{}
	for _, row := range rows {
		groups = append(groups, row.Group)
	}
	if e = store.WriteIndex(public, store.Index{Schema: 1, Version: Version, Host: hostname, GeneratedAt: at, Boots: boots, Unavailable: missing, Groups: groups}); e != nil {
		fmt.Fprintln(stderr, e)
		code = 1
	}
	if e = store.WriteSummary(public, store.SummaryMeta{Version: Version, Host: hostname, GeneratedAt: at}); e != nil {
		fmt.Fprintf(stderr, "summary: %v\n", e)
		code = 1
	}
	state := State{at, boots, metrics.Observe(s)}
	b, e := json.Marshal(state)
	if e == nil {
		e = store.AtomicWrite(statePath, append(b, '\n'), 0600, base)
	}
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	return code
}
