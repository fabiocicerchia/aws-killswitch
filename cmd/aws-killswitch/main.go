// aws-killswitch — stop an account spending, without losing anything.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/engine"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

const usage = `aws-killswitch — stop an account spending, without losing anything

  aws-killswitch plan             what would be stopped, and what would not
  aws-killswitch fire             stop it (requires --yes)
  aws-killswitch status           what is currently stopped, and any deadlines
  aws-killswitch restore <plan>   put everything back
  aws-killswitch spend            month-to-date cost against the threshold

Flags
  -config PATH        policy file (default ./killswitch.json)
  -state URI          where snapshots live: s3://bucket/prefix (overrides config)
  -local DIR          local snapshot copy (default ~/.aws-killswitch)
  -audit PATH         append-only log (default ~/.aws-killswitch/audit.jsonl)
  -threshold USD      for ` + "`spend`" + `: exit non-zero above this
  -yes                actually make changes; without it everything is a dry run
  -force              proceed past the blast-radius threshold and any warnings
  -json               machine-readable output

Nothing changes without --yes. Stateful storage — S3, EFS, DynamoDB, EBS
volumes, snapshots, backups — is never touched under any flag.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}

	fs := flag.NewFlagSet("aws-killswitch", flag.ExitOnError)
	configPath := fs.String("config", "killswitch.json", "")
	stateURI := fs.String("state", "", "")
	localDir := fs.String("local", defaultLocalDir(), "")
	auditPath := fs.String("audit", "", "")
	threshold := fs.Float64("threshold", 0, "")
	yes := fs.Bool("yes", false, "")
	force := fs.Bool("force", false, "")
	asJSON := fs.Bool("json", false, "")
	fs.Usage = func() { fmt.Print(usage) }

	cmd := os.Args[1]
	var positional []string
	rest := os.Args[2:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			os.Exit(2)
		}
		rest = fs.Args()
		if len(rest) > 0 {
			positional = append(positional, rest[0])
			rest = rest[1:]
		}
	}

	ctx := context.Background()
	if err := run(ctx, cmd, positional, options{
		configPath: *configPath, stateURI: *stateURI, localDir: *localDir,
		auditPath: *auditPath, threshold: *threshold,
		yes: *yes, force: *force, asJSON: *asJSON,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	configPath, stateURI, localDir, auditPath string
	threshold                                 float64
	yes, force, asJSON                        bool
}

func run(ctx context.Context, cmd string, args []string, o options) error {
	switch cmd {
	case "plan", "fire", "status", "restore", "spend":
	default:
		fmt.Print(usage)
		return errors.New("unknown command " + cmd)
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS credentials: %w", err)
	}

	if cmd == "spend" {
		return cmdSpend(ctx, cfg, o)
	}

	pol, err := loadPolicy(o.configPath)
	if err != nil {
		return err
	}
	if o.stateURI != "" {
		pol.StateURI = o.stateURI
	}
	if err := pol.Validate(); err != nil {
		return fmt.Errorf("%s: %w", o.configPath, err)
	}

	log, logErr := audit.New(o.auditPathOrDefault())
	if logErr != nil {
		// An unwritable audit file must not be the reason a cost incident goes
		// unhandled, but it should be said out loud.
		fmt.Fprintf(os.Stderr, "warning: audit log unavailable (%v); continuing\n", logErr)
	}

	store, err := buildStore(ctx, cfg, pol, o.localDir)
	if err != nil {
		return err
	}

	switch cmd {
	case "status":
		return cmdStatus(ctx, store, o)
	case "restore":
		if len(args) == 0 {
			return errors.New("restore needs a plan id — see `aws-killswitch status`")
		}
		return cmdRestore(ctx, cfg, store, log, args[0], o)
	}

	account := awsx.AccountID(ctx, cfg)
	regions := pol.Scope.Regions
	if len(regions) == 0 {
		regions = []string{cfg.Region}
	}

	clients := map[string]*awsx.Clients{}
	var resources []model.Resource
	var discoveryErrs []error
	for i, region := range regions {
		c := awsx.NewClients(cfg, region)
		if i == 0 {
			// CloudFront is global. Exactly one region walks it, or a run across
			// five regions plans the same five disables.
			c = c.WithCloudFront(cfg)
		}
		clients[region] = c
		rs, errs := c.Discover(ctx)
		resources = append(resources, rs...)
		discoveryErrs = append(discoveryErrs, errs...)
	}
	for _, e := range discoveryErrs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}

	now := time.Now().UTC()
	p := plan.Build(plan.Input{
		Account: account, Regions: regions, Resources: resources,
		Now: now, PlanID: planID(now),
	}, pol)

	if cmd == "plan" {
		return printPlan(p, pol, o)
	}
	return cmdFire(ctx, p, pol, store, awsx.NewExecutor(clients), log, o)
}

// --- commands ----------------------------------------------------------------

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
			fmt.Printf("  %-14s %-40s %s\n", a.Resource.Kind, truncate(a.Resource.Ref(), 40), a.Op)
		}
		fmt.Println()
	}

	if len(p.Refusals) > 0 {
		fmt.Printf("NOT TOUCHED (%d)\n", len(p.Refusals))
		for _, r := range p.Refusals {
			fmt.Printf("  %-14s %-40s %s\n", r.Resource.Kind, truncate(r.Resource.Ref(), 40), r.Reason)
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
			region := e.Region
			if region == model.GlobalRegion {
				// A distribution has no region. Build the client on the
				// configured default; only its CloudFront half is ever used.
				region = cfg.Region
			}
			// Every executor client carries the global client, so a restore
			// that includes a distribution can always reach it.
			clients[e.Region] = awsx.NewClients(cfg, region).WithCloudFront(cfg)
		}
	}

	if !o.yes {
		fmt.Printf("plan %s would restore %d resources, compute first:\n\n", planID, len(changed))
		for _, e := range plan.RestoreOrder(changed) {
			fmt.Printf("  %-14s %-40s %s\n", e.Kind, truncate(refOf(e), 40), e.Phase)
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

// --- wiring ------------------------------------------------------------------

func loadPolicy(path string) (policy.Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy.Policy{}, fmt.Errorf(
				"no policy at %s — this tool will not act without one, because the default would otherwise be the whole account.\n"+
					"Write one:\n\n%s", path, samplePolicy)
		}
		return policy.Policy{}, err
	}
	p, err := policy.Parse(b)
	if err != nil {
		return policy.Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

const samplePolicy = `{
  "scope": { "tags": { "Env": "dev" }, "regions": ["eu-west-1"] },
  "state_uri": "s3://my-ops-bucket/killswitch",
  "confirm_above": 25,
  "include_databases": false,
  "delete_nat_gateways": false
}
`

func buildStore(_ context.Context, cfg aws.Config, pol policy.Policy, localDir string) (state.Store, error) {
	if pol.StateURI == "" {
		fmt.Fprintf(os.Stderr,
			"warning: no state_uri — snapshots are only in %s. Lose that directory and the restore record goes with it.\n", localDir)
	}
	// Durable store first, so a rebuilt laptop still finds the record.
	return state.Build(s3.NewFromConfig(cfg), pol.StateURI, localDir)
}

func (o options) auditPathOrDefault() string {
	if o.auditPath != "" {
		return o.auditPath
	}
	return filepath.Join(o.localDir, "audit.jsonl")
}

func defaultLocalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".aws-killswitch"
	}
	return filepath.Join(home, ".aws-killswitch")
}

func planID(now time.Time) string { return "ks-" + now.Format("20060102-150405") }

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
