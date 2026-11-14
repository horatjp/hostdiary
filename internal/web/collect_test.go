package web_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"hostdiary/internal/collect"
)

func TestCollectPublishesViewerOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../testdata")); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	at := int64(1793404800)
	if code := collect.Run(root, &log, func() time.Time { return time.Unix(at, 0) }); code != 0 {
		t.Fatal(code, log.String())
	}
	public := filepath.Join(root, "var/lib/hostdiary/public")
	html := filepath.Join(public, "index.html")
	marker := time.Unix(123456, 0)
	if err := os.Chtimes(html, marker, marker); err != nil {
		t.Fatal(err)
	}
	if code := collect.Run(root, &log, func() time.Time { return time.Unix(at+60, 0) }); code != 0 {
		t.Fatal(code, log.String())
	}
	st, err := os.Stat(html)
	if err != nil || !st.ModTime().Equal(marker) {
		t.Fatal("same-version collect rewrote viewer", err)
	}
	version, err := os.ReadFile(filepath.Join(public, "assets/VERSION"))
	if err != nil || !regexp.MustCompile(`^`+regexp.QuoteMeta(collect.Version)+`\+[a-f0-9]{8}\n$`).Match(version) {
		t.Fatal(string(version), err)
	}
}
