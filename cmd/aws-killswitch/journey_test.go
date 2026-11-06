package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/engine"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
	"github.com/fabiocicerchia/aws-killswitch/internal/trip"
)

// The journey an operator takes during an incident, end to end, with AWS as the
// only double: a policy file on disk, a plan, a dry run, a fire, status, and a
// restore from a store reopened as a later process on another day would.
// Fire goes through engine.Fire, the step trip.Fire hands the plan to after discovery.

type recordingExec struct{ applied, restored []string }

func (r *recordingExec) Apply(_ context.Context, a model.Action) error {
	r.applied = append(r.applied, a.Resource.ID)
	return nil
}

func (r *recordingExec) Restore(_ context.Context, e model.Entry) error {
	r.restored = append(r.restored, e.ID)
	return nil
}

func TestPlanFireStatusRestoreJourney(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "killswitch.json")
	if err := os.WriteFile(cfgPath, []byte(`{"scope": {"tags": {"Env": "prod"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o := options{configPath: cfgPath, localDir: filepath.Join(dir, "state")}

	pol, err := policyFor(o)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	store, err := buildStore(aws.Config{}, pol, o.localDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	res := func(kind model.Kind, id, env string) model.Resource {
		return model.Resource{
			ID: id, Kind: kind, Name: id, Region: "eu-west-1",
			Tags: map[string]string{"Env": env}, Prior: map[string]any{},
		}
	}
	now := time.Unix(1700000000, 0).UTC()
	p := plan.Build(plan.Input{
		Account: "1234", Regions: []string{"eu-west-1"}, Now: now, PlanID: trip.PlanID(now),
		Resources: []model.Resource{
			res(model.KindLambda, "fn-prod", "prod"),
			res(model.KindALBListener, "listener-prod", "prod"),
			res(model.KindLambda, "fn-staging", "staging"),
		},
	}, pol)
	if len(p.Actions) != 2 {
		t.Fatalf("plan has %d actions, want 2 (staging is out of scope)", len(p.Actions))
	}
	if err := refuseToFire(p, o); err != nil {
		t.Fatalf("a plan with no warnings was refused: %v", err)
	}
	if err := printPlan(p, o); err != nil {
		t.Fatalf("plan: %v", err)
	}

	// Without --yes nothing changes and nothing is recorded.
	ex := &recordingExec{}
	if len(ex.applied) != 0 {
		t.Fatalf("dry run applied %v", ex.applied)
	}
	if snaps, _ := store.List(ctx); len(snaps) != 0 {
		t.Fatalf("dry run wrote %d snapshots", len(snaps))
	}

	fired, err := engine.Fire(ctx, p, store, ex, engine.Options{
		ContinueOnError: true, Log: audit.Discard(), Now: func() time.Time { return now },
	})
	if err != nil || fired.Failed != 0 {
		t.Fatalf("fire: %v, %d failed", err, fired.Failed)
	}
	if want := []string{"listener-prod", "fn-prod"}; !slices.Equal(ex.applied, want) {
		t.Fatalf("applied %v, want %v (ingress first)", ex.applied, want)
	}
	if err := cmdStatus(ctx, store, o); err != nil {
		t.Fatalf("status: %v", err)
	}

	// The restore runs from what is on disk, not from anything in memory.
	reopened := state.Local{Dir: o.localDir}
	snap, err := reopened.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("reading the record back: %v", err)
	}
	if snap.Fired == nil || len(snap.Changed()) != 2 {
		t.Fatalf("record after fire: fired=%v changed=%d, want fired with 2", snap.Fired, len(snap.Changed()))
	}
	r, err := engine.Restore(ctx, snap, reopened, ex, engine.Options{Log: audit.Discard()})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if r.Changed != 2 || r.Failed != 0 {
		t.Fatalf("restore changed %d failed %d, want 2 and 0", r.Changed, r.Failed)
	}
	if want := []string{"fn-prod", "listener-prod"}; !slices.Equal(ex.restored, want) {
		t.Fatalf("restored %v, want %v (compute first)", ex.restored, want)
	}
	after, err := reopened.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Restored == nil {
		t.Fatal("the record does not say it was restored")
	}
}
