// Package audit is the append-only record of what this tool did.
//
// A kill switch is answerable afterwards: someone will ask which resources were
// touched, when, by whom, and whether the thing that is still broken was broken
// by this. One JSON object per line, never rewritten.
package audit

import (
	"encoding/json"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"
)

type Log struct {
	mu  sync.Mutex
	w   io.Writer
	who string
	now func() time.Time
}

// New opens an append-only log. A log that cannot be opened is not fatal — an
// unwritable audit file must not be the reason a cost incident goes unhandled —
// but it is reported by the caller.
func New(path string) (*Log, error) {
	if path == "" {
		return Discard(), nil
	}
	// The default path is inside a directory that will not exist on a first
	// run, and "no such file or directory" is a confusing first impression for
	// a tool whose whole job is to be trustworthy.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Discard(), err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Discard(), err
	}
	return &Log{w: f, who: whoami()}, nil
}

func Discard() *Log { return &Log{w: io.Discard, who: whoami()} }

func To(w io.Writer) *Log { return &Log{w: w, who: whoami()} }

func (l *Log) Event(kind string, fields map[string]any) {
	if l == nil || l.w == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	rec := map[string]any{
		"ts": l.clock().UTC().Format(time.RFC3339Nano),
		"ev": kind,
		"by": l.who,
	}
	for k, v := range fields {
		// Never let a field overwrite the provenance of the record.
		if k == "ts" || k == "ev" || k == "by" {
			continue
		}
		rec[k] = v
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = l.w.Write(append(b, '\n'))
}

func (l *Log) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func whoami() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "unknown"
}
