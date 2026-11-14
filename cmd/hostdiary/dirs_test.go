package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDirsDispatch(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "etc/hostdiary/config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"dirs":{"enabled":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var log, out bytes.Buffer
	if code := run([]string{"dirs", "--root", root}, &log, &out); code != 0 {
		t.Fatal(code, log.String())
	}
	for _, args := range [][]string{{"dirs", "extra"}, {"dirs", "--bad"}, {"dirs", "--help"}} {
		if code := run(args, &log, &out); code != 2 {
			t.Fatal(args, code)
		}
	}
}
