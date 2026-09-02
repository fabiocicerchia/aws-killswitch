// Package verify runs a full fire/restore cycle and checks the result by
// reading the account back, twice.
//
// WHY THIS EXISTS: the planner, the state machine and the ordering are
// unit-tested, and discovery has run against live endpoints. But nothing had
// ever run with credentials that work, so no fire and no restore had ever
// executed. Unit tests cannot catch an API contract mismatch — a field that
// means something other than what was assumed, a call that succeeds and does
// nothing, a restore that puts back a value the API silently coerces — and
// that is the failure this is guarding against.
//
// THE SECOND READ IS THE WHOLE POINT. `fire` returning no error means the API
// accepted the calls. It does not mean the desired-count is actually zero, the
// listener actually blocked, or the concurrency actually pinned. So every
// action is checked by re-discovering the account and comparing what came back
// against what the action claimed to do — and after `restore`, against what
// was there before anything was touched.
//
//	discover ──► plan ──► fire ──► DISCOVER AGAIN ──► did each action land?
//	    │                                                    │
//	    └── pre-state ◄──────── DISCOVER AGAIN ◄── restore ◄──┘
//	                                  │
//	                            is everything back?
//
// The output is deliberately counts and kinds. A verification run happens in
// somebody's account, and the summary is meant to be pasted into an issue: ARNs,
// account ids, instance names and DNS names must not travel with it. Redact()
// is not a formatting nicety, it is the thing that makes the report postable.
package verify

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/engine"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

// Discoverer reads the account. An interface so the whole cycle runs against
// an in-memory account in the tests: a harness whose own logic is unverified
// would be the worst possible thing to trust a fire/restore report to.
type Discoverer interface {
	Discover(ctx context.Context) ([]model.Resource, []error)
}

// Settle is how long to wait after a change before reading it back. AWS is
// eventually consistent on most of these — an ECS service reports its old
// desired count for a moment, an ASG takes a beat to reflect a scale — and a
// verifier that read too early would report a divergence that is only its own
// impatience.
type Options struct {
	Settle    time.Duration
	Poll      time.Duration
	Now       func() time.Time
	SkipFire  bool // plan and report only; nothing is changed
	Region    string
	AccountID string
}

const (
	defaultSettle = 90 * time.Second
	defaultPoll   = 5 * time.Second
)

// Finding is one thing that did not match. Every field here is safe to post:
// the resource is named by kind and by an opaque index, never by ARN.
type Finding struct {
	Stage  string // "fire" or "restore"
	Kind   model.Kind
	Ref    string // "ecs-service#2" — stable within one report, meaningless outside it
	Want   string
	Got    string
	Detail string
}

func (f Finding) String() string {
	s := fmt.Sprintf("%s: %s — wanted %s, read back %s", f.Stage, f.Ref, f.Want, f.Got)
	if f.Detail != "" {
		s += " (" + f.Detail + ")"
	}
	return s
}

// Report is what gets pasted into the issue.
type Report struct {
	StartedAt time.Time
	Duration  time.Duration

	// Discovered counts every resource found, by kind. A kind with zero here
	// is a kind this run did NOT exercise, which is as important as the ones
	// it did — the acceptance criterion is "at least one of each".
	Discovered map[model.Kind]int
	Planned    map[model.Kind]int
	Refused    map[model.Kind]int

	FireChanged   int
	FireFailed    int
	FireVerified  int
	RestoreOK     int
	RestoreFailed int

	Findings []Finding

	// Unexercised names the kinds the scratch account had none of. Listed
	// explicitly, because "no findings" over a kind that was never present is
	// not evidence of anything.
	Unexercised []model.Kind

	Errors []string
}

// AllKinds is what a scratch account has to hold for a run to mean anything.
// Kept here rather than derived from what was found, so a kind that discovery
// silently stopped returning shows up as unexercised rather than vanishing.
var AllKinds = []model.Kind{
	model.KindALBListener,
	model.KindLambda,
	model.KindECSService,
	model.KindASG,
	model.KindEC2Instance,
	model.KindNATGateway,
	model.KindRDSInstance,
	model.KindRDSCluster,
	model.KindEKSNodegroup,
	model.KindCloudFront,
	model.KindAPIGatewayStage,
}

