package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestWriteAssetsVersionAndPermissions(t *testing.T) {
	base := t.TempDir()
	public := filepath.Join(base, "public")
	if err := WriteAssets(public, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	names := []string{"index.html", "llms.txt", "assets/app.js", "assets/i18n.js", "assets/views.js", "assets/app.css", "assets/vendor/uplot-1.6.32/uPlot.iife.min.js", "assets/vendor/uplot-1.6.32/uPlot.min.css", "assets/vendor/uplot-1.6.32/LICENSE", "assets/VERSION"}
	for _, name := range names {
		p := filepath.Join(public, name)
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0644 {
			t.Fatal(name, st, err)
		}
	}
	marker := time.Unix(123456, 0)
	path := filepath.Join(public, "index.html")
	if err := os.Chtimes(path, marker, marker); err != nil {
		t.Fatal(err)
	}
	if err := WriteAssets(public, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if !st.ModTime().Equal(marker) {
		t.Fatal("same version rewrote files")
	}
	if err := WriteAssets(public, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(path)
	if st.ModTime().Equal(marker) {
		t.Fatal("new version did not rewrite files")
	}
	version, err := os.ReadFile(filepath.Join(public, "assets/VERSION"))
	fingerprint, hashErr := assetFingerprint(files)
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	if err != nil || string(version) != "0.2.0+"+fingerprint+"\n" {
		t.Fatal(string(version), err)
	}
	if err := filepath.WalkDir(public, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		want := fs.FileMode(0644)
		if d.IsDir() {
			want = 0755
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s: %o", p, st.Mode().Perm())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAssetsAreRestored(t *testing.T) {
	for _, name := range []string{"index.html", "assets/app.js", "assets/i18n.js", "assets/views.js", "assets/app.css", "assets/vendor/uplot-1.6.32/LICENSE"} {
		t.Run(name, func(t *testing.T) {
			public := filepath.Join(t.TempDir(), "public")
			if err := WriteAssets(public, "0.1.0"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(public, filepath.FromSlash(name))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := WriteAssets(public, "0.1.0"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			want, readErr := files.ReadFile(name)
			if err != nil || readErr != nil || string(got) != string(want) {
				t.Fatal(name, err, readErr)
			}
		})
	}
}

func TestAssetFingerprintChangesWithContentAndPath(t *testing.T) {
	base := fstest.MapFS{"index.html": {Data: []byte("index")}, "assets/app.js": {Data: []byte("one")}}
	first, err := assetFingerprint(base)
	if err != nil || len(first) != 8 {
		t.Fatal(first, err)
	}
	again, err := assetFingerprint(base)
	if err != nil || again != first {
		t.Fatal(again, err)
	}
	for _, changed := range []fstest.MapFS{
		{"index.html": {Data: []byte("index")}, "assets/app.js": {Data: []byte("two")}},
		{"index.html": {Data: []byte("index")}, "assets/other.js": {Data: []byte("one")}},
	} {
		got, err := assetFingerprint(changed)
		if err != nil || got == first {
			t.Fatal(got, err)
		}
	}
}

func TestFingerprintMismatchRewritesSameVersion(t *testing.T) {
	public := filepath.Join(t.TempDir(), "public")
	if err := WriteAssets(public, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(public, "index.html")
	os.WriteFile(path, []byte("old"), 0644)
	guide := filepath.Join(public, "llms.txt")
	os.WriteFile(guide, []byte("old guide"), 0644)
	os.WriteFile(filepath.Join(public, "assets/VERSION"), []byte("0.1.0+00000000\n"), 0644)
	if err := WriteAssets(public, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	want, _ := files.ReadFile("index.html")
	if err != nil || string(got) != string(want) {
		t.Fatal(err)
	}
	got, err = os.ReadFile(guide)
	want, _ = files.ReadFile("llms.txt")
	if err != nil || string(got) != string(want) {
		t.Fatal("same-version guide update failed", err)
	}
}

func TestGuideExplainsObservedHistoryLimits(t *testing.T) {
	public := filepath.Join(t.TempDir(), "public")
	if err := WriteAssets(public, "fixture-version"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(public, "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"1095", "three year files", "current boot", "not necessarily the current boot", `"complete"`, "does not guarantee", "absent day, group or column", "not zero", "old copy", "Questions and where to look", "not record reasons"} {
		if !strings.Contains(string(b), phrase) {
			t.Errorf("guide is missing history limit: %s", phrase)
		}
	}
}

func TestFailedAssetWriteDoesNotMarkVersion(t *testing.T) {
	base := t.TempDir()
	public := filepath.Join(base, "public")
	os.MkdirAll(filepath.Join(public, "assets/app.js"), 0755)
	if err := WriteAssets(public, "0.1.0"); err == nil {
		t.Fatal("expected rename failure")
	}
	if _, err := os.Stat(filepath.Join(public, "assets/VERSION")); !os.IsNotExist(err) {
		t.Fatal("failed update marked version")
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
}
