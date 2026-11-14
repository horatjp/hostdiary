// Shared mirror preparation/rollback keeps minute writes and coarse batches consistent.
package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type mirrorTarget struct {
	path, ext string
	size      int64
	header    Header
}

func prepareMirrors(base string) ([]mirrorTarget, error) {
	targets := []mirrorTarget{}
	for _, ext := range []string{".jsonl", ".js"} {
		p := base + ext
		if e := RepairTail(p); e != nil {
			return nil, e
		}
		h, e := lastHeader(p, ext)
		if e != nil {
			return nil, e
		}
		st, e := os.Stat(p)
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		var size int64
		if e == nil {
			size = st.Size()
		}
		targets = append(targets, mirrorTarget{p, ext, size, h})
	}
	return targets, nil
}
func rollbackMirrors(targets []mirrorTarget, err error) error {
	for _, dst := range targets {
		if e := os.Truncate(dst.path, dst.size); e != nil && !os.IsNotExist(e) {
			err = fmt.Errorf("%w; rollback %s: %w", err, dst.path, e)
		}
	}
	return err
}

func appendRows(path string, h Header, rows [][]any) error {
	return appendRowsWith(path, h, rows, writeBatch)
}

func appendRowsWith(path string, h Header, rows [][]any, write func(string, []byte) error) (err error) {
	if len(rows) == 0 {
		return nil
	}
	if len(h.Columns) != len(h.Units) {
		return fmt.Errorf("column count mismatch")
	}
	lines := make([][]byte, 0, len(rows))
	for _, r := range rows {
		if len(r) != len(h.Columns) {
			return fmt.Errorf("column count mismatch")
		}
		b, e := json.Marshal(r)
		if e != nil {
			return e
		}
		lines = append(lines, b)
	}
	hb, e := json.Marshal(h)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	base := strings.TrimSuffix(path, ".jsonl")
	targets, e := prepareMirrors(base)
	if e != nil {
		return e
	}
	defer func() {
		if err != nil {
			err = rollbackMirrors(targets, err)
		}
	}()
	// The key is relative to data/: resolution/period/group.
	dir := filepath.Dir(base)
	key := filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(dir)), filepath.Base(dir), filepath.Base(base)))
	kb, _ := json.Marshal(key)
	for _, dst := range targets {
		var buf []byte
		add := func(b []byte) {
			if dst.ext == ".js" {
				buf = fmt.Appendf(buf, "HD.l(%s,%s);\n", kb, b)
			} else {
				buf = append(buf, b...)
				buf = append(buf, '\n')
			}
		}
		if !reflect.DeepEqual(dst.header, h) {
			add(hb)
		}
		for _, b := range lines {
			add(b)
		}
		if e := write(dst.path, buf); e != nil {
			return e
		}
	}
	return nil
}
func writeBatch(path string, b []byte) error {
	return writeBatchOpening(path, b, func(path string) (batchFile, error) {
		return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	})
}

type batchFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func writeBatchOpening(path string, b []byte, open func(string) (batchFile, error)) error {
	f, e := open(path)
	if e != nil {
		return e
	}
	n, e := f.Write(b)
	resolution := filepath.Base(filepath.Dir(filepath.Dir(path)))
	// Persist both precursor resolutions before a daily completion marker can be written.
	if e == nil && n == len(b) && (resolution == "5m" || resolution == "1h") {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return ce
}
