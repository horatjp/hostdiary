package collect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"hostdiary/internal/metrics"
)

// Past boots include the cutoff; the current boot has no age limit.
func TestBootHistoryRetentionWindow(t *testing.T) {
	const at int64 = 1793404800
	const cutoff = at - 1095*86400
	prev := State{Boots: []int64{cutoff - 1, cutoff, cutoff + 1, at - 20*86400}, Observation: metrics.Observation{BootID: "previous"}}
	before := append([]int64(nil), prev.Boots...)
	got := bootHistory(prev, metrics.Observation{BootID: "current", Uptime: 10.9, UptimeValid: true}, at)
	want := []int64{cutoff, cutoff + 1, at - 20*86400, at - 10}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(prev.Boots, before) {
		t.Fatal(got, prev.Boots)
	}
	got[0] = 0
	if !reflect.DeepEqual(prev.Boots, before) {
		t.Fatal("output aliases previous boots")
	}
}

func TestBootHistoryCurrentAndMissingInputs(t *testing.T) {
	const at int64 = 1793404800
	const old = at - 1200*86400
	for _, tc := range []struct {
		name string
		prev State
		cur  metrics.Observation
		want []int64
	}{
		{"same boot keeps old estimate without uptime", State{Boots: []int64{old}, Observation: metrics.Observation{BootID: "same"}}, metrics.Observation{BootID: "same"}, []int64{old}},
		{"changed boot expires old current", State{Boots: []int64{old}, Observation: metrics.Observation{BootID: "before"}}, metrics.Observation{BootID: "after", Uptime: 10, UptimeValid: true}, []int64{at - 10}},
		{"changed boot without uptime leaves previous last", State{Boots: []int64{at - 20*86400}, Observation: metrics.Observation{BootID: "before"}}, metrics.Observation{BootID: "after"}, []int64{at - 20*86400}},
		{"missing boot id leaves previous last", State{Boots: []int64{at - 20*86400}, Observation: metrics.Observation{BootID: "before"}}, metrics.Observation{Uptime: 10, UptimeValid: true}, []int64{at - 20*86400}},
		{"empty state estimates current", State{}, metrics.Observation{BootID: "current", Uptime: 10, UptimeValid: true}, []int64{at - 10}},
		{"empty state without information", State{}, metrics.Observation{}, []int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]int64(nil), tc.prev.Boots...)
			if got := bootHistory(tc.prev, tc.cur, at); !reflect.DeepEqual(got, tc.want) {
				t.Fatal(got, tc.want)
			}
			if !reflect.DeepEqual(tc.prev.Boots, before) {
				t.Fatal("input changed")
			}
		})
	}
}

func TestBootRetentionPublishedAndStateLoss(t *testing.T) {
	root := copyFixture(t)
	const at int64 = 1793404800
	write(t, root, "proc/uptime", "1728000 0\n")
	if code, log := runAt(root, at); code != 0 {
		t.Fatal(code, log)
	}
	write(t, root, "proc/sys/kernel/random/boot_id", "fixture-next-boot\n")
	write(t, root, "proc/uptime", "10 0\n")
	if code, log := runAt(root, at+60); code != 0 {
		t.Fatal(code, log)
	}
	if got := index(t, root).Boots; !reflect.DeepEqual(got, []int64{at - 20*86400, at + 50}) {
		t.Fatal(got)
	}
	if err := os.Remove(filepath.Join(root, "var/lib/hostdiary/state.json")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "proc/uptime", "70 0\n")
	if code, log := runAt(root, at+120); code != 0 {
		t.Fatal(code, log)
	}
	if got := index(t, root).Boots; !reflect.DeepEqual(got, []int64{at + 50}) {
		t.Fatal("state loss must not reconstruct previous boots", got)
	}
}
