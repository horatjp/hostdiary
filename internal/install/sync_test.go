package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameSyncsParentAfterReplacementAndPropagatesFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "sync-failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			tmp := filepath.Join(dir, ".hostdiary-test")
			writeTestFile(t, target, "old", 0644)
			writeTestFile(t, tmp, "new", 0644)
			called := false
			sentinel := errors.New("directory sync failure")
			err := renameAndSync(tmp, target, func(path string) error {
				called = true
				if path != dir {
					t.Fatal(path)
				}
				got, err := os.ReadFile(target)
				if err != nil || string(got) != "new" {
					t.Fatal("sync before rename", err)
				}
				if _, err := os.Stat(tmp); !os.IsNotExist(err) {
					t.Fatal("temporary name still present", err)
				}
				if fail {
					return sentinel
				}
				return syncDirectory(path)
			})
			if !called || fail && !errors.Is(err, sentinel) || !fail && err != nil {
				t.Fatal(called, err)
			}
		})
	}
}

func TestRenameFailureDoesNotSyncParent(t *testing.T) {
	dir := t.TempDir()
	err := renameAndSync(filepath.Join(dir, "missing"), filepath.Join(dir, "target"), func(string) error { t.Error("sync after failed rename"); return nil })
	if err == nil {
		t.Fatal("expected rename failure")
	}
}
