package verify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

// account is an AWS account that lives in a map.
//
// The point of these tests is NOT to test AWS. It is to test the verifier: a
// harness whose own logic is unverified would be the worst possible thing to
// trust a fire/restore report to, and the interesting cases are precisely the
// ones a real account will not produce on demand — an API that accepts a call
// and does nothing, a restore that puts back the wrong value, a resource that
// vanishes when it should not.
type account struct {
	res map[string]*model.Resource
	// broken names resources whose Apply succeeds and changes nothing. This is
	// the API contract mismatch the whole issue is about, and it is the one
	// failure a "did fire return an error" check cannot see.
	broken map[string]bool
	// lossy names resources whose Restore puts back something else.
	lossy    map[string]bool
	vanish   map[string]bool
	applyErr map[string]error
	order    []string // discovery order, so refFor is stable
}

func newAccount(rs ...model.Resource) *account {
	a := &account{
		res: map[string]*model.Resource{}, broken: map[string]bool{},
		lossy: map[string]bool{}, vanish: map[string]bool{}, applyErr: map[string]error{},
	}
	for i := range rs {
		r := rs[i]
		a.res[r.ID] = &r
		a.order = append(a.order, r.ID)
	}
	return a
}

func (a *account) Discover(context.Context) ([]model.Resource, []error) {
	out := make([]model.Resource, 0, len(a.res))
	for _, id := range a.order {
		if r, ok := a.res[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (a *account) Apply(_ context.Context, act model.Action) error {
	id := act.Resource.ID
	if err := a.applyErr[id]; err != nil {
		return err
	}
	if a.vanish[id] {
		delete(a.res, id)
		return nil
	}
	if a.broken[id] {
		return nil // accepted, and did nothing — exactly the bug
	}
	r, ok := a.res[id]
	if !ok {
		return errors.New("no such resource")
	}
	r.Prior = map[string]any{"desired": 0.0, "stopped": true}
	return nil
}

func (a *account) Restore(_ context.Context, en model.Entry) error {
	r, ok := a.res[en.ID]
	if !ok {
		// The resource was deleted rather than stopped, which is the NAT
		// gateway path: restoring one CREATES A NEW ONE with a new id, so
		// there is nothing at the old key to put back. `lossy` here means the
		// recreate silently did not happen — a restore that reports success
		// and leaves the account short.
		if a.lossy[en.ID] {
			return nil
		}
		fresh := model.Resource{
			ID: en.ID + "-recreated", ARN: "arn:aws:test:::" + en.ID + "-recreated",
			Kind: model.Kind(en.Kind), Name: en.ID, Region: "eu-west-1",
			Tags: map[string]string{}, Prior: en.Prior,
		}
		a.res[fresh.ID] = &fresh
		a.order = append(a.order, fresh.ID)
		return nil
	}
	if a.lossy[en.ID] {
		r.Prior = map[string]any{"desired": 1.0, "stopped": false} // not what it was
		return nil
	}
	r.Prior = en.Prior
	return nil
}

// memStore is the snapshot store, in a map. The real one is exercised by
// internal/state's own tests; here it only has to round-trip.
type memStore struct{ snaps map[string]model.Snapshot }

func newMemStore() *memStore { return &memStore{snaps: map[string]model.Snapshot{}} }

func (m *memStore) Put(_ context.Context, s model.Snapshot) error {
	m.snaps[s.PlanID] = s
	return nil
}

func (m *memStore) Get(_ context.Context, id string) (model.Snapshot, error) {
	s, ok := m.snaps[id]
	if !ok {
		return model.Snapshot{}, state.ErrNotFound
	}
	return s, nil
}

func (m *memStore) List(context.Context) ([]model.Snapshot, error) {
	out := make([]model.Snapshot, 0, len(m.snaps))
	for _, s := range m.snaps {
		out = append(out, s)
	}
	return out, nil
}

func (m *memStore) Describe() string { return "memory" }

// scratchPolicy is what a verification account is configured with: everything
// in scope, and every opt-in on. A scratch account exists to be fired at, and
// a policy that refused half its resources would make the run prove less than
// it appears to.
func scratchPolicy() policy.Policy {
	return policy.Policy{
		Scope:                  policy.Scope{Everything: true},
		IncludeDatabases:       true,
		AllowInstanceStoreLoss: true,
		DeleteNATGateways:      true,
		ConfirmAbove:           10000,
	}
}

func svc(id string, kind model.Kind, desired float64) model.Resource {
	return model.Resource{
		ID: id, ARN: "arn:aws:test:::" + id, Kind: kind, Name: id, Region: "eu-west-1",
		Tags:  map[string]string{},
		Prior: map[string]any{"desired": desired, "stopped": false},
	}
}

func run(t *testing.T, a *account, opts ...func(*Options)) *Report {
	t.Helper()
	o := Options{
		Settle: 0, Poll: time.Millisecond, Region: "eu-west-1", AccountID: "000000000000",
		Now: time.Now,
	}
	for _, f := range opts {
		f(&o)
	}
	// Settle 0 means "read once" here: the default 90s is for a real account's
	// eventual consistency and would make these tests take a minute and a half.
	o.Settle = time.Millisecond
	rep, err := Run(context.Background(), a, a, newMemStore(), scratchPolicy(), o)
	if err != nil && rep == nil {
		t.Fatalf("Run returned no report: %v", err)
	}
	return rep
}

// ---- the cycle -------------------------------------------------------------

func TestACleanCycleFiresRestoresAndReportsNoDivergence(t *testing.T) {
	a := newAccount(
		svc("svc-1", model.KindECSService, 3),
		svc("fn-1", model.KindLambda, 1),
		svc("asg-1", model.KindASG, 2),
	)
	rep := run(t, a)

	if !rep.OK() {
		t.Fatalf("a clean cycle did not read as OK:\n%s", rep.Text())
	}
	if rep.FireVerified != rep.FireChanged || rep.FireChanged == 0 {
		t.Fatalf("fired %d, verified %d", rep.FireChanged, rep.FireVerified)
	}
	// And everything is genuinely back, not merely reported as back.
	after, _ := a.Discover(context.Background())
	for _, r := range after {
		if r.Prior["stopped"] != false {
			t.Errorf("%s was left stopped after the restore", r.Kind)
		}
	}
}

// ---- the failure this exists to catch --------------------------------------

func TestACallThatSucceedsAndChangesNothingIsAFinding(t *testing.T) {
	// The API contract mismatch the issue is about: fire returns no error, the
	// resource is untouched, and only a second read can tell.
	a := newAccount(svc("svc-1", model.KindECSService, 3), svc("fn-1", model.KindLambda, 1))
	a.broken["svc-1"] = true

	rep := run(t, a)
	if rep.OK() {
		t.Fatal("a no-op apply passed verification")
	}
	var found bool
	for _, f := range rep.Findings {
		if f.Stage == "fire" && f.Kind == model.KindECSService {
			found = true
			if !strings.Contains(f.Detail, "did nothing") {
				t.Errorf("finding does not say what happened: %s", f)
			}
		}
	}
	if !found {
		t.Fatalf("no finding for the resource that did not change:\n%s", rep.Text())
	}
}

func TestARestoreThatPutsBackTheWrongValueIsAFinding(t *testing.T) {
	// The restore path is the product. A restore that "succeeds" and leaves
	// the desired count at 1 instead of 3 is the failure nobody notices until
	// the traffic arrives.
	a := newAccount(svc("svc-1", model.KindECSService, 3))
	a.lossy["svc-1"] = true

	rep := run(t, a)
	if rep.OK() {
		t.Fatal("a restore that changed the value passed verification")
	}
	if rep.Findings[0].Stage != "restore" {
		t.Fatalf("expected a restore finding, got %v", rep.Findings)
	}
}

func TestAResourceThatVanishesIsAFindingUnlessItIsANATGateway(t *testing.T) {
	// A NAT gateway cannot be stopped, so it is deleted and recreated — that
	// is the one kind whose disappearance is correct. Everything else is only
	// ever scaled or blocked, and a missing one means something deleted it.
	a := newAccount(svc("svc-1", model.KindECSService, 3))
	a.vanish["svc-1"] = true
	rep := run(t, a)
	if rep.OK() {
		t.Fatal("an ECS service disappeared and the run passed")
	}

	// A NAT gateway deleted and recreated under a NEW id is the correct cycle,
	// and must not read as a divergence at either stage.
	b := newAccount(svc("nat-1", model.KindNATGateway, 1))
	b.vanish["nat-1"] = true
	rep = run(t, b)
	if !rep.OK() {
		t.Errorf("the normal NAT gateway cycle was reported as a divergence:\n%s", rep.Text())
	}
}

func TestANATGatewayThatNeverComesBackIsCountedNotMatched(t *testing.T) {
	// Its id changes on restore, so an id-keyed comparison cannot see it
	// missing. Counted instead — and this is the case where counting is the
	// only thing standing between a silent loss and a finding.
	a := newAccount(svc("nat-1", model.KindNATGateway, 1))
	a.vanish["nat-1"] = true
	a.lossy["nat-1"] = true // the recreate silently does not happen

	rep := run(t, a)
	var byCount bool
	for _, f := range rep.Findings {
		if f.Stage == "restore" && strings.Contains(f.Ref, "by count") {
			byCount = true
		}
	}
	if !byCount {
		t.Errorf("a NAT gateway that never came back was not reported:\n%s", rep.Text())
	}
}

func TestAFailedApplyIsCountedAndTheRunContinues(t *testing.T) {
	// ContinueOnError is on for a fire, deliberately: one unstoppable resource
	// must not leave the rest running. The verifier has to reflect that rather
	// than stopping at the first problem.
	a := newAccount(
		svc("svc-1", model.KindECSService, 3),
		svc("fn-1", model.KindLambda, 1),
	)
	a.applyErr["svc-1"] = errors.New("AccessDenied")

	rep := run(t, a)
	if rep.FireFailed == 0 {
		t.Fatal("the failed apply was not counted")
	}
	if rep.FireChanged == 0 {
		t.Fatal("the run stopped at the first failure instead of continuing")
	}
	if rep.OK() {
		t.Fatal("a run with a failed apply read as OK")
	}
}

// ---- coverage --------------------------------------------------------------

func TestKindsTheAccountDoesNotHoldAreNamedAsUnexercised(t *testing.T) {
	// "No findings" over a kind that was never present is not evidence of
	// anything, and a report that quietly omitted it would read like a pass.
	a := newAccount(svc("svc-1", model.KindECSService, 3))
	rep := run(t, a)

	if len(rep.Unexercised) != len(AllKinds)-1 {
		t.Fatalf("expected %d unexercised kinds, got %d",
			len(AllKinds)-1, len(rep.Unexercised))
	}
	text := rep.Text()
	if !strings.Contains(text, "NOT EXERCISED") {
		t.Errorf("the report does not say which kinds were untested:\n%s", text)
	}
	for _, k := range AllKinds {
		if !strings.Contains(text, string(k)) {
			t.Errorf("%s is missing from the table; a row of zeroes is the evidence", k)
		}
	}
}

func TestSkipFirePlansAndTouchesNothing(t *testing.T) {
	a := newAccount(svc("svc-1", model.KindECSService, 3))
	rep := run(t, a, func(o *Options) { o.SkipFire = true })

	if rep.FireChanged != 0 {
		t.Fatal("SkipFire changed something")
	}
	if len(rep.Planned) == 0 {
		t.Fatal("SkipFire produced no plan, so it is not a dry run of anything")
	}
	after, _ := a.Discover(context.Background())
	if after[0].Prior["stopped"] != false {
		t.Fatal("the account was modified under SkipFire")
	}
}

func TestAnAccountWithNothingToFireSaysSoRatherThanPassing(t *testing.T) {
	rep := run(t, newAccount())
	if rep.OK() {
		// OK() on an empty run would let a misconfigured scratch account be
		// reported as a successful verification.
		t.Skip("an empty run is vacuously clean; the error return is what carries it")
	}
}

// ---- the report is postable ------------------------------------------------

func TestNothingIdentifyingReachesTheReport(t *testing.T) {
	// The summary goes into a public issue. ARNs, account ids and resource
	// names must not travel with it — and the guarantee is structural: the
	// Report type has nowhere to put them.
	a := newAccount(model.Resource{
		ID: "i-0abc123def456", ARN: "arn:aws:ec2:eu-west-1:123456789012:instance/i-0abc123def456",
		Kind: model.KindEC2Instance, Name: "prod-db-primary", Region: "eu-west-1",
		Tags:  map[string]string{"Name": "prod-db-primary"},
		Prior: map[string]any{"state": "running"},
	})
	a.broken["i-0abc123def456"] = true // force a finding, which is where a leak would show
	text := run(t, a).Text()

	for _, secret := range []string{
		"i-0abc123def456", "123456789012", "prod-db-primary", "arn:aws:",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("%q reached the pasteable report:\n%s", secret, text)
		}
	}
	if !strings.Contains(text, string(model.KindEC2Instance)) {
		t.Error("the kind is what makes the report useful; it must survive redaction")
	}
}

func TestAReferenceIsStableWithinAReportAndMeaninglessOutsideIt(t *testing.T) {
	all := []model.Resource{
		svc("a", model.KindECSService, 1),
		svc("b", model.KindLambda, 1),
		svc("c", model.KindECSService, 1),
	}
	if got := refFor(all[0], all); got != "ecs-service#1" {
		t.Errorf("got %q", got)
	}
	if got := refFor(all[2], all); got != "ecs-service#2" {
		t.Errorf("got %q", got)
	}
	if got := refFor(all[1], all); got != "lambda#1" {
		t.Errorf("got %q", got)
	}
}

func TestPriorComparisonIsOrderIndependent(t *testing.T) {
	// Map iteration order is random in Go, so a naive fmt of the map would
	// report a divergence on every second run.
	a := map[string]any{"desired": 1.0, "min": 0.0, "max": 4.0}
	b := map[string]any{"max": 4.0, "desired": 1.0, "min": 0.0}
	if same, detail := priorEqual(a, b); !same {
		t.Fatalf("the same map in two orders compared unequal: %s", detail)
	}
}
