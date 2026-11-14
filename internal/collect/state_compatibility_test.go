package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostdiary/internal/store"
)

func lastExtraRow(t *testing.T, root, id string) map[string]any {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31", id+".jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	var cols []string
	var values []any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.HasPrefix(line, "{") {
			var header struct{ Columns []string }
			if e := json.Unmarshal([]byte(line), &header); e != nil {
				t.Fatal(e)
			}
			cols = header.Columns
		} else if e := json.Unmarshal([]byte(line), &values); e != nil {
			t.Fatal(e)
		}
	}
	if len(values) != len(cols) {
		t.Fatal(cols, values)
	}
	out := map[string]any{}
	for i, col := range cols {
		out[col] = values[i]
	}
	return out
}

func TestExtraPersistedOldStateAndSNMPRecovery(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if code, log := runAt(root, at); code != 0 {
		t.Fatal(code, log)
	}
	// Recreate the old state schema with no optional counters and short arrays.
	statePath := filepath.Join(root, "var/lib/hostdiary/state.json")
	b, e := os.ReadFile(statePath)
	if e != nil {
		t.Fatal(e)
	}
	var state map[string]any
	if e = json.Unmarshal(b, &state); e != nil {
		t.Fatal(e)
	}
	obs := state["observation"].(map[string]any)
	delete(obs, "psi")
	delete(obs, "vmstat")
	delete(obs, "tcp_segs")
	obs["uptime"] = 100.50
	obs["net"] = map[string][]uint64{"eth0": {1000, 2000}}
	obs["diskio"] = map[string][]uint64{"sda": {100, 200, 10, 20, 300}}
	b, e = json.Marshal(state)
	if e != nil {
		t.Fatal(e)
	}
	write(t, root, "var/lib/hostdiary/state.json", string(b))
	write(t, root, "proc/uptime", "160.50 2000\n")
	write(t, root, "etc/hostdiary/config.json", `{"hostname":"fixture-host","disk":{"exclude":["/","/space disk"]},"apps":{"worker":["worker (one)"]}}`)
	if code, log := runAt(root, at+60); code != 0 {
		t.Fatal(code, log)
	}
	checks := map[string][]string{
		"psi": {"cpu_some", "memory_some", "memory_full", "io_some", "io_full"},
		"mem": {"oom_kills"}, "swap": {"in", "out"}, "tcp": {"out_segs", "retrans_segs"},
		store.GroupID("net", "eth0"):   {"rx_errors", "rx_drops", "tx_errors", "tx_drops"},
		store.GroupID("diskio", "sda"): {"read_time", "write_time"},
	}
	for id, cols := range checks {
		r := lastExtraRow(t, root, id)
		for _, col := range cols {
			if v, ok := r[col]; !ok || v != nil {
				t.Fatal(id, col, r)
			}
		}
	}
	if r := lastExtraRow(t, root, store.GroupID("app", "worker")); r["started"] != float64(1) {
		t.Fatal(r)
	}
	for id, cols := range map[string][]string{
		store.GroupID("net", "eth0"):   {"rx", "tx"},
		store.GroupID("diskio", "sda"): {"read", "write", "read_ops", "write_ops", "busy"},
	} {
		r := lastExtraRow(t, root, id)
		for _, col := range cols {
			if v, ok := r[col]; !ok || v != float64(0) {
				t.Fatal("existing baseline lost", id, col, r)
			}
		}
	}
	write(t, root, "proc/uptime", "220.50 2100\n")
	if code, log := runAt(root, at+120); code != 0 {
		t.Fatal(code, log)
	}
	for id, cols := range checks {
		r := lastExtraRow(t, root, id)
		for _, col := range cols {
			if r[col] != float64(0) {
				t.Fatal(id, col, r)
			}
		}
	}
	// A failed SNMP read must replace the saved baseline with absence.
	write(t, root, "proc/net/snmp", "broken")
	write(t, root, "proc/uptime", "280.50 2200\n")
	if code, log := runAt(root, at+180); code != 1 || !strings.Contains(log, "/proc/net/snmp:") {
		t.Fatal(code, log)
	}
	write(t, root, "proc/net/snmp", "Tcp: OutSegs RetransSegs MaxConn\nTcp: 701 322 -1\n")
	for i, uptime := range []string{"340.50 2300\n", "400.50 2400\n"} {
		write(t, root, "proc/uptime", uptime)
		if code, log := runAt(root, at+240+int64(i)*60); code != 0 {
			t.Fatal(code, log)
		}
		r := lastExtraRow(t, root, "tcp")
		var want any
		if i == 1 {
			want = float64(0)
		}
		if r["out_segs"] != want || r["retrans_segs"] != want {
			t.Fatal(r)
		}
	}
}
