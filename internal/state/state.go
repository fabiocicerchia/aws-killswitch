// Package state persists the snapshot that makes a fire reversible.
//
// The ordering rule is absolute: the snapshot is written, and read back, before
// a single API call changes anything. If the write fails the fire is abandoned.
// An account that is still expensive is a problem; an account that is stopped
// with no record of how to start it is an outage of unknown length.
//
// S3 is the intended home — it is in the never-touch set, so the kill switch
// cannot destroy its own restore — with a local copy alongside for the case
// where the reason you are firing is that something is wrong with the account.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

type Store interface {
	Put(ctx context.Context, s model.Snapshot) error
	Get(ctx context.Context, planID string) (model.Snapshot, error)
	List(ctx context.Context) ([]model.Snapshot, error)
	Describe() string
}

var ErrNotFound = errors.New("no snapshot with that plan id")

// snapshotExt is the extension every store writes and every listing looks for.
// Named once because Put and List have to agree: a store that writes one suffix
// and scans for another loses the restore record without failing.
const snapshotExt = ".json"

// PutVerified writes and reads back, comparing what returned against what was
// sent. A store that accepts a write and loses it is the one failure this tool
// cannot survive, and it is cheap to rule out.
func PutVerified(ctx context.Context, s Store, snap model.Snapshot) error {
	if err := s.Put(ctx, snap); err != nil {
		return fmt.Errorf("writing snapshot to %s: %w", s.Describe(), err)
	}
	back, err := s.Get(ctx, snap.PlanID)
	if err != nil {
		return fmt.Errorf("snapshot written to %s but could not be read back: %w", s.Describe(), err)
	}
	if len(back.Entries) != len(snap.Entries) {
		return fmt.Errorf("snapshot in %s has %d entries, wrote %d — refusing to proceed without a reliable restore record",
			s.Describe(), len(back.Entries), len(snap.Entries))
	}
	return nil
}

// Multi writes to every store and requires all of them to succeed, so the
// local copy and the durable copy cannot disagree about what was stopped.
type Multi struct{ Stores []Store }

func (m Multi) Put(ctx context.Context, s model.Snapshot) error {
	for _, st := range m.Stores {
		if err := st.Put(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", st.Describe(), err)
		}
	}
	return nil
}

// Get reads from the first store that has it. Order matters: the durable store
// should come first, since the local one may be on a machine that was rebuilt.
func (m Multi) Get(ctx context.Context, planID string) (model.Snapshot, error) {
	var lastErr error
	for _, st := range m.Stores {
		snap, err := st.Get(ctx, planID)
		if err == nil {
			return snap, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNotFound
	}
	return model.Snapshot{}, lastErr
}

func (m Multi) List(ctx context.Context) ([]model.Snapshot, error) {
	seen := map[string]model.Snapshot{}
	for _, st := range m.Stores {
		list, err := st.List(ctx)
		if err != nil {
			continue
		}
		for _, s := range list {
			// A later store may hold a fresher copy of the same plan.
			if prev, ok := seen[s.PlanID]; !ok || fresher(s, prev) {
				seen[s.PlanID] = s
			}
		}
	}
	return sorted(seen), nil
}

func (m Multi) Describe() string {
	parts := make([]string, 0, len(m.Stores))
	for _, s := range m.Stores {
		parts = append(parts, s.Describe())
	}
	return strings.Join(parts, " + ")
}

func fresher(a, b model.Snapshot) bool {
	return stamp(a).After(stamp(b))
}

func stamp(s model.Snapshot) time.Time {
	if s.Restored != nil {
		return *s.Restored
	}
	if s.Fired != nil {
		return *s.Fired
	}
	return s.CreatedAt
}

func sorted(m map[string]model.Snapshot) []model.Snapshot {
	out := make([]model.Snapshot, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return stamp(out[i]).After(stamp(out[j])) })
	return out
}

// --- local -------------------------------------------------------------------

type Local struct{ Dir string }

func (l Local) path(planID string) string {
	return filepath.Join(l.Dir, planID+snapshotExt)
}

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
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
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

func (l Local) Describe() string { return "file://" + l.Dir }

// --- building the snapshot ---------------------------------------------------

// From converts a plan into the record that will make it reversible.
func From(p model.Plan) model.Snapshot {
	snap := model.Snapshot{
		PlanID: p.ID, CreatedAt: p.CreatedAt,
		Account: p.Account, Regions: p.Regions,
	}
	for _, a := range p.Actions {
		snap.Entries = append(snap.Entries, model.Entry{
			Kind: a.Resource.Kind, ID: a.Resource.ID, ARN: a.Resource.ARN,
			Name: a.Resource.Name, Region: a.Resource.Region, Phase: a.Phase,
			Prior: a.Resource.Prior,
		})
	}
	return snap
}

// Build assembles the store a run should use: the durable one first, so a
// rebuilt laptop still finds the record, with the local directory behind it.
//
// localDir may be empty, which is the Lambda's case — there is no durable
// filesystem there, and a local copy that dies with the execution environment
// would be a restore record that does not exist.
func Build(s3c *s3.Client, uri, localDir string) (Store, error) {
	var stores []Store
	if uri != "" {
		bucket, prefix, ok := ParseURI(uri)
		if !ok {
			return nil, fmt.Errorf("state_uri must be s3://bucket/prefix, got %q", uri)
		}
		stores = append(stores, S3{Client: s3c, Bucket: bucket, Prefix: prefix})
	}
	if localDir != "" {
		stores = append(stores, Local{Dir: localDir})
	}
	switch len(stores) {
	case 0:
		return nil, errors.New("no state store: set state_uri, or pass a local directory")
	case 1:
		return stores[0], nil
	}
	return Multi{Stores: stores}, nil
}
