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

// Log is the append-only record of what this tool was asked to do and what
// it did. It is the answer to "who stopped production", so every event
// carries who and when and nothing can overwrite them.
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

// Discard is the log for a run with nowhere to write: events are formed and
// dropped, so a failed open never changes the caller's control flow.
func Discard() *Log { return &Log{w: io.Discard, who: whoami()} }

// To logs to an arbitrary writer, which is how a test reads what was
// recorded without going through a file.
func To(w io.Writer) *Log { return &Log{w: w, who: whoami()} }

// Event appends one JSON record. A nil Log is usable and writes nothing, so
// no caller has to check before logging.
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
	// An audit file that cannot be written must not be the reason a cost
	// incident goes unhandled -- that is the same decision New makes when the
	// file will not open.
	_, _ = l.w.Write(append(b, '\n')) //nolint:errcheck // see above
}

func (l *Log) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now() //nolint:forbidigo // the default for l.now, which a test sets
}

func whoami() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	//nolint:forbidigo // the fallback identity for a container with no passwd
	// entry. Read here because it is only needed when user.Current fails,
	// and an audit record with no author is worse than one from $USER.
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "unknown"
}
