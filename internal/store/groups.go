package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

type Group struct {
	ID           string              `json:"id"`
	Kind         string              `json:"kind"`
	Name         string              `json:"name"`
	Columns      []string            `json:"columns"`
	Units        []string            `json:"units"`
	Descriptions []string            `json:"descriptions"`
	LastSeen     int64               `json:"last_seen"`
	Days         []string            `json:"days"`
	Coarse       map[string][]string `json:"coarse"`
	Traffic      *traffic            `json:"traffic,omitempty"`
}
type Header struct {
	Schema  int      `json:"schema"`
	Group   string   `json:"group"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Step    int      `json:"step"`
	Columns []string `json:"columns"`
	Units   []string `json:"units"`
}
type Unavailable struct {
	Group  string  `json:"group"`
	Column *string `json:"column"`
	Reason string  `json:"reason"`
}
type Index struct {
	Schema        int             `json:"schema"`
	Version       string          `json:"version"`
	Host          string          `json:"host"`
	GeneratedAt   int64           `json:"generated_at"`
	Boots         []int64         `json:"boots"`
	Unavailable   []Unavailable   `json:"unavailable"`
	Groups        []Group         `json:"groups"`
	Resolutions   []resolution    `json:"resolutions"`
	CoarseColumns string          `json:"coarse_columns"`
	Dirs          *directoryIndex `json:"dirs,omitempty"`
	Guide         string          `json:"guide"`   // AI reading guide beside the index
	Summary       string          `json:"summary"` // per-day statistics (WriteSummary)
}

func (g Group) Header() Header { return Header{1, g.ID, g.Kind, g.Name, 60, g.Columns, g.Units} }

func AppendGroup(public string, g Group, at int64, row []any) error {
	return appendGroup(public, g, at, row, AppendLine)
}

// appendLine is injectable to verify rollback after partial/failed filesystem I/O.
func appendGroup(public string, g Group, at int64, row []any, appendLine func(string, []byte) error) (err error) {
	if len(row) != len(g.Columns) || len(g.Units) != len(g.Columns) {
		return fmt.Errorf("column count mismatch")
	}
	day := time.Unix(at, 0).UTC().Format("2006-01-02")
	dir := filepath.Join(public, "data", "1m", day)
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	key := "1m/" + day + "/" + g.ID
	h := g.Header()
	hb, e := json.Marshal(h)
	if e != nil {
		return e
	}
	rb, e := json.Marshal(row)
	if e != nil {
		return e
	}
	targets, e := prepareMirrors(filepath.Join(dir, g.ID))
	if e != nil {
		return e
	}
	defer func() {
		if err != nil {
			err = rollbackMirrors(targets, err)
		}
	}()
	for _, dst := range targets {
		lines := [][]byte{}
		if !reflect.DeepEqual(dst.header, h) {
			lines = append(lines, hb)
		}
		lines = append(lines, rb)
		for _, b := range lines {
			if dst.ext == ".js" {
				k, _ := json.Marshal(key)
				b = fmt.Appendf(nil, "HD.l(%s,%s);", k, b)
			}
			if e := appendLine(dst.path, b); e != nil {
				return e
			}
		}
	}
	return nil
}
func lastHeader(path, ext string) (Header, error) {
	var h Header
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return h, nil
	}
	if e != nil {
		return h, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1024*1024)
	for s.Scan() {
		b := s.Bytes()
		if ext == ".js" {
			i := bytes.IndexByte(b, ',')
			if i < 0 || !bytes.HasSuffix(b, []byte(");")) {
				continue
			}
			b = b[i+1 : len(b)-2]
		}
		var next Header
		if json.Unmarshal(b, &next) == nil && next.Schema > 0 {
			h = next
		}
	}
	return h, s.Err()
}
func WriteIndex(public string, index Index) error {
	return writeIndex(public, index, recoverCoarseGroup)
}

// Recovery is injectable so fallback behavior can be tested under any UID.
func writeIndex(public string, index Index, recoverGroup func([]coarseFile, Group) (Group, error)) error {
	if index.Boots == nil {
		index.Boots = []int64{}
	}
	if index.Unavailable == nil {
		index.Unavailable = []Unavailable{}
	}
	// Recover old groups from retained history, so absent devices remain visible.
	meta := map[string]Group{}
	for _, g := range index.Groups {
		meta[g.ID] = g
	}
	previous, e := os.ReadFile(filepath.Join(public, "index.json"))
	if e == nil {
		var old Index
		if json.Unmarshal(previous, &old) == nil {
			for _, g := range old.Groups {
				if _, ok := meta[g.ID]; !ok {
					meta[g.ID] = g
				}
			}
		}
	}
	found := map[string]Group{}
	var skipped []error
	root := filepath.Join(public, "data", "1m")
	days, e := os.ReadDir(root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, day := range days {
		if !day.IsDir() {
			continue
		}
		if !ValidDay(day.Name()) {
			continue
		}
		files, e := os.ReadDir(filepath.Join(root, day.Name()))
		if e != nil {
			return e
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".jsonl" {
				continue
			}
			p := filepath.Join(root, day.Name(), file.Name())
			f, e := os.Open(p)
			if e != nil {
				skipped = append(skipped, fmt.Errorf("index read %s: %w", p, e))
				continue
			}
			s := bufio.NewScanner(f)
			s.Buffer(make([]byte, 4096), 1024*1024)
			var h Header
			var last int64
			for s.Scan() {
				b := s.Bytes()
				if len(b) == 0 {
					continue
				}
				if b[0] == '{' {
					var next Header
					if json.Unmarshal(b, &next) == nil && next.Schema == 1 {
						h = next
					}
				} else {
					var row []json.RawMessage
					if json.Unmarshal(b, &row) == nil && len(row) > 0 {
						var at int64
						if json.Unmarshal(row[0], &at) == nil && at > last {
							last = at
						}
					}
				}
			}
			e = s.Err()
			f.Close()
			if e != nil {
				skipped = append(skipped, fmt.Errorf("index read %s: %w", p, e))
				continue
			}
			if h.Group == "" || last == 0 {
				continue
			}
			g, ok := found[h.Group]
			if !ok {
				g = meta[h.Group]
				g.ID = h.Group
				g.Kind = h.Kind
				g.Name = h.Name
				g.Columns = h.Columns
				g.Units = h.Units
				g.Days = []string{}
				g.Coarse = nil // Rebuild inventories from this run's stat results.
				g.LastSeen = 0
			}
			g.Days = append(g.Days, day.Name())
			if last > g.LastSeen {
				g.LastSeen = last
				g.Columns = h.Columns
				g.Units = h.Units
			}
			found[g.ID] = g
		}
	}
	// Choose descriptions only after the latest minute header is known. An older
	// day's shorter header must not replace current metadata during the scan.
	for id, g := range found {
		g.Descriptions = meta[id].Descriptions
		if len(g.Descriptions) != len(g.Columns) {
			g.Descriptions = append([]string{}, g.Columns...)
		}
		found[id] = g
	}
	coarse, e := listCoarse(public)
	if e != nil {
		skipped = append(skipped, e)
	}
	for id, files := range coarse {
		g, ok := found[id]
		if !ok {
			g, e = recoverGroup(files, meta[id])
			if g.ID == "" && e == nil {
				e = fmt.Errorf("index coarse %s: no readable rows", id)
			}
			if e != nil {
				skipped = append(skipped, e)
			}
			if g.ID != "" && g.ID != id {
				skipped = append(skipped, fmt.Errorf("index coarse %s: recovered group ID %q does not match file ID", id, g.ID))
				continue
			}
			if e != nil || g.ID == "" {
				// Keep descriptions and last_seen through temporary recovery failures;
				// the inventory below still reflects this run's stat results.
				g = meta[id]
				g.Days = []string{} // No minute history was found in this run.
				if g.ID == "" {
					continue
				}
			}
		}
		g.Coarse = coarsePeriods(files)
		found[id] = g
	}
	index.Guide = "llms.txt"
	index.Summary = "summary.json"
	index.Resolutions = resolutions()
	index.CoarseColumns = coarseColumnsDescription
	index.Groups = []Group{}
	for _, g := range found {
		if g.Coarse == nil {
			g.Coarse = coarsePeriods(nil)
		}
		g.Traffic = nil
		if g.Kind == "net" {
			value, e := readTraffic(public, g.ID, time.Unix(index.GeneratedAt, 0))
			if e != nil {
				skipped = append(skipped, e)
			} else {
				g.Traffic = &value
			}
		}
		sort.Strings(g.Days)
		index.Groups = append(index.Groups, g)
	}
	sort.Slice(index.Groups, func(i, j int) bool { return index.Groups[i].ID < index.Groups[j].ID })
	index.Dirs, e = readDirectoryIndex(public)
	if e != nil {
		skipped = append(skipped, e)
	}
	b, e := json.Marshal(index)
	if e != nil {
		return errors.Join(append(skipped, e)...)
	}
	if e = AtomicWrite(filepath.Join(public, "index.json"), append(b, '\n'), 0644, filepath.Dir(public)); e != nil {
		return errors.Join(append(skipped, e)...)
	}
	e = AtomicWrite(filepath.Join(public, "index.js"), append(append([]byte("HD.index("), b...), []byte(");\n")...), 0644, filepath.Dir(public))
	return errors.Join(append(skipped, e)...)
}
