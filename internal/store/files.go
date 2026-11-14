// Package store owns the public append-only files and their index.
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func GroupID(kind, name string) string {
	switch kind {
	case "cpu", "load", "mem", "swap", "procs", "tcp", "psi":
		return kind
	}
	var b strings.Builder
	for _, r := range name {
		if b.Len() >= 32 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	h := sha256.Sum256([]byte(name))
	return kind + "-" + b.String() + "-" + hex.EncodeToString(h[:4])
}
func ExpiredDays(names []string, keep int) []string {
	if keep < 1 {
		panic("keep must be positive")
	}
	days := []string{}
	for _, n := range names {
		if ValidDay(n) {
			days = append(days, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	if len(days) <= keep {
		return []string{}
	}
	return days[keep:]
}

// RepairTail scans backwards in bounded blocks, including a partial line >4 KiB.
func RepairTail(path string) error {
	f, e := os.OpenFile(path, os.O_RDWR, 0)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	end := st.Size()
	if end == 0 {
		return nil
	}
	buf := make([]byte, 4096)
	for end > 0 {
		start := max(int64(0), end-int64(len(buf)))
		n, e := f.ReadAt(buf[:end-start], start)
		if e != nil && e != io.EOF {
			return e
		}
		if i := bytes.LastIndexByte(buf[:n], '\n'); i >= 0 {
			length := start + int64(i) + 1
			if length == st.Size() {
				return nil // A complete file must retain its mtime for readers/backups.
			}
			return f.Truncate(length)
		}
		end = start
	}
	return f.Truncate(0)
}
func AppendLine(path string, line []byte) error {
	if bytes.ContainsAny(line, "\r\n") {
		return fmt.Errorf("expected a single line")
	}
	if e := RepairTail(path); e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if e != nil {
		return e
	}
	b := append(append([]byte{}, line...), '\n')
	n, e := f.Write(b)
	ce := f.Close()
	if e != nil {
		return e
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return ce
}

// AtomicWrite stages in the caller's private directory on the same filesystem.
func AtomicWrite(path string, b []byte, mode os.FileMode, tempDir string) error {
	f, e := os.CreateTemp(tempDir, ".hostdiary-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
