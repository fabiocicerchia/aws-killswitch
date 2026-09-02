// aws-killswitch — stop an account spending, without losing anything.
//
// This file is the front door only: flags, the command table, and the order the
// stages run in. The stages themselves live next to it — commands.go runs one,
// render.go prints one, wiring.go builds what they need.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/plan"
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

	pol, err := policyFor(o)
	if err != nil {
		return err
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

	account := accountID(ctx, cfg)
	regions, clients, resources := discoverAll(ctx, cfg, pol)

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
