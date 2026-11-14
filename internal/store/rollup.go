// Rollup resumes from durable coarse rows; a daily row commits that group's day.
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// lastTime reads backwards in blocks, skipping incomplete tails and headers.
func lastTime(path string) (int64, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return 0, e
	}
	end := st.Size()
	var carry []byte
	tail := true
	parse := func(b []byte) (int64, bool) {
		var r []json.RawMessage
		var t int64
		if json.Unmarshal(b, &r) != nil || len(r) == 0 || json.Unmarshal(r[0], &t) != nil {
			return 0, false
		}
		return t, true
	}
	for end > 0 {
		start := max(int64(0), end-4096)
		chunk := make([]byte, end-start)
		_, e := f.ReadAt(chunk, start)
		if e != nil && e != io.EOF {
			return 0, e
		}
		data := append(chunk, carry...)
		parts := bytes.Split(data, []byte{'\n'})
		for i := len(parts) - 1; i >= 1; i-- {
			if tail {
				tail = false
				continue
			}
			if t, ok := parse(parts[i]); ok {
				return t, nil
			}
		}
		carry = parts[0]
		end = start
	}
	if !tail {
		if t, ok := parse(carry); ok {
			return t, nil
		}
	}
	return 0, nil
}

func readMinuteDay(path string) (Header, []namedRow, error) {
	var h, lastValid Header
	rows := []namedRow{}
	f, e := os.Open(path)
	if e != nil {
		return h, nil, e
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		b, e := reader.ReadBytes('\n')
		if e == io.EOF {
			break
		}
		if e != nil {
			return h, nil, e
		}
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			continue
		}
		if b[0] == '{' {
			var next Header
			if json.Unmarshal(b, &next) != nil {
				continue // Discard only the broken line.
			}
			h = next
			if h.Schema == 1 && h.Step == 60 {
				if _, _, e := coarseColumns(h.Columns, h.Units); e == nil {
					lastValid = h
					continue
				}
			}
			h = Header{}
			continue
		}
		if h.Schema != 1 {
			continue
		}
		var raw []json.RawMessage
		var at int64
		if json.Unmarshal(b, &raw) != nil || len(raw) != len(h.Columns) || json.Unmarshal(raw[0], &at) != nil {
			continue
		}
		values := map[string]*float64{}
		valid := true
		for i, c := range h.Columns[1:] {
			var v *float64
			if json.Unmarshal(raw[i+1], &v) != nil {
				valid = false
				break
			}
			values[c] = v
		}
		if valid {
			rows = append(rows, namedRow{at, values})
		}
	}
	return lastValid, rows, nil
}

type dailyCursor struct {
	year   string
	last   int64
	failed bool
}

func Rollup(public string, at time.Time) error {
	return rollup(public, at, appendRows)
}

func rollup(public string, at time.Time, appendDay func(string, Header, [][]any) error) error {
	root := filepath.Join(public, "data", "1m")
	days, e := os.ReadDir(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	cursors := map[string]*dailyCursor{}
	var failures []error
	today := at.UTC().Format("2006-01-02")
	// ReadDir sorts names; processing days outside groups preserves oldest-first order.
	for _, day := range days {
		name := day.Name()
		if !day.IsDir() || !ValidDay(name) || name >= today {
			continue
		}
		date, _ := time.Parse("2006-01-02", name)
		files, e := os.ReadDir(filepath.Join(root, name))
		if e != nil {
			failures = append(failures, fmt.Errorf("rollup day %s: %w", name, e))
			// Group membership is unknown for this day. Stop later days to avoid
			// advancing any group's durable cursor past possibly missing data.
			break
		}
		for _, file := range files {
			if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".jsonl" {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".jsonl")
			cur := cursors[id]
			if cur == nil {
				cur = &dailyCursor{}
				cursors[id] = cur
			}
			if cur.failed {
				continue
			}
			process := func() error {
				year := name[:4]
				if cur.year != year {
					last, e := lastTime(filepath.Join(public, "data/1d", year, id+".jsonl"))
					if e != nil {
						return e
					}
					cur.year = year
					cur.last = last
				}
				if cur.last >= date.Unix() {
					return nil
				}
				h, rows, e := readMinuteDay(filepath.Join(root, name, file.Name()))
				if e != nil {
					return e
				}
				if len(rows) == 0 {
					return nil // Empty/unsupported input is absent data, not an I/O failure.
				}
				columns, units, e := coarseColumns(h.Columns, h.Units)
				if e != nil {
					return e
				}
				for _, res := range []struct {
					name string
					step int64
				}{{"5m", 300}, {"1h", 3600}, {"1d", 86400}} {
					period := name[:7]
					if res.name == "1d" {
						period = year
					}
					path := filepath.Join(public, "data", res.name, period, id+".jsonl")
					last := cur.last
					if res.name != "1d" {
						last, e = lastTime(path)
						if e != nil {
							return e
						}
					}
					buckets, e := aggregate(rows, res.step, date.Unix())
					if e != nil {
						return e
					}
					output := [][]any{}
					for _, b := range buckets {
						if b.Time <= last {
							continue
						}
						r := []any{b.Time, b.Samples, b.LastTime}
						for _, c := range h.Columns[1:] {
							r = append(r, b.Mean[c], b.Max[c])
						}
						output = append(output, r)
					}
					coarse := h
					coarse.Step = int(res.step)
					coarse.Columns = columns
					coarse.Units = units
					if e := appendDay(path, coarse, output); e != nil {
						return e
					}
					if res.name == "1d" && len(output) > 0 {
						cur.last = output[len(output)-1][0].(int64)
					}
				}
				return nil
			}
			if e := process(); e != nil {
				cur.failed = true
				failures = append(failures, fmt.Errorf("rollup %s/%s: %w", name, id, e))
			}
		}
	}
	return errors.Join(failures...)
}
