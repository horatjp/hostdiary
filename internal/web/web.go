// Package web embeds the offline viewer and publishes missing or changed assets.
package web

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"hostdiary/internal/store"
)

//go:embed index.html llms.txt assets
var files embed.FS

// assetFingerprint hashes names, lengths and contents in WalkDir's lexical order.
// Framing each file prevents different path/content boundaries hashing alike.
func assetFingerprint(source fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%d:%s:%d:", len(path), path, len(b))
		h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:4]), nil
}

// WriteAssets stages files outside public, and marks the version last so a failed
// update is retried. Matching version+fingerprint and present files skip writes.
func WriteAssets(public, version string) error {
	fingerprint, err := assetFingerprint(files)
	if err != nil {
		return err
	}
	want := version + "+" + fingerprint
	marker := filepath.Join(public, "assets/VERSION")
	b, err := os.ReadFile(marker)
	if err == nil && strings.TrimSpace(string(b)) == want {
		complete := true
		err = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			st, err := os.Stat(filepath.Join(public, filepath.FromSlash(path)))
			if os.IsNotExist(err) {
				complete = false
				return nil
			}
			if err != nil {
				return err
			}
			if !st.Mode().IsRegular() {
				complete = false
			}
			return nil
		})
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	mkdir := func(path string) error {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		return os.Chmod(path, 0755)
	}
	if err = mkdir(public); err != nil {
		return err
	}
	private := filepath.Dir(public)
	err = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		dest := filepath.Join(public, filepath.FromSlash(path))
		if d.IsDir() {
			return mkdir(dest)
		}
		b, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		return store.AtomicWrite(dest, b, 0644, private)
	})
	if err != nil {
		return err
	}
	return store.AtomicWrite(marker, []byte(want+"\n"), 0644, private)
}
