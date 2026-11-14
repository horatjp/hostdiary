package dirs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hostdiary/internal/store"
)

type Document struct {
	Schema      int          `json:"schema"`
	Kind        string       `json:"kind"`
	StartedAt   int64        `json:"started_at"`
	FinishedAt  int64        `json:"finished_at"`
	Depth       int          `json:"depth"`
	Top         int          `json:"top"`
	Filesystems []Filesystem `json:"filesystems"`
}

func writeDocument(base, dir string, doc Document) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	day := time.Unix(doc.StartedAt, 0).UTC().Format("2006-01-02")
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	// Only the JSON name advertises a complete pair to collect's future index.
	js := []byte(fmt.Sprintf("HD.dirs(%q, %s);\n", day, b))
	if err = store.AtomicWrite(filepath.Join(dir, day+".js"), js, 0644, base); err != nil {
		return err
	}
	return store.AtomicWrite(filepath.Join(dir, day+".json"), append(b, '\n'), 0644, base)
}

func datedName(name string) (string, bool) {
	ext := filepath.Ext(name)
	if ext != ".json" && ext != ".js" {
		return "", false
	}
	day := strings.TrimSuffix(name, ext)
	return day, store.ValidDay(day)
}

// pruneFiles uses names, never the wall clock; disabled also removes orphan JS mirrors.
func pruneFiles(dir string, disabled bool, keepDay string) error {
	return pruneFilesWithRemove(dir, disabled, keepDay, os.Remove)
}

// The remove function lets tests fail a mirror deletion after removing JSON.
func pruneFilesWithRemove(dir string, disabled bool, keepDay string, remove func(string) error) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	days := []string{}
	for _, f := range files {
		day, ok := datedName(f.Name())
		if ok && !f.IsDir() && filepath.Ext(f.Name()) == ".json" {
			days = append(days, day)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	cutoff := ""
	if len(days) > 31 {
		cutoff = days[31]
	}
	// Remove advertised JSON names before mirrors, even if a later removal fails.
	for _, ext := range []string{".json", ".js"} {
		for _, f := range files {
			day, ok := datedName(f.Name())
			if !ok || f.IsDir() || filepath.Ext(f.Name()) != ext {
				continue
			}
			if disabled || day != keepDay && cutoff != "" && day <= cutoff {
				if err := remove(filepath.Join(dir, f.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