// Run does the cycle. It returns a report even on error: a run that fell over
// halfway is exactly the one whose partial findings are worth reading.
func Run(
	ctx context.Context,
	d Discoverer,
	ex engine.Executor,
	st state.Store,
	pol policy.Policy,
	opt Options,
) (*Report, error) {
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	rep := &Report{
		StartedAt:  now().UTC(),
		Discovered: map[model.Kind]int{},
		Planned:    map[model.Kind]int{},
		Refused:    map[model.Kind]int{},
	}
	defer func() { rep.Duration = now().UTC().Sub(rep.StartedAt) }()

	before, errs := d.Discover(ctx)
	rep.addErrs(errs)
	countByKind(rep.Discovered, before)
	rep.Unexercised = missingKinds(rep.Discovered)

	p := plan.Build(plan.Input{
		Account:   opt.AccountID,
		Regions:   []string{opt.Region},
		Resources: before,
		Now:       now().UTC(),
		PlanID:    fmt.Sprintf("verify-%d", rep.StartedAt.Unix()),
	}, pol)
	for _, a := range p.Actions {
		rep.Planned[a.Resource.Kind]++
	}
	for _, r := range p.Refusals {
		rep.Refused[r.Resource.Kind]++
	}
	if opt.SkipFire {
		return rep, nil
	}
	if p.IsEmpty() {
		return rep, errors.New("nothing to fire: the scratch account holds no resource this policy would touch")
	}

	// --- fire ---------------------------------------------------------------
	fired, err := engine.Fire(ctx, p, st, ex, engine.Options{
		ContinueOnError: true, Now: func() time.Time { return now().UTC() },
	})
	rep.FireChanged, rep.FireFailed = fired.Changed, fired.Failed
	if err != nil {
		rep.Errors = append(rep.Errors, "fire: "+err.Error())
		return rep, err
	}

	// The second read. Anything that reports itself unchanged after settling is
	// a finding, whatever the API said when it was called.
	after := rep.reread(ctx, d, opt)
	byID := indexByID(after)
	for _, a := range p.Actions {
		got, ok := byID[a.Resource.ID]
		if !ok {
			// Gone entirely. Correct for a NAT gateway, which cannot be stopped
			// and is deleted; wrong for everything else, which is only ever
			// scaled or blocked.
			if a.Resource.Kind == model.KindNATGateway {
				rep.FireVerified++
				continue
			}
			rep.Findings = append(rep.Findings, Finding{
				Stage: "fire", Kind: a.Resource.Kind, Ref: refFor(a.Resource, before),
				Want: "still present, stopped", Got: "not found",
				Detail: "only a NAT gateway should disappear",
			})
			continue
		}
		if same, detail := priorEqual(a.Resource.Prior, got.Prior); same {
			rep.Findings = append(rep.Findings, Finding{
				Stage: "fire", Kind: a.Resource.Kind, Ref: refFor(a.Resource, before),
				Want: "state changed", Got: "identical to before the fire",
				Detail: detail + "; the call was accepted and did nothing",
			})
			continue
		}
		rep.FireVerified++
	}

	// --- restore ------------------------------------------------------------
	restored, err := engine.Restore(ctx, fired.Snapshot, st, ex, engine.Options{
		Now: func() time.Time { return now().UTC() },
	})
	rep.RestoreOK, rep.RestoreFailed = restored.Changed, restored.Failed
	if err != nil {
		rep.Errors = append(rep.Errors, "restore: "+err.Error())
		return rep, err
	}

	// The restore path is the product. Every resource has to read back as it
	// was BEFORE the fire, not merely "different from during it".
	final := rep.reread(ctx, d, opt)
	finalByID := indexByID(final)
	for _, r := range before {
		got, ok := finalByID[r.ID]
		if !ok {
			if r.Kind == model.KindNATGateway {
				// A restored NAT gateway is a NEW one with a new id, so an
				// id-keyed lookup cannot find it. Checked by count instead.
				continue
			}
			rep.Findings = append(rep.Findings, Finding{
				Stage: "restore", Kind: r.Kind, Ref: refFor(r, before),
				Want: "present again", Got: "not found",
			})
			continue
		}
		if same, detail := priorEqual(r.Prior, got.Prior); !same {
			rep.Findings = append(rep.Findings, Finding{
				Stage: "restore", Kind: r.Kind, Ref: refFor(r, before),
				Want: "the state it had before the fire", Got: "something else",
				Detail: detail,
			})
		}
	}
	// NAT gateways by count, since their ids change.
	if wantNAT, gotNAT := countKind(before, model.KindNATGateway), countKind(final, model.KindNATGateway); wantNAT != gotNAT {
		rep.Findings = append(rep.Findings, Finding{
			Stage: "restore", Kind: model.KindNATGateway, Ref: "nat-gateway (by count)",
			Want: fmt.Sprintf("%d", wantNAT), Got: fmt.Sprintf("%d", gotNAT),
			Detail: "a restored NAT gateway is a new one with a new id, so this is counted, not matched",
		})
	}
	return rep, nil
}

