package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// Local is the on-disk store. It is always present, even when a durable
// store is configured: a laptop that loses its network mid-incident still
// has the record of what it stopped.
type Local struct{ Dir string }

func (l Local) path(planID string) string {
	return filepath.Join(l.Dir, planID+snapshotExt)
}

// Put writes the snapshot atomically -- temp file, sync, rename -- so a
// crash mid-write cannot leave a truncated record where a complete one
// used to be.
func (l Local) Put(ctx context.Context, s model.Snapshot) error {
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temporary file and rename, so a crash mid-write cannot leave a
	// truncated snapshot where a complete one used to be.
	tmp, err := os.CreateTemp(l.Dir, ".snap-*")
	if err != nil {
		return err
	}
	// Removes the temp file on every path that did not rename it away. The
	// successful path renames first, so this finds nothing.
	defer func() { _ = os.Remove(tmp.Name()) }() //nolint:errcheck // see above
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write already failed; this file is going to be removed
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close() //nolint:errcheck // as above
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), l.path(s.PlanID))
}

// Get reads one snapshot. A corrupt file is reported as corrupt rather
// than as missing: those need different reactions.
func (l Local) Get(ctx context.Context, planID string) (model.Snapshot, error) {
	b, err := os.ReadFile(l.path(planID))
	if err != nil {
		if os.IsNotExist(err) {
			return model.Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, planID)
		}
		return model.Snapshot{}, err
	}
	var s model.Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return model.Snapshot{}, fmt.Errorf("snapshot %s is corrupt: %w", planID, err)
	}
	return s, nil
}

// List returns the snapshots in the directory, newest first. A missing
// directory is an empty list, not an error: nothing has been fired yet.
func (l Local) List(ctx context.Context) ([]model.Snapshot, error) {
	entries, err := os.ReadDir(l.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []model.Snapshot
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), snapshotExt) {
			continue
		}
		s, err := l.Get(ctx, strings.TrimSuffix(e.Name(), snapshotExt))
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return stamp(out[i]).After(stamp(out[j])) })
	return out, nil
}

// Describe names this store in errors and in warnings.
func (l Local) Describe() string { return "file://" + l.Dir }
