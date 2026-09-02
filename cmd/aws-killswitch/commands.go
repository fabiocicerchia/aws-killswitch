package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/engine"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

func cmdFire(ctx context.Context, p model.Plan, pol policy.Policy, store state.Store, ex engine.Executor, log *audit.Log, o options) error {
	if p.IsEmpty() {
		return errors.New("nothing in scope to stop")
	}
	if len(p.AckRequired) > 0 && !o.force {
		fmt.Fprintln(os.Stderr, "this plan needs --force:")
		for _, a := range p.AckRequired {
			fmt.Fprintf(os.Stderr, "  ! %s\n", a)
		}
		return errors.New("refusing to fire without acknowledgement")
	}

	opt := engine.Options{DryRun: !o.yes, ContinueOnError: true, Log: log}
	if !o.yes {
		if err := printPlan(p, pol, o); err != nil {
			return err
		}
		fmt.Println("\nThis was a dry run — --yes was not passed, so nothing changed.")
		return nil
	}

	res, err := engine.Fire(ctx, p, store, ex, opt)
	if err != nil {
		return err
	}
	fmt.Printf("plan %s: %d changed, %d failed\n", p.ID, res.Changed, res.Failed)
	for _, d := range engine.Deadlines(res.Snapshot, time.Now().UTC()) {
		fmt.Printf("  ! %s\n", d)
	}
	fmt.Printf("\nRestore with:  aws-killswitch restore %s --yes\n", p.ID)
	if res.Failed > 0 {
		return fmt.Errorf("%d resources could not be stopped; see the audit log", res.Failed)
	}
	return nil
}

func cmdStatus(ctx context.Context, store state.Store, o options) error {
	snaps, err := store.List(ctx)
	if err != nil {
		return err
	}
	if o.asJSON {
		return emit(snaps)
	}
	if len(snaps) == 0 {
		fmt.Printf("no snapshots in %s — nothing has been fired from here\n", store.Describe())
		return nil
	}
	now := time.Now().UTC()
	for _, s := range snaps {
		state := "planned"
		switch {
		case s.Restored != nil:
			state = "restored " + s.Restored.Format(time.RFC3339)
		case s.Fired != nil:
			state = "fired " + s.Fired.Format(time.RFC3339)
		}
		fmt.Printf("%s  %s  %d changed of %d  (%s)\n",
			s.PlanID, state, len(s.Changed()), len(s.Entries), s.Account)
		for _, d := range engine.Deadlines(s, now) {
			fmt.Printf("    ! %s\n", d)
		}
	}
	return nil
}

func cmdRestore(ctx context.Context, cfg aws.Config, store state.Store, log *audit.Log, planID string, o options) error {
	snap, err := store.Get(ctx, planID)
	if err != nil {
		return err
	}
	changed := snap.Changed()
	if len(changed) == 0 {
		return errors.New("this snapshot records no changes to undo")
	}

	clients := map[string]*awsx.Clients{}
	for _, e := range changed {
		if _, ok := clients[e.Region]; !ok {
			clients[e.Region] = awsx.NewClients(cfg, e.Region)
		}
	}

	if !o.yes {
		fmt.Printf("plan %s would restore %d resources, compute first:\n\n", planID, len(changed))
		for _, e := range plan.RestoreOrder(changed) {
			fmt.Printf(planRowFormat, e.Kind, refWidth, truncate(refOf(e), refWidth), e.Phase)
		}
		fmt.Println("\nDry run — pass --yes to apply.")
		return nil
	}

	// A restore that presses on after a failure usually means credentials are
	// wrong, and would report a misleading partial success.
	res, err := engine.Restore(ctx, snap, store, awsx.NewExecutor(clients),
		engine.Options{ContinueOnError: false, Log: log})
	if err != nil {
		return err
	}
	fmt.Printf("restored %d, failed %d\n", res.Changed, res.Failed)
	return nil
}

func cmdSpend(ctx context.Context, cfg aws.Config, o options) error {
	// Cost Explorer is only in us-east-1.
	c := cfg.Copy()
	c.Region = "us-east-1"
	s, err := awsx.MonthToDate(ctx, costexplorer.NewFromConfig(c), time.Now().UTC())
	if err != nil {
		return err
	}
	if o.asJSON {
		return emit(s)
	}
	fmt.Printf("month to date: $%.2f (%s to %s)\n",
		s.MonthToDateUSD, s.Start.Format("2006-01-02"), s.End.Format("2006-01-02"))
	if s.Stale {
		fmt.Println("  figures are estimated and lag by hours — for a fast trip, drive this from a Budgets action, not from polling")
	}
	if o.threshold > 0 {
		if s.MonthToDateUSD > o.threshold {
			fmt.Printf("  OVER the $%.2f threshold\n", o.threshold)
			os.Exit(3)
		}
		fmt.Printf("  under the $%.2f threshold ($%.2f left)\n", o.threshold, o.threshold-s.MonthToDateUSD)
	}
	return nil
}
