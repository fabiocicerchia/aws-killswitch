package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
)

func res(kind model.Kind, id string, tags map[string]string) model.Resource {
	if tags == nil {
		tags = map[string]string{"Env": "prod"}
	}
	return model.Resource{
		ID: id, Kind: kind, Name: id, Region: "eu-west-1", Tags: tags,
		ARN:   "arn:aws:" + string(kind) + ":eu-west-1:1234:" + id,
		Prior: map[string]any{},
	}
}

func prodScope() policy.Policy {
	return policy.Policy{Scope: policy.Scope{Tags: map[string]string{"Env": "prod"}}}
}

func build(p policy.Policy, rs ...model.Resource) model.Plan {
	return Build(Input{
		Account: "1234", Regions: []string{"eu-west-1"},
		Resources: rs, Now: time.Unix(1700000000, 0).UTC(), PlanID: "test",
	}, p)
}

func kinds(p model.Plan) []model.Kind {
	var out []model.Kind
	for _, a := range p.Actions {
		out = append(out, a.Resource.Kind)
	}
	return out
}

func refusalFor(p model.Plan, id string) (string, bool) {
	for _, r := range p.Refusals {
		if r.Resource.ID == id {
			return r.Reason, true
		}
	}
	return "", false
}

// Ingress before compute. Draining the fleet while traffic still arrives means
// a target-tracking policy fights the teardown and health checks alarm.
func TestIngressIsCutBeforeComputeIsDrained(t *testing.T) {
	p := build(prodScope(),
		res(model.KindASG, "asg-1", nil),
		res(model.KindALBListener, "listener-1", nil),
		res(model.KindLambda, "fn-1", nil),
	)
	got := []model.Phase{}
	for _, a := range p.Actions {
		got = append(got, a.Phase)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(got))
	}
	if got[0] != model.PhaseIngress {
		t.Errorf("first action is %v, must be ingress", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("phases out of order: %v", got)
		}
	}
}

// An ASG replaces an instance stopped underneath it, so the group has to go to
// zero before loose instances are touched.
func TestASGIsScaledBeforeLooseInstancesAreStopped(t *testing.T) {
	p := build(prodScope(),
		res(model.KindEC2Instance, "i-1", nil),
		res(model.KindASG, "asg-1", nil),
	)
	if len(p.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(p.Actions))
	}
	if p.Actions[0].Resource.Kind != model.KindASG {
		t.Errorf("order is %v, the ASG must come first or it replaces the stopped instance", kinds(p))
	}
}

// The absolute rule. No tag, no flag, no scope makes stateful storage eligible.
func TestStatefulStorageIsNeverTouched(t *testing.T) {
	everything := policy.Policy{Scope: policy.Scope{Everything: true}}
	for _, arn := range []string{
		"arn:aws:s3:::my-bucket",
		"arn:aws:dynamodb:eu-west-1:1234:table/orders",
		"arn:aws:elasticfilesystem:eu-west-1:1234:file-system/fs-1",
		"arn:aws:backup:eu-west-1:1234:vault/default",
	} {
		r := model.Resource{ID: "x", ARN: arn, Kind: model.Kind("s3"), Tags: map[string]string{}}
		p := build(everything, r)
		if len(p.Actions) != 0 {
			t.Errorf("%s produced an action; it must never be touched", arn)
		}
	}
	// And the deny list itself.
	for _, s := range []string{"s3", "dynamodb", "ebs-volume", "snapshot", "kms", "iam"} {
		if !model.IsNeverTouch(s) {
			t.Errorf("%q should be in the never-touch set", s)
		}
	}
	if model.IsNeverTouch("ecs-service") || model.IsNeverTouch("asg") {
		t.Error("compute must not be in the never-touch set, or the tool does nothing")
	}
}

// The protect tag beats everything, including scope.everything.
func TestProtectTagWinsOverEveryScope(t *testing.T) {
	protected := map[string]string{"Env": "prod", policy.ProtectTag: "true"}
	for _, pol := range []policy.Policy{prodScope(), {Scope: policy.Scope{Everything: true}}} {
		p := build(pol, res(model.KindASG, "asg-1", protected))
		if len(p.Actions) != 0 {
			t.Error("a protected resource was scheduled for a change")
		}
		if why, ok := refusalFor(p, "asg-1"); !ok || !strings.Contains(why, policy.ProtectTag) {
			t.Errorf("refusal should name the protect tag, got %q", why)
		}
	}
}

// An empty or odd value still protects — guessing "no" is the wrong way to be
// wrong about a tag someone added deliberately.
func TestProtectTagOnlyIgnoredWhenExplicitlyFalse(t *testing.T) {
	cases := map[string]bool{
		"true": true, "yes": true, "": true, "1": true, "because reasons": true,
		"false": false, "0": false, "no": false, "off": false, "FALSE": false,
	}
	for value, wantProtected := range cases {
		tags := map[string]string{"Env": "prod", policy.ProtectTag: value}
		p := build(prodScope(), res(model.KindLambda, "fn-1", tags))
		protected := len(p.Actions) == 0
		if protected != wantProtected {
			t.Errorf("%s=%q: protected=%v, want %v", policy.ProtectTag, value, protected, wantProtected)
		}
	}
}

