package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
)

const (
	// planRowFormat lays out one resource row — kind, reference, note — in the
	// plan, the refusals and the restore preview, so the three read as one
	// table. refWidth is passed into it as the reference column's width and is
	// what the reference is truncated to: the two have to be the same number or
	// a long name pushes the notes out of line.
	planRowFormat = "  %-14s %-*s %s\n"
	refWidth      = 40
)

func printPlan(p model.Plan, pol policy.Policy, o options) error {
	if o.asJSON {
		return emit(p)
	}
	b := p.BlastRadius()
	fmt.Printf("plan %s — account %s, regions %s\n\n", p.ID, p.Account, strings.Join(p.Regions, ", "))

	if p.IsEmpty() {
		fmt.Println("Nothing in scope would be changed.")
	}
	for phase, actions := range p.ByPhase() {
		if len(actions) == 0 {
			continue
		}
		ph := model.Phase(phase)
		fmt.Printf("%s — %s\n", strings.ToUpper(ph.String()), ph.Why())
		for _, a := range actions {
			fmt.Printf(planRowFormat, a.Resource.Kind, refWidth, truncate(a.Resource.Ref(), refWidth), a.Op)
		}
		fmt.Println()
	}

	if len(p.Refusals) > 0 {
		fmt.Printf("NOT TOUCHED (%d)\n", len(p.Refusals))
		for _, r := range p.Refusals {
			fmt.Printf(planRowFormat, r.Resource.Kind, refWidth, truncate(r.Resource.Ref(), refWidth), r.Reason)
		}
		fmt.Println()
	}

	fmt.Printf("%d resources would change", b.Total)
	if b.SavingsUSD > 0 {
		fmt.Printf(", saving about $%.2f/hour", b.SavingsUSD)
	}
	fmt.Println()
	printSavings(b)
	if len(p.AckRequired) > 0 {
		fmt.Println("\nRequires --force:")
		for _, a := range p.AckRequired {
			fmt.Printf("  ! %s\n", a)
		}
	}
	if !p.IsEmpty() {
		fmt.Println("\nNothing has changed. Run `fire --yes` to apply.")
	}
	return nil
}

// printSavings breaks the estimate down per kind, and says which resources
// have no estimate at all.
//
// The two are reported separately on purpose. Folding an unpriced resource
// into the total as zero would assert that stopping it is free, which is a
// different claim from not knowing what it costs — and the second is the
// honest one for a family this build has never seen.
func printSavings(b model.BlastRadius) {
	kinds := make([]model.Kind, 0, len(b.SavingsByKind)+len(b.UnpricedByKind))
	for k := range b.SavingsByKind {
		kinds = append(kinds, k)
	}
	for k := range b.UnpricedByKind {
		if _, seen := b.SavingsByKind[k]; !seen {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		return
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	fmt.Println("\nEstimated hourly saving (us-east-1 list price, compute only):")
	for _, k := range kinds {
		line := fmt.Sprintf("  %-14s", k)
		if usd, ok := b.SavingsByKind[k]; ok {
			line += fmt.Sprintf(" $%.2f/hour", usd)
		} else {
			line += fmt.Sprintf(" %-11s", "not estimated")
		}
		if n := b.UnpricedByKind[k]; n > 0 {
			line += fmt.Sprintf("  (%d not estimated)", n)
		}
		fmt.Println(line)
	}
}

func refOf(e model.Entry) string {
	if e.Name != "" && e.Name != e.ID {
		return e.Name + " (" + e.ID + ")"
	}
	return e.ID
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func emit(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
