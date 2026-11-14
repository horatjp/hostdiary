package store

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type directoryIndex struct {
	Days        []string `json:"days"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
}

const directoryDescription = "Daily disk usage of directories up to depth 2 per filesystem, top 20 by bytes (st_blocks × 512, hard links once, one filesystem). Parent and child entries overlap; do not add them. depth is relative to the mount point. Entries list directories only; files directly under the mount point count in total but have no entry. A path absent on a day was outside that day's top 20 or did not exist; absence does not mean zero. total is the sum of the walked tree (like du -xs) and differs from the disk group's used bytes (deleted-but-open files, reserved blocks, files hidden under mount points). errors counts entries skipped (unreadable or vanished during the walk); complete=false means at least one was unreadable. Non-printable/non-UTF-8 bytes and backslashes in paths are escaped as \\xHH." + " When complete=false, total and entries are lower bounds; a filesystem whose mount point itself could not be read has total 0 and no entries. Blocks shared by hard links are attributed to the first path visited in lexical order."

// Only names are read: partial or malformed daily contents do not change inventory.
func readDirectoryIndex(public string) (*directoryIndex, error) {
	files, err := os.ReadDir(filepath.Join(public, "data/dirs"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	days := []string{}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		day := strings.TrimSuffix(f.Name(), ".json")
		if ValidDay(day) {
			days = append(days, day)
		}
	}
	if len(days) == 0 {
		return nil, nil
	}
	return &directoryIndex{days, "data/dirs/{day}.json", directoryDescription}, nil
}

// ValidDay recognizes canonical, real calendar dates without consulting the clock.
func ValidDay(name string) bool {
	day, err := time.Parse("2006-01-02", name)
	return err == nil && day.Format("2006-01-02") == name
}