// Out-of-scope resources are listed with a reason rather than silently dropped.
// "Why is that still running" is the first question asked afterwards.
func TestOutOfScopeResourcesAreExplained(t *testing.T) {
	p := build(prodScope(),
		res(model.KindLambda, "fn-prod", map[string]string{"Env": "prod"}),
		res(model.KindLambda, "fn-staging", map[string]string{"Env": "staging"}),
		res(model.KindLambda, "fn-untagged", map[string]string{}),
	)
	if len(p.Actions) != 1 || p.Actions[0].Resource.ID != "fn-prod" {
		t.Fatalf("only the prod function should be in scope, got %d actions", len(p.Actions))
	}
	if why, _ := refusalFor(p, "fn-staging"); !strings.Contains(why, "staging") {
		t.Errorf("refusal should say what the tag actually was, got %q", why)
	}
	if why, _ := refusalFor(p, "fn-untagged"); !strings.Contains(why, "no Env tag") {
		t.Errorf("refusal should say the tag was missing, got %q", why)
	}
}

// Databases are out unless asked for, and even then they carry the seven-day
// warning — the trap that makes a kill switch silently un-kill itself.
func TestDatabasesAreOptInAndCarryTheSevenDayWarning(t *testing.T) {
	db := res(model.KindRDSInstance, "db-1", nil)
	db.Prior = map[string]any{"status": "available"}

	off := build(prodScope(), db)
	if len(off.Actions) != 0 {
		t.Error("databases must not be stopped by default")
	}
	if why, _ := refusalFor(off, "db-1"); !strings.Contains(why, "include_databases") {
		t.Errorf("refusal should name the flag, got %q", why)
	}

	pol := prodScope()
	pol.IncludeDatabases = true
	on := build(pol, db)
	if len(on.Actions) != 1 {
		t.Fatal("include_databases should schedule the database")
	}
	if on.Actions[0].Phase != model.PhaseData {
		t.Error("databases belong in the data phase, last")
	}
	if !strings.Contains(on.Actions[0].Warning, "7 days") {
		t.Errorf("the auto-restart trap must be warned about, got %q", on.Actions[0].Warning)
	}
	if len(on.AckRequired) == 0 {
		t.Error("that warning must require acknowledgement")
	}
}

// A stopped database restarts itself. The deadline is computed, not assumed.
func TestDatabaseRestoreDeadlineIsSevenDays(t *testing.T) {
	fired := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	e := model.Entry{Kind: model.KindRDSInstance}
	got, ok := e.RestoreDeadline(fired)
	if !ok {
		t.Fatal("a database entry must have a deadline")
	}
	if want := fired.Add(7 * 24 * time.Hour); !got.Equal(want) {
		t.Errorf("deadline %v, want %v", got, want)
	}
	if _, ok := (model.Entry{Kind: model.KindLambda}).RestoreDeadline(fired); ok {
		t.Error("only databases have an auto-restart deadline")
	}
}

// Stopping an instance erases its instance store, and the API does not warn.
func TestInstanceStoreLossIsRefusedByDefault(t *testing.T) {
	inst := res(model.KindEC2Instance, "i-1", nil)
	inst.Prior = map[string]any{"state": "running"}
	inst.HasInstanceStore = true

	p := build(prodScope(), inst)
	if len(p.Actions) != 0 {
		t.Error("an instance-store instance must not be stopped by default")
	}
	if why, _ := refusalFor(p, "i-1"); !strings.Contains(why, "instance-store") {
		t.Errorf("the refusal must say why, got %q", why)
	}

	pol := prodScope()
	pol.AllowInstanceStoreLoss = true
	on := build(pol, inst)
	if len(on.Actions) != 1 {
		t.Fatal("the flag should allow it")
	}
	if !strings.Contains(on.Actions[0].Warning, "lost permanently") {
		t.Error("allowing it must still warn")
	}
	if !on.BlastRadius().DataLoss {
		t.Error("blast radius must report that data will be lost")
	}
}

// NAT gateways cannot be stopped, only deleted — which is a different promise
// from the rest of the tool, so it is opt-in and says so.
func TestNATGatewayDeletionIsOptInAndFlagged(t *testing.T) {
	p := build(prodScope(), res(model.KindNATGateway, "nat-1", nil))
	if len(p.Actions) != 0 {
		t.Error("NAT gateways must not be deleted by default")
	}
	pol := prodScope()
	pol.DeleteNATGateways = true
	on := build(pol, res(model.KindNATGateway, "nat-1", nil))
	if len(on.Actions) != 1 || on.Actions[0].Phase != model.PhaseNetwork {
		t.Fatal("opting in should schedule it in the network phase")
	}
	if !strings.Contains(on.Actions[0].Warning, "not a stop") {
		t.Errorf("the warning must distinguish deletion from stopping, got %q", on.Actions[0].Warning)
	}
}

