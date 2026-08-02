package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

// --- doubles -----------------------------------------------------------------

type fakeExec struct {
	applied  []string
	restored []string
	failOn   map[string]error
}

func (f *fakeExec) Apply(_ context.Context, a model.Action) error {
	if err, ok := f.failOn[a.Resource.ID]; ok {
		return err
	}
	f.applied = append(f.applied, a.Resource.ID)
	return nil
}

func (f *fakeExec) Restore(_ context.Context, e model.Entry) error {
	if err, ok := f.failOn[e.ID]; ok {
		return err
	}
	f.restored = append(f.restored, e.ID)
	return nil
}

type memStore struct {
	snaps    map[string]model.Snapshot
	puts     int
	failPut  error
	failGet  error
	loseData bool // accepts writes and silently drops entries
}

func newMem() *memStore { return &memStore{snaps: map[string]model.Snapshot{}} }

func (m *memStore) Put(_ context.Context, s model.Snapshot) error {
	m.puts++
	if m.failPut != nil {
		return m.failPut
	}
	if m.loseData {
		s.Entries = nil
	}
	// Store a copy; a real store round-trips through JSON and does not alias.
	cp := s
	cp.Entries = append([]model.Entry(nil), s.Entries...)
	m.snaps[s.PlanID] = cp
	return nil
}

func (m *memStore) Get(_ context.Context, id string) (model.Snapshot, error) {
	if m.failGet != nil {
		return model.Snapshot{}, m.failGet
	}
	s, ok := m.snaps[id]
	if !ok {
		return model.Snapshot{}, state.ErrNotFound
	}
	return s, nil
}

func (m *memStore) List(context.Context) ([]model.Snapshot, error) { return nil, nil }
func (m *memStore) Describe() string                               { return "memory" }

func action(kind model.Kind, id string, phase model.Phase) model.Action {
	return model.Action{
		Resource: model.Resource{ID: id, Kind: kind, Name: id, Prior: map[string]any{"desired_count": 3}},
		Phase:    phase, Op: "stop",
	}
}

func testPlan(actions ...model.Action) model.Plan {
	return model.Plan{
		ID: "plan-1", CreatedAt: time.Unix(1700000000, 0).UTC(),
		Account: "1234", Actions: actions,
	}
}

func opts() Options {
	return Options{
		ContinueOnError: true,
		Now:             func() time.Time { return time.Unix(1700000000, 0).UTC() },
		Log:             audit.Discard(),
	}
}

// --- the invariant -----------------------------------------------------------

// Nothing is touched until the restore record is safely written. An account
// that is still expensive is a problem; an account that is stopped with no
// record of how to start it is an outage of unknown length.
func TestNothingIsTouchedIfTheSnapshotCannotBeWritten(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	st.failPut = errors.New("s3: access denied")

	_, err := Fire(context.Background(), testPlan(action(model.KindLambda, "fn-1", model.PhaseCompute)), st, ex, opts())
	if err == nil {
		t.Fatal("firing must fail when the snapshot cannot be written")
	}
	if !strings.Contains(err.Error(), "refusing to fire") {
		t.Errorf("the error should say the fire was abandoned, got %q", err)
	}
	if len(ex.applied) != 0 {
		t.Errorf("nothing may be changed: %v was applied", ex.applied)
	}
}

// A store that accepts a write and loses it is the one failure this cannot
// survive, so the write is read back and compared.
func TestSilentlyLossyStoreIsCaughtBeforeAnythingChanges(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	st.loseData = true

	_, err := Fire(context.Background(), testPlan(action(model.KindASG, "asg-1", model.PhaseCompute)), st, ex, opts())
	if err == nil {
		t.Fatal("a store that drops entries must be detected")
	}
	if !strings.Contains(err.Error(), "refusing to fire") {
		t.Errorf("got %q", err)
	}
	if len(ex.applied) != 0 {
		t.Error("nothing may be changed when the record is unreliable")
	}
}

