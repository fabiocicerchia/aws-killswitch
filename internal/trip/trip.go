// Package trip is one discover → plan → fire cycle, with nothing in it that
// belongs to a particular way of being invoked.
//
// It exists because there are now two front ends. The CLI is fired by a person
// who has seen a bill; the Lambda is fired by an AWS Budgets action, which sees
// the spend signal itself and does not wait for a cron interval. Both must take
// exactly the same path — a second implementation of "what gets stopped" is a
// second thing to get wrong, and the one that only runs at three in the morning
// during an incident is the one nobody would notice was wrong.
package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/engine"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

// Cooldown is how long after a fire a second trip is refused as a duplicate.
//
// A Budgets action is not delivered once. The same threshold can notify
// repeatedly, and a retried invocation is the normal case rather than the
// exceptional one — so without this, the second delivery would discover an
// account that is already stopped, plan almost nothing, and write a second
// snapshot whose "prior" state is the stopped one. Restoring that snapshot
// would put the account back exactly as it was after the first fire: down.
//
// An hour is long enough to cover retries and re-notifications, and short
// enough that a genuine second incident the same day is not blocked.
const Cooldown = time.Hour

// Options are the choices a caller makes about one trip.
type Options struct {
	// DryRun plans and reports without changing anything.
	DryRun bool
	// Force applies a plan that carries acknowledgement-required warnings.
	Force bool
	// Now is the clock, injectable for tests.
	Now time.Time
	// SkipCooldown fires even if something was fired inside the window. Only
	// for a human who has decided that is what they want.
	SkipCooldown bool
}

// Result is what happened, in the terms a caller reports.
type Result struct {
	Plan      model.Plan
	Fired     bool
	Skipped   string // why nothing was fired, when nothing was
	Changed   int
	Failed    int
	Snapshot  model.Snapshot
	Discovery []error
}

// Discover walks every region in the policy's scope, read-only.
//
// Per-service failures are collected rather than returned: a missing permission
// on one service should degrade the plan and say so, not leave the operator
// with nothing during an incident.
func Discover(ctx context.Context, cfg aws.Config, regions []string) ([]model.Resource, map[string]*awsx.Clients, []error) {
	clients := map[string]*awsx.Clients{}
	var resources []model.Resource
	var errs []error
	for i, region := range regions {
		c := awsx.NewClients(cfg, region)
		if i == 0 {
			// CloudFront is global. Exactly one region walks it, or a run
			// across five regions plans the same five disables.
			c = c.WithCloudFront(cfg)
		}
		clients[region] = c
		rs, e := c.Discover(ctx)
		resources = append(resources, rs...)
		errs = append(errs, e...)
	}
	return resources, clients, errs
}

// Plan discovers and builds, without changing anything.
func Plan(ctx context.Context, cfg aws.Config, pol policy.Policy, account string, now time.Time) (model.Plan, map[string]*awsx.Clients, []error) {
	regions := pol.Scope.Regions
	if len(regions) == 0 {
		regions = []string{cfg.Region}
	}
	resources, clients, errs := Discover(ctx, cfg, regions)
	p := plan.Build(plan.Input{
		Account: account, Regions: regions, Resources: resources,
		Now: now, PlanID: PlanID(now),
	}, pol)
	return p, clients, errs
}

// PlanID is the identifier a plan is stored and restored under.
func PlanID(now time.Time) string { return now.UTC().Format("20060102-150405") }

// Fire runs the whole cycle. This is the path both front ends take.
func Fire(ctx context.Context, cfg aws.Config, pol policy.Policy, account string, st state.Store, log *audit.Log, opt Options) (Result, error) {
	now := opt.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if !opt.SkipCooldown && !opt.DryRun {
		if since, recent := FiredRecently(ctx, st, now); recent {
			return Result{Skipped: fmt.Sprintf(
				"already fired %s ago; inside the %s cooldown", round(since), Cooldown,
			)}, nil
		}
	}

	p, clients, discoveryErrs := Plan(ctx, cfg, pol, account, now)
	res := Result{Plan: p, Discovery: discoveryErrs}

	if p.IsEmpty() {
		res.Skipped = "nothing in scope to stop"
		return res, nil
	}
	if len(p.AckRequired) > 0 && !opt.Force {
		res.Skipped = fmt.Sprintf("%d action(s) need acknowledgement; not firing unattended", len(p.AckRequired))
		return res, nil
	}

	out, err := engine.Fire(ctx, p, st, awsx.NewExecutor(clients), engine.Options{
		DryRun: opt.DryRun, ContinueOnError: true, Log: log,
		Now: func() time.Time { return now },
	})
	if err != nil {
		return res, err
	}
	res.Fired = !opt.DryRun
	res.Changed, res.Failed, res.Snapshot = out.Changed, out.Failed, out.Snapshot
	return res, nil
}

// FiredRecently reports whether any snapshot was fired inside the cooldown.
//
// Read from the state store rather than from anything in the process, because
// the caller that matters is a Lambda: every delivery is a cold start with no
// memory of the last one, and the snapshot is the only thing both share.
func FiredRecently(ctx context.Context, st state.Store, now time.Time) (time.Duration, bool) {
	snaps, err := st.List(ctx)
	if err != nil {
		// Unreadable state is not permission to fire twice, but it is also not
		// a reason to refuse during an incident. Say nothing and let the caller
		// proceed — the alternative is a killswitch that will not press.
		return 0, false
	}
	for _, s := range snaps {
		if s.Fired == nil || s.Restored != nil {
			continue
		}
		if since := now.Sub(*s.Fired); since >= 0 && since < Cooldown {
			return since, true
		}
	}
	return 0, false
}

func round(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}
