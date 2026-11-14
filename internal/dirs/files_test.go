package dirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDocumentJSFailureDoesNotPublishJSON(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "public/data/dirs")
	if err := os.MkdirAll(filepath.Join(dir, "2026-11-01.js"), 0755); err != nil {
		t.Fatal(err)
	}
	doc := Document{Schema: 1, Kind: "dirs", StartedAt: clock().Unix(), Filesystems: []Filesystem{}}
	if err := writeDocument(base, dir, doc); err == nil {
		t.Fatal("expected failed JS rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-01.json")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	files, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "public" {
		t.Fatal("private temporary file leaked", files)
	}
}

func TestDisabledRemovalFailureReturnsFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not deny root")
	}
	root := t.TempDir()
	writeFixture(t, root, "etc/hostdiary/config.json", []byte(`{"dirs":{"enabled":false}}`))
	p := writeFixture(t, root, "var/lib/hostdiary/public/data/dirs/2026-11-01.json", []byte("private"))
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755)
	if code := Run(root, os.Stderr, clock); code != 1 {
		t.Fatal(code)
	}
}

func TestEnabledWriteFailureReturnsFailure(t *testing.T) {
	root := setup(t)
	writeFixture(t, root, "var/lib/hostdiary/public/data/dirs", []byte("blocked"))
	if code := Run(root, os.Stderr, clock); code != 1 {
		t.Fatal(code)
	}
}