// An interrupted fire must leave a record of exactly what was changed, so the
// snapshot is updated after every action rather than once at the end.
func TestSnapshotIsUpdatedAfterEveryChange(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	p := testPlan(
		action(model.KindLambda, "fn-1", model.PhaseCompute),
		action(model.KindLambda, "fn-2", model.PhaseCompute),
		action(model.KindASG, "asg-1", model.PhaseCompute),
	)
	res, err := Fire(context.Background(), p, st, ex, opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed != 3 {
		t.Fatalf("changed %d, want 3", res.Changed)
	}
	// One verified write up front, then one per action.
	if st.puts < 4 {
		t.Errorf("snapshot written %d times, expected at least 4 (one before, one per change)", st.puts)
	}
	saved, _ := st.Get(context.Background(), "plan-1")
	if len(saved.Changed()) != 3 {
		t.Errorf("the stored record lists %d changes, want 3", len(saved.Changed()))
	}
}

// A failure partway through leaves the successes recorded, so restore knows
// what to undo and — just as important — what not to touch.
func TestPartialFireRecordsOnlyWhatActuallyChanged(t *testing.T) {
	ex := &fakeExec{failOn: map[string]error{"fn-2": errors.New("throttled")}}
	st := newMem()
	p := testPlan(
		action(model.KindLambda, "fn-1", model.PhaseCompute),
		action(model.KindLambda, "fn-2", model.PhaseCompute),
		action(model.KindLambda, "fn-3", model.PhaseCompute),
	)
	res, err := Fire(context.Background(), p, st, ex, opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed != 2 || res.Failed != 1 {
		t.Fatalf("changed=%d failed=%d, want 2 and 1", res.Changed, res.Failed)
	}
	saved, _ := st.Get(context.Background(), "plan-1")
	if got := len(saved.Changed()); got != 2 {
		t.Errorf("restore would act on %d resources, want 2", got)
	}
	for _, e := range saved.Entries {
		if e.ID == "fn-2" && e.Result != model.ResultFailed {
			t.Errorf("fn-2 recorded as %q, want failed", e.Result)
		}
	}
}

// A dry run must not write a snapshot. A record of a fire that never happened
// is exactly the thing someone would later restore from, putting back state
// that was never changed.
func TestDryRunWritesNothingAndChangesNothing(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	o := opts()
	o.DryRun = true

	res, err := Fire(context.Background(), testPlan(action(model.KindASG, "asg-1", model.PhaseCompute)), st, ex, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.applied) != 0 {
		t.Error("a dry run must not call the API")
	}
	if st.puts != 0 {
		t.Errorf("a dry run must not write a snapshot, wrote %d times", st.puts)
	}
	if res.Skipped != 1 {
		t.Errorf("skipped %d, want 1", res.Skipped)
	}
}

// Restore runs backwards: compute returns before traffic is let back in.
func TestRestoreRunsInReversePhaseOrder(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	p := testPlan(
		action(model.KindALBListener, "listener-1", model.PhaseIngress),
		action(model.KindASG, "asg-1", model.PhaseCompute),
		action(model.KindRDSInstance, "db-1", model.PhaseData),
	)
	if _, err := Fire(context.Background(), p, st, ex, opts()); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.Get(context.Background(), "plan-1")

	ex2 := &fakeExec{}
	if _, err := Restore(context.Background(), saved, st, ex2, opts()); err != nil {
		t.Fatal(err)
	}
	want := []string{"db-1", "asg-1", "listener-1"}
	if len(ex2.restored) != 3 {
		t.Fatalf("restored %v", ex2.restored)
	}
	for i := range want {
		if ex2.restored[i] != want[i] {
			t.Errorf("restore order %v, want %v — traffic must not arrive before compute is back",
				ex2.restored, want)
		}
	}
}

// Restoring twice must not run the same change twice.
func TestRestoreIsIdempotent(t *testing.T) {
	ex := &fakeExec{}
	st := newMem()
	if _, err := Fire(context.Background(), testPlan(action(model.KindASG, "asg-1", model.PhaseCompute)), st, ex, opts()); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.Get(context.Background(), "plan-1")

	ex2 := &fakeExec{}
	if _, err := Restore(context.Background(), saved, st, ex2, opts()); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Get(context.Background(), "plan-1")
	if _, err := Restore(context.Background(), after, st, ex2, opts()); err == nil {
		t.Error("a second restore should report there is nothing left to undo")
	}
	if len(ex2.restored) != 1 {
		t.Errorf("the resource was restored %d times, want 1", len(ex2.restored))
	}
}

// The seven-day fuse. Nobody is watching a week later, so the tool has to be.
func TestDatabaseDeadlinesAreReported(t *testing.T) {
	fired := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	snap := model.Snapshot{
		PlanID: "p", Fired: &fired,
		Entries: []model.Entry{
			{Kind: model.KindRDSInstance, ID: "db-1", Result: model.ResultChanged},
			{Kind: model.KindLambda, ID: "fn-1", Result: model.ResultChanged},
		},
	}
	msgs := Deadlines(snap, fired.Add(48*time.Hour))
	if len(msgs) != 1 {
		t.Fatalf("expected one deadline (the database only), got %v", msgs)
	}
	if !strings.Contains(msgs[0], "db-1") || !strings.Contains(msgs[0], "5.0 days") {
		t.Errorf("got %q, want the database and the time remaining", msgs[0])
	}

	// Past the deadline the message has to change: it is billing again.
	late := Deadlines(snap, fired.Add(8*24*time.Hour))
	if !strings.Contains(late[0], "already restarted") {
		t.Errorf("past the fuse the message must say so, got %q", late[0])
	}
}

// An empty plan is a mistake, not a no-op to be executed quietly.
func TestEmptyPlanIsRejected(t *testing.T) {
	if _, err := Fire(context.Background(), testPlan(), newMem(), &fakeExec{}, opts()); err == nil {
		t.Error("firing an empty plan should be an error")
	}
}

// If the record cannot be updated after a change has been made, stop — the
// alternative is changing more things with no way to undo any of them.
func TestFireStopsWhenTheRecordCanNoLongerBeUpdated(t *testing.T) {
	ex := &fakeExec{}
	st := &brokenAfterN{memStore: newMem(), failAfter: 2}
	p := testPlan(
		action(model.KindLambda, "fn-1", model.PhaseCompute),
		action(model.KindLambda, "fn-2", model.PhaseCompute),
		action(model.KindLambda, "fn-3", model.PhaseCompute),
	)
	_, err := Fire(context.Background(), p, st, ex, opts())
	if err == nil {
		t.Fatal("losing the record mid-fire must stop the fire")
	}
	if !strings.Contains(err.Error(), "restore") {
		t.Errorf("the error must tell the operator what to do next, got %q", err)
	}
	if len(ex.applied) > 2 {
		t.Errorf("kept going after the record was lost: applied %v", ex.applied)
	}
}

type brokenAfterN struct {
	*memStore
	failAfter int
}

func (b *brokenAfterN) Put(ctx context.Context, s model.Snapshot) error {
	if b.memStore.puts >= b.failAfter {
		b.memStore.puts++
		return errors.New("s3: connection reset")
	}
	return b.memStore.Put(ctx, s)
}