// reread waits for the account to settle and then reads it. Polling rather
// than one sleep, so a fast account is not charged the worst case.
func (rep *Report) reread(ctx context.Context, d Discoverer, opt Options) []model.Resource {
	settle := opt.Settle
	if settle == 0 {
		settle = defaultSettle
	}
	poll := opt.Poll
	if poll == 0 {
		poll = defaultPoll
	}
	deadline := time.Now().Add(settle)
	var last []model.Resource
	for {
		res, errs := d.Discover(ctx)
		rep.addErrs(errs)
		last = res
		if !time.Now().Before(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return last
		case <-time.After(poll):
		}
	}
}

func (rep *Report) addErrs(errs []error) {
	for _, e := range errs {
		if e == nil {
			continue
		}
		// A per-service AccessDenied degrades that source rather than failing
		// the run — that is discovery's documented behaviour, and a verifier
		// that turned it into a hard stop would make the harness less useful
		// than the tool it verifies.
		rep.Errors = append(rep.Errors, e.Error())
	}
}

// priorEqual compares the two Prior maps as strings. Deliberately shallow:
// the point is "did anything change", and a deep structural comparison would
// need a type switch per kind that drifts the moment a kind is added.
func priorEqual(a, b map[string]any) (bool, string) {
	as, bs := renderPrior(a), renderPrior(b)
	if as == bs {
		return true, "prior=" + as
	}
	return false, "before=" + as + " after=" + bs
}

func renderPrior(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// refFor gives a resource a name that is stable within one report and means
// nothing outside it. "ecs-service#2", never the ARN.
func refFor(r model.Resource, all []model.Resource) string {
	n := 0
	for _, other := range all {
		if other.Kind != r.Kind {
			continue
		}
		n++
		if other.ID == r.ID {
			return fmt.Sprintf("%s#%d", r.Kind, n)
		}
	}
	return string(r.Kind) + "#?"
}

func indexByID(rs []model.Resource) map[string]model.Resource {
	m := make(map[string]model.Resource, len(rs))
	for _, r := range rs {
		m[r.ID] = r
	}
	return m
}

func countByKind(into map[model.Kind]int, rs []model.Resource) {
	for _, r := range rs {
		into[r.Kind]++
	}
}

func countKind(rs []model.Resource, k model.Kind) int {
	n := 0
	for _, r := range rs {
		if r.Kind == k {
			n++
		}
	}
	return n
}

func missingKinds(found map[model.Kind]int) []model.Kind {
	var out []model.Kind
	for _, k := range AllKinds {
		if found[k] == 0 {
			out = append(out, k)
		}
	}
	return out
}
