package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirsConfig(t *testing.T) {
	for _, tc := range []struct {
		text        string
		enabled, ok bool
	}{
		{`{}`, true, true}, {`{"dirs":{}}`, true, true}, {`{"dirs":{"enabled":true}}`, true, true},
		{`{"dirs":{"enabled":false}}`, false, true}, {`{"dirs":{"typo":false}}`, false, false},
		{`{"dirs":{"enabled":"false"}}`, false, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(p, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(p)
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
			if tc.ok && c.Dirs.IsEnabled() != tc.enabled {
				t.Fatal(c)
			}
		})
	}
}