// Firing at something already stopped is noise in the plan and, worse, records
// a prior state of zero that a restore would faithfully put back.
func TestAlreadyStoppedResourcesAreSkipped(t *testing.T) {
	asg := res(model.KindASG, "asg-1", nil)
	asg.Prior = map[string]any{"desired_capacity": 0}
	svc := res(model.KindECSService, "svc-1", nil)
	svc.Prior = map[string]any{"desired_count": 0}
	inst := res(model.KindEC2Instance, "i-1", nil)
	inst.Prior = map[string]any{"state": "stopped"}

	p := build(prodScope(), asg, svc, inst)
	if len(p.Actions) != 0 {
		t.Errorf("nothing already at rest should be scheduled, got %v", kinds(p))
	}
	if len(p.Refusals) != 3 {
		t.Errorf("each should be explained, got %d refusals", len(p.Refusals))
	}
}

// JSON turns every number into a float64. A desired count of 3 read back as an
// int assertion fails, and a restore would set zero — leaving the account down
// after a restore that reported success.
func TestPriorIntSurvivesJSONRoundTrip(t *testing.T) {
	prior := map[string]any{"a": float64(3), "b": int(4), "c": int64(5), "d": int32(6)}
	for k, want := range map[string]int{"a": 3, "b": 4, "c": 5, "d": 6} {
		got, ok := model.PriorInt(prior, k)
		if !ok || got != want {
			t.Errorf("%s: got %d ok=%v, want %d", k, got, ok, want)
		}
	}
	if _, ok := model.PriorInt(prior, "missing"); ok {
		t.Error("a missing key must report not-ok rather than zero")
	}
	if _, ok := model.PriorInt(map[string]any{"s": "3"}, "s"); ok {
		t.Error("a string must not be silently coerced")
	}
}

// Restore runs the plan backwards: compute comes back before traffic is let in,
// or the first request lands on nothing.
func TestRestoreBringsComputeBackBeforeIngress(t *testing.T) {
	entries := []model.Entry{
		{ID: "listener-1", Kind: model.KindALBListener, Phase: model.PhaseIngress},
		{ID: "asg-1", Kind: model.KindASG, Phase: model.PhaseCompute},
		{ID: "db-1", Kind: model.KindRDSInstance, Phase: model.PhaseData},
	}
	got := RestoreOrder(entries)
	if got[0].Phase != model.PhaseData || got[len(got)-1].Phase != model.PhaseIngress {
		t.Errorf("restore order wrong: %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

// A big plan must not fire on a single keystroke.
func TestLargePlansRequireAcknowledgement(t *testing.T) {
	var many []model.Resource
	for i := 0; i < 30; i++ {
		many = append(many, res(model.KindLambda, "fn-"+string(rune('a'+i%26))+string(rune('0'+i/26)), nil))
	}
	pol := prodScope()
	pol.ConfirmAbove = 25
	p := build(pol, many...)
	if p.BlastRadius().Total <= 25 {
		t.Fatalf("expected more than 25 actions, got %d", p.BlastRadius().Total)
	}
	var found bool
	for _, a := range p.AckRequired {
		if strings.Contains(a, "threshold") {
			found = true
		}
	}
	if !found {
		t.Errorf("crossing the threshold must require acknowledgement, got %v", p.AckRequired)
	}
}

// The one that costs money to get wrong in the other direction: a policy with
// no scope must refuse to do anything at all, rather than defaulting to the
// whole account.
func TestPolicyWithNoScopeIsInvalid(t *testing.T) {
	if err := (policy.Policy{}).Validate(); err == nil {
		t.Error("an unscoped policy must not validate")
	}
	if err := (policy.Policy{Scope: policy.Scope{Everything: true, Tags: map[string]string{"a": "b"}}}).Validate(); err == nil {
		t.Error("everything and tags together is a contradiction and must be rejected")
	}
	if err := (policy.Policy{Scope: policy.Scope{
		Tags: map[string]string{policy.ProtectTag: "true"}}}).Validate(); err == nil {
		t.Error("scoping on the protect tag must be rejected; it only ever excludes")
	}
	if err := (policy.Policy{Scope: policy.Scope{Tags: map[string]string{"Env": "prod"}}}).Validate(); err != nil {
		t.Errorf("a tag-scoped policy should validate: %v", err)
	}
}

// Two runs over the same inventory must produce the same plan, or a reviewed
// plan is not the plan that fires.
func TestPlanIsDeterministic(t *testing.T) {
	rs := []model.Resource{
		res(model.KindLambda, "fn-b", nil), res(model.KindLambda, "fn-a", nil),
		res(model.KindASG, "asg-z", nil), res(model.KindALBListener, "l-1", nil),
	}
	first := build(prodScope(), rs...)
	// Same set, different discovery order.
	shuffled := []model.Resource{rs[2], rs[0], rs[3], rs[1]}
	second := build(prodScope(), shuffled...)

	if len(first.Actions) != len(second.Actions) {
		t.Fatal("action counts differ")
	}
	for i := range first.Actions {
		if first.Actions[i].Resource.ID != second.Actions[i].Resource.ID {
			t.Errorf("position %d: %s vs %s", i,
				first.Actions[i].Resource.ID, second.Actions[i].Resource.ID)
		}
	}
}
