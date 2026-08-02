package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

func snap(id string, n int) model.Snapshot {
	s := model.Snapshot{PlanID: id, CreatedAt: time.Unix(1700000000, 0).UTC(), Account: "1234"}
	for i := 0; i < n; i++ {
		s.Entries = append(s.Entries, model.Entry{
			Kind: model.KindASG, ID: "asg-" + string(rune('a'+i)), Region: "eu-west-1",
			Prior: map[string]any{"desired_capacity": 3, "min_size": 1, "max_size": 6},
		})
	}
	return s
}

// The prior state has to survive the file, or restore puts back the wrong
// numbers — and JSON turns every integer into a float on the way back.
func TestLocalStoreRoundTripsPriorStateAsNumbers(t *testing.T) {
	l := Local{Dir: t.TempDir()}
	ctx := context.Background()
	if err := l.Put(ctx, snap("p1", 1)); err != nil {
		t.Fatal(err)
	}
	got, err := l.Get(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("got %d entries", len(got.Entries))
	}
	for key, want := range map[string]int{"desired_capacity": 3, "min_size": 1, "max_size": 6} {
		v, ok := model.PriorInt(got.Entries[0].Prior, key)
		if !ok || v != want {
			t.Errorf("%s: got %d ok=%v, want %d", key, v, ok, want)
		}
	}
}

func TestMissingSnapshotIsNotFound(t *testing.T) {
	l := Local{Dir: t.TempDir()}
	_, err := l.Get(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// A crash mid-write must not leave a truncated snapshot where a complete one
// was — the file is written to a temporary path and renamed.
func TestWriteIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	l := Local{Dir: dir}
	ctx := context.Background()
	if err := l.Put(ctx, snap("p1", 3)); err != nil {
		t.Fatal(err)
	}
	if err := l.Put(ctx, snap("p1", 5)); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "p1.json" {
			t.Errorf("left a stray file behind: %s", e.Name())
		}
	}
	// The record names what is running in the account; it is not world-readable.
	info, err := os.Stat(filepath.Join(dir, "p1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("snapshot is %v, should not be readable by others", perm)
	}
}

// PutVerified exists to catch a store that accepts a write and loses it. That
// is the one failure the tool cannot survive, and it is cheap to rule out.
func TestPutVerifiedCatchesALossyStore(t *testing.T) {
	ctx := context.Background()
	if err := PutVerified(ctx, Local{Dir: t.TempDir()}, snap("p1", 2)); err != nil {
		t.Errorf("a working store should verify: %v", err)
	}
	if err := PutVerified(ctx, lossy{}, snap("p1", 2)); err == nil {
		t.Error("a store that drops entries must fail verification")
	}
	if err := PutVerified(ctx, unreadable{}, snap("p1", 2)); err == nil {
		t.Error("a store that cannot read back must fail verification")
	}
}

type lossy struct{}

func (lossy) Put(context.Context, model.Snapshot) error { return nil }
func (lossy) Get(_ context.Context, id string) (model.Snapshot, error) {
	return model.Snapshot{PlanID: id}, nil // no entries
}
func (lossy) List(context.Context) ([]model.Snapshot, error) { return nil, nil }
func (lossy) Describe() string                               { return "lossy" }

type unreadable struct{}

func (unreadable) Put(context.Context, model.Snapshot) error { return nil }
func (unreadable) Get(context.Context, string) (model.Snapshot, error) {
	return model.Snapshot{}, errors.New("access denied")
}
func (unreadable) List(context.Context) ([]model.Snapshot, error) { return nil, nil }
func (unreadable) Describe() string                               { return "unreadable" }

// Multi must require every store to accept the write, or the durable copy and
// the local copy can disagree about what was stopped.
func TestMultiRequiresEveryStoreToAcceptTheWrite(t *testing.T) {
	m := Multi{Stores: []Store{Local{Dir: t.TempDir()}, failing{}}}
	if err := m.Put(context.Background(), snap("p1", 1)); err == nil {
		t.Error("one failing store must fail the write")
	}
}

// ...but a read succeeds from whichever store still has it, because the reason
// you are restoring may be that the machine holding the local copy is gone.
func TestMultiReadsFromWhicheverStoreHasIt(t *testing.T) {
	local := Local{Dir: t.TempDir()}
	if err := local.Put(context.Background(), snap("p1", 1)); err != nil {
		t.Fatal(err)
	}
	m := Multi{Stores: []Store{failing{}, local}}
	got, err := m.Get(context.Background(), "p1")
	if err != nil {
		t.Fatalf("should have fallen through to the working store: %v", err)
	}
	if got.PlanID != "p1" {
		t.Errorf("got %q", got.PlanID)
	}
}

type failing struct{}

func (failing) Put(context.Context, model.Snapshot) error { return errors.New("nope") }
func (failing) Get(context.Context, string) (model.Snapshot, error) {
	return model.Snapshot{}, errors.New("nope")
}
func (failing) List(context.Context) ([]model.Snapshot, error) { return nil, errors.New("nope") }
func (failing) Describe() string                               { return "failing" }

func TestParseURI(t *testing.T) {
	cases := []struct {
		in             string
		bucket, prefix string
		ok             bool
	}{
		{"s3://b/p", "b", "p", true},
		{"s3://b/p/q/", "b", "p/q", true},
		{"s3://b", "b", "", true},
		{"/local/path", "", "", false},
		{"https://example.com", "", "", false},
	}
	for _, c := range cases {
		b, p, ok := ParseURI(c.in)
		if b != c.bucket || p != c.prefix || ok != c.ok {
			t.Errorf("%q: got (%q,%q,%v), want (%q,%q,%v)", c.in, b, p, ok, c.bucket, c.prefix, c.ok)
		}
	}
}

// The snapshot has to carry the prior state of every action, or the restore
// has nothing to work from.
func TestFromPlanCarriesEveryActionsPriorState(t *testing.T) {
	p := model.Plan{
		ID: "p1", Account: "1234", CreatedAt: time.Unix(1700000000, 0).UTC(),
		Actions: []model.Action{
			{Resource: model.Resource{ID: "asg-1", Kind: model.KindASG, Region: "eu-west-1",
				Prior: map[string]any{"desired_capacity": 4}}, Phase: model.PhaseCompute},
			{Resource: model.Resource{ID: "l-1", Kind: model.KindALBListener, Region: "eu-west-1",
				Prior: map[string]any{"default_actions": []any{"x"}}}, Phase: model.PhaseIngress},
		},
	}
	s := From(p)
	if len(s.Entries) != 2 {
		t.Fatalf("got %d entries", len(s.Entries))
	}
	for _, e := range s.Entries {
		if len(e.Prior) == 0 {
			t.Errorf("%s carries no prior state; it could not be restored", e.ID)
		}
	}
}
