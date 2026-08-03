// Package engine executes a plan, and enforces the two rules that make firing
// one survivable: the restore record is written first, and it is updated after
// every single change.
//
// The second rule matters more than it looks. A fire interrupted halfway —
// network drops, credentials expire, someone hits ctrl-C — must leave a record
// that says exactly which resources were changed and which were not. Writing
// the snapshot only at the end would turn an interrupted fire into an account
// in an unknown state, which is the situation this tool exists to prevent.
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

// Executor performs one change, or undoes one.
type Executor interface {
	Apply(ctx context.Context, a model.Action) error
	Restore(ctx context.Context, e model.Entry) error
}

type Options struct {
	// DryRun prints what would happen and touches nothing. The default
	// everywhere: firing has to be the thing you asked for explicitly.
	DryRun bool
	// ContinueOnError keeps going when one resource fails. On by default for a
	// fire — a single unstoppable resource should not leave the rest running —
	// and off for a restore, where an early failure usually means credentials
	// are wrong and pressing on would produce a misleading partial success.
	ContinueOnError bool
	Now             func() time.Time
	Log             *audit.Log
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now().UTC()
}

type Result struct {
	Snapshot model.Snapshot
	Changed  int
	Failed   int
	Skipped  int
}

// Fire executes the plan. The snapshot is written and verified before the first
// change; if that fails, nothing is touched.
func Fire(ctx context.Context, p model.Plan, st state.Store, ex Executor, opt Options) (Result, error) {
	if p.IsEmpty() {
		return Result{}, errors.New("plan is empty; nothing to do")
	}

	snap := state.From(p)
	firedAt := opt.now()
	snap.Fired = &firedAt

	if opt.DryRun {
		// A dry run must not write a snapshot either — a record of a fire that
		// did not happen is exactly the thing someone would later restore from.
		for i := range snap.Entries {
			snap.Entries[i].Result = model.ResultSkipped
		}
		return Result{Snapshot: snap, Skipped: len(snap.Entries)}, nil
	}

	// Nothing has changed yet. This is the last point at which abandoning the
	// fire is free, and a store that cannot hold the record is a reason to.
	if err := state.PutVerified(ctx, st, snap); err != nil {
		return Result{}, fmt.Errorf("refusing to fire: %w", err)
	}
	opt.Log.Event("fire.begin", map[string]any{
		"plan": p.ID, "actions": len(p.Actions), "store": st.Describe(),
	})

	var res Result
	res.Snapshot = snap
	byIndex := indexEntries(snap.Entries)

	for _, a := range p.Actions {
		i, ok := byIndex[key(a.Resource.Kind, a.Resource.ID)]
		if !ok {
			continue
		}
		err := ex.Apply(ctx, a)
		if err != nil {
			snap.Entries[i].Result = model.ResultFailed
			snap.Entries[i].Error = err.Error()
			res.Failed++
			opt.Log.Event("action.failed", map[string]any{
				"kind": a.Resource.Kind, "id": a.Resource.ID, "error": err.Error(),
			})
		} else {
			snap.Entries[i].Result = model.ResultChanged
			res.Changed++
			opt.Log.Event("action.applied", map[string]any{
				"kind": a.Resource.Kind, "id": a.Resource.ID, "op": a.Op,
			})
		}

		// After every change, not at the end. An interrupted fire must still
		// leave a record of exactly what was touched.
		if perr := st.Put(ctx, snap); perr != nil {
			opt.Log.Event("snapshot.write_failed", map[string]any{"error": perr.Error()})
			// The change is already made; losing the record from here is worse
			// than stopping, so stop rather than change anything further.
			return res, fmt.Errorf("changed %s but could not update the restore record (%w) — stopping here; run `restore` against plan %s",
				a.Resource.Ref(), perr, p.ID)
		}
		if err != nil && !opt.ContinueOnError {
			return res, fmt.Errorf("stopping after %s failed: %w", a.Resource.Ref(), err)
		}
	}

	res.Snapshot = snap
	opt.Log.Event("fire.end", map[string]any{
		"changed": res.Changed, "failed": res.Failed,
	})
	return res, nil
}

// Restore puts back what was changed, in reverse phase order.
func Restore(ctx context.Context, snap model.Snapshot, st state.Store, ex Executor, opt Options) (Result, error) {
	changed := snap.Changed()
	if len(changed) == 0 {
		return Result{Snapshot: snap}, errors.New("this snapshot records no changes to undo")
	}

	entries := plan.RestoreOrder(changed)
	if opt.DryRun {
		return Result{Snapshot: snap, Skipped: len(entries)}, nil
	}

	opt.Log.Event("restore.begin", map[string]any{"plan": snap.PlanID, "entries": len(entries)})
	byIndex := indexEntries(snap.Entries)

	var res Result
	for _, e := range entries {
		i, ok := byIndex[key(e.Kind, e.ID)]
		if !ok {
			continue
		}
		if err := ex.Restore(ctx, e); err != nil {
			snap.Entries[i].Error = err.Error()
			res.Failed++
			opt.Log.Event("restore.failed", map[string]any{
				"kind": e.Kind, "id": e.ID, "error": err.Error(),
			})
			_ = st.Put(ctx, snap)
			if !opt.ContinueOnError {
				return res, fmt.Errorf("restoring %s failed: %w", e.ID, err)
			}
			continue
		}
		// Clearing the result is what stops a second restore from running the
		// same change twice.
		snap.Entries[i].Result = model.ResultUnchanged
		snap.Entries[i].Error = ""
		res.Changed++
		opt.Log.Event("restore.applied", map[string]any{"kind": e.Kind, "id": e.ID})
		if perr := st.Put(ctx, snap); perr != nil {
			return res, fmt.Errorf("restored %s but could not update the record: %w", e.ID, perr)
		}
	}

	if res.Failed == 0 {
		done := opt.now()
		snap.Restored = &done
		_ = st.Put(ctx, snap)
	}
	res.Snapshot = snap
	opt.Log.Event("restore.end", map[string]any{"restored": res.Changed, "failed": res.Failed})
	return res, nil
}

// Deadlines reports databases that AWS will restart by itself, and when. This
// is the thing that turns a kill switch into a surprise a week later.
func Deadlines(snap model.Snapshot, now time.Time) []string {
	if snap.Fired == nil {
		return nil
	}
	var out []string
	for _, e := range snap.Changed() {
		deadline, ok := e.RestoreDeadline(*snap.Fired)
		if !ok {
			continue
		}
		left := deadline.Sub(now)
		switch {
		case left <= 0:
			out = append(out, fmt.Sprintf("%s %s: AWS has already restarted this — it is billing again", e.Kind, e.ID))
		default:
			out = append(out, fmt.Sprintf("%s %s: AWS restarts this in %s (%s)",
				e.Kind, e.ID, round(left), deadline.Format(time.RFC3339)))
		}
	}
	return out
}

func round(d time.Duration) string {
	if d > 24*time.Hour {
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	}
	return d.Round(time.Minute).String()
}

func indexEntries(entries []model.Entry) map[string]int {
	m := make(map[string]int, len(entries))
	for i, e := range entries {
		m[key(e.Kind, e.ID)] = i
	}
	return m
}

func key(k model.Kind, id string) string { return string(k) + "\x00" + id }
