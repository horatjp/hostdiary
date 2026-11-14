package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		ok         bool
	}{
		{"valid", `{"hostname":"fixture","apps":{"worker":["worker"]},"net":{"include":["lo"]}}`, true},
		{"empty", `{}`, true}, {"unknown", `{"public":"x"}`, false}, {"nested unknown", `{"net":{"exlude":[]}}`, false},
		{"type", `{"hostname":12}`, false}, {"trailing", `{} {}`, false}, {"null", `null`, false}, {"empty aliases", `{"apps":{"worker":[]}}`, false}, {"empty name", `{"apps":{"": ["x"]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(p, []byte(tc.text), 0600)
			_, e := Load(p)
			if (e == nil) != tc.ok {
				t.Fatal(e)
			}
		})
	}
	if _, e := Load(filepath.Join(t.TempDir(), "missing")); e != nil {
		t.Fatal(e)
	}
}

func TestCommByteLimit(t *testing.T) {
	for _, tc := range []struct {
		name, alias string
		ok          bool
	}{
		{"ascii15", strings.Repeat("a", 15), true}, {"ascii16", strings.Repeat("a", 16), false},
		{"utf8_15", strings.Repeat("あ", 5), true}, {"utf8_18", strings.Repeat("あ", 6), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(p, []byte(`{"apps":{"worker":["`+tc.alias+`"]}}`), 0600)
			_, e := Load(p)
			if (e == nil) != tc.ok {
				t.Fatal(e)
			}
			if e != nil && !strings.Contains(e.Error(), "comm is truncated to 15 bytes") {
				t.Fatal(e)
			}
		})
	}
}
func TestFilter(t *testing.T) {
	for _, tc := range []struct {
		f              Filter
		name           string
		excluded, want bool
	}{
		{Filter{}, "lo", true, false}, {Filter{Include: []string{"lo"}}, "lo", true, true}, {Filter{Include: []string{"eth0"}}, "eth1", false, false}, {Filter{Include: []string{"lo"}, Exclude: []string{"lo"}}, "lo", true, false}, {Filter{Exclude: []string{"eth0"}}, "eth0", false, false},
	} {
		if got := tc.f.Select(tc.name, tc.excluded); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
