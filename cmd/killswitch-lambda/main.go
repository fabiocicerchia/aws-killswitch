// Command killswitch-lambda fires the killswitch from an AWS Budgets action,
// so the fast trip is driven by the spend signal itself.
//
// Give it a generous timeout — 5 minutes is a reasonable floor. Discovery
// across many regions is not fast, and a Lambda killed mid-fire leaves a
// snapshot written with only some of it applied. That is recoverable, because
// the snapshot goes in before the first change, but it is not what anyone wants
// to find.
//
// A cron schedule can only be as fast as its interval: the worst case is a full
// interval of spend before anything happens, and shortening the interval trades
// cost for latency without ever getting below it. A Budgets action fires when
// the threshold is crossed.
//
// Everything about *what* gets stopped lives in internal/trip, which the CLI
// uses too. There is no second implementation here — the path that only runs at
// three in the morning during an incident is the one nobody would notice was
// wrong.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fabiocicerchia/aws-killswitch/internal/audit"
	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
	"github.com/fabiocicerchia/aws-killswitch/internal/trip"
)

// Event is what a Budgets action delivers, via SNS.
//
// Only the fields that change the decision are read. A Budgets notification is
// an SNS message whose Message field is a string, not an object, and the shape
// of that string is not contractual — so nothing here depends on parsing it.
// The invocation itself is the signal; the body is recorded for the log.
type Event struct {
	Records []struct {
		SNS struct {
			Subject   string `json:"Subject"`
			Message   string `json:"Message"`
			MessageID string `json:"MessageId"`
			Timestamp string `json:"Timestamp"`
		} `json:"Sns"`
	} `json:"Records"`

	// DryRun lets a test invocation exercise the whole path — credentials,
	// permissions, discovery, planning — without stopping anything. This is
	// how the wiring gets tested before there is real spend to trigger it.
	DryRun bool `json:"dry_run"`
	// Force applies a plan carrying acknowledgement-required warnings. Off by
	// default: unattended is exactly when nobody is there to acknowledge.
	Force bool `json:"force"`
}

// Response is what the invocation returns, and what lands in CloudWatch Logs.
type Response struct {
	Fired     bool     `json:"fired"`
	DryRun    bool     `json:"dry_run"`
	PlanID    string   `json:"plan_id,omitempty"`
	Changed   int      `json:"changed"`
	Failed    int      `json:"failed"`
	Skipped   string   `json:"skipped,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Trigger   string   `json:"trigger,omitempty"`
	AccountID string   `json:"account_id,omitempty"`
}

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, ev Event) (Response, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("loading AWS config: %w", err)
	}

	pol, err := loadPolicy()
	if err != nil {
		return Response{}, err
	}
	if err := pol.Validate(); err != nil {
		return Response{}, fmt.Errorf("policy: %w", err)
	}

	// No local directory: a Lambda's filesystem dies with the execution
	// environment, so a local copy would be a restore record that does not
	// exist. S3 or nothing.
	store, err := state.Build(s3.NewFromConfig(cfg), pol.StateURI, "")
	if err != nil {
		// Refusing here is the right answer even during an incident. The
		// snapshot is written and read back *before* the first change, so
		// without a store there is no restore path — and a killswitch with no
		// restore path is the thing this repo exists not to be.
		return Response{}, fmt.Errorf("state store %q unusable, refusing to fire without a restore path: %w",
			pol.StateURI, err)
	}

	// Audit goes to stderr, which is CloudWatch Logs here. There is no durable
	// filesystem to write to and no reason to invent one.
	auditLog := audit.To(os.Stderr)

	res, err := trip.Fire(ctx, cfg, pol, awsx.AccountID(ctx, cfg), store, auditLog, trip.Options{
		DryRun:       ev.DryRun,
		Force:        ev.Force,
		Now:          time.Now().UTC(),
		SkipCooldown: false, // never, here: repeated delivery is the normal case
	})
	if err != nil {
		return Response{Trigger: trigger(ev)}, err
	}

	out := Response{
		Fired: res.Fired, DryRun: ev.DryRun,
		Changed: res.Changed, Failed: res.Failed,
		Skipped: res.Skipped, PlanID: res.Plan.ID,
		Trigger: trigger(ev), AccountID: res.Plan.Account,
	}
	for _, e := range res.Discovery {
		out.Warnings = append(out.Warnings, e.Error())
	}
	body, _ := json.Marshal(out)
	log.Printf("killswitch: %s", body)
	return out, nil
}

// loadPolicy reads the policy from the environment.
//
// KILLSWITCH_POLICY carries the whole file inline, which keeps the Lambda to a
// single artifact with no bucket to read at fire time — one fewer thing that
// can be unreachable in the minute it matters. KILLSWITCH_POLICY_PATH is the
// alternative for a policy shipped in the deployment package.
func loadPolicy() (policy.Policy, error) {
	if raw := os.Getenv("KILLSWITCH_POLICY"); strings.TrimSpace(raw) != "" {
		return policy.Parse([]byte(raw))
	}
	path := os.Getenv("KILLSWITCH_POLICY_PATH")
	if path == "" {
		return policy.Policy{}, fmt.Errorf(
			"no policy: set KILLSWITCH_POLICY to the file's contents, or KILLSWITCH_POLICY_PATH to a path in the package")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return policy.Policy{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return policy.Parse(data)
}

// trigger is a short description of what caused this invocation, for the log.
// The SNS message id is the useful part: it is what distinguishes a genuine
// second notification from a retry of the first.
func trigger(ev Event) string {
	if len(ev.Records) == 0 {
		return "direct invocation"
	}
	sns := ev.Records[0].SNS
	subject := sns.Subject
	if subject == "" {
		subject = "budgets notification"
	}
	return subject + " (" + sns.MessageID + ")"
}
