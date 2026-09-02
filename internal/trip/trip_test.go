package trip

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// fakeStore is a state store in memory, with an optional List failure.
type fakeStore struct {
	snaps   []model.Snapshot
	listErr error
}

func (f *fakeStore) Put(context.Context, model.Snapshot) error { return nil }
func (f *fakeStore) Get(context.Context, string) (model.Snapshot, error) {
	return model.Snapshot{}, errors.New("not used")
}
func (f *fakeStore) List(context.Context) ([]model.Snapshot, error) {
	return f.snaps, f.listErr
}
func (f *fakeStore) Describe() string { return "fake" }

func at(t time.Time) *time.Time { return &t }

func TestARepeatedNotificationInsideTheCooldownDoesNotDoubleFire(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	st := &fakeStore{snaps: []model.Snapshot{
		{PlanID: "20260902-114500", Fired: at(now.Add(-15 * time.Minute))},
	}}

	// The second delivery would otherwise discover an account that is already
	// stopped and write a snapshot whose "prior" state is the stopped one —
	// restoring which would put the account back exactly as it is now: down.
	since, recent := FiredRecently(context.Background(), st, now)
	if !recent {
		t.Fatal("a fire 15 minutes ago is inside the cooldown")
	}
	if since != 15*time.Minute {
		t.Errorf("since = %v, want 15m", since)
	}
}

func TestAFireOlderThanTheCooldownDoesNotBlockANewOne(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	st := &fakeStore{snaps: []model.Snapshot{
		{PlanID: "old", Fired: at(now.Add(-2 * time.Hour))},
	}}
	if _, recent := FiredRecently(context.Background(), st, now); recent {
		t.Error("a genuine second incident two hours later must not be blocked")
	}
}

func TestARestoredSnapshotDoesNotBlockANewFire(t *testing.T) {
	// Fired and then put back: the account is running again, so a new spend
	// spike is a real event and the killswitch has to work.
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	st := &fakeStore{snaps: []model.Snapshot{{
		PlanID:   "restored",
		Fired:    at(now.Add(-10 * time.Minute)),
		Restored: at(now.Add(-5 * time.Minute)),
	}}}
	if _, recent := FiredRecently(context.Background(), st, now); recent {
		t.Error("a snapshot that has been restored must not hold the cooldown open")
	}
}

func TestAPlannedButNeverFiredSnapshotDoesNotBlock(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	st := &fakeStore{snaps: []model.Snapshot{{PlanID: "dry", Fired: nil}}}
	if _, recent := FiredRecently(context.Background(), st, now); recent {
		t.Error("a plan that never fired is not a fire")
	}
}

func TestUnreadableStateDoesNotRefuseToFire(t *testing.T) {
	// Unreadable state is not permission to double-fire, but it is also not a
	// reason to refuse during an incident. A killswitch that will not press is
	// worse than one that presses twice — the second fire is a near no-op, and
	// the first snapshot still holds the restore path.
	st := &fakeStore{listErr: errors.New("s3: access denied")}
	if _, recent := FiredRecently(context.Background(), st, time.Now()); recent {
		t.Error("an unreadable store must not block the trip")
	}
}

func TestAFireStampedInTheFutureIsIgnored(t *testing.T) {
	// Clock skew between whoever wrote the snapshot and whoever is reading it
	// would otherwise hold the cooldown open indefinitely.
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	st := &fakeStore{snaps: []model.Snapshot{
		{PlanID: "skewed", Fired: at(now.Add(30 * time.Minute))},
	}}
	if _, recent := FiredRecently(context.Background(), st, now); recent {
		t.Error("a future-stamped fire must not block")
	}
}

func TestPlanIDIsSortableAndSecondPrecise(t *testing.T) {
	a := PlanID(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	b := PlanID(time.Date(2026, 9, 2, 12, 0, 1, 0, time.UTC))
	if a >= b {
		t.Errorf("%q should sort before %q", a, b)
	}
	if a != "20260902-120000" {
		t.Errorf("plan id = %q", a)
	}
}
