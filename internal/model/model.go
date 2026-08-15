// Package model is the vocabulary: what can be stopped, what stopping it means,
// and — the part that matters most — what must never be touched.
//
// This tool exists to be fired at two in the morning by someone who has just
// seen a bill. Everything here is shaped by two rules that follow from that:
//
//	Stop, never delete. A stopped thing costs nothing and comes back. A deleted
//	thing may not, and nobody deletes carefully at two in the morning.
//
//	The restore is the product. Stopping an account is easy and useless on its
//	own. What makes this safe to fire is that the exact prior state is recorded
//	durably first, so putting it back is mechanical rather than archaeological.
package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Kind string

const (
	KindALBListener Kind = "alb-listener"
	KindLambda      Kind = "lambda"
	KindECSService  Kind = "ecs-service"
	KindASG         Kind = "asg"
	KindEC2Instance Kind = "ec2-instance"
	KindNATGateway  Kind = "nat-gateway"
	KindRDSInstance Kind = "rds-instance"
	KindRDSCluster  Kind = "rds-cluster"
)

// Phase is the order things happen in, and the order is not cosmetic.
//
// Ingress goes first. Drain the compute while traffic is still arriving and a
// target-tracking scaling policy will fight the teardown, health checks will
// alarm, and whatever is driving the spend keeps driving it against a shrinking
// fleet. Cut the traffic and the rest goes quietly.
type Phase int

const (
	PhaseIngress Phase = iota
	PhaseCompute
	PhaseNetwork
	PhaseData
)

func (p Phase) String() string {
	switch p {
	case PhaseIngress:
		return "ingress"
	case PhaseCompute:
		return "compute"
	case PhaseNetwork:
		return "network"
	case PhaseData:
		return "data"
	}
	return "unknown"
}

func (p Phase) Why() string {
	switch p {
	case PhaseIngress:
		return "stop the traffic that drives the spend, before draining what serves it"
	case PhaseCompute:
		return "drain compute; EBS is kept, nothing is deleted"
	case PhaseNetwork:
		return "hourly-billed plumbing that holds no state"
	case PhaseData:
		return "databases — opt-in, and reversible only on a deadline"
	}
	return ""
}

// Resource is one thing found in the account.
type Resource struct {
	ID     string
	ARN    string
	Kind   Kind
	Name   string
	Region string
	Tags   map[string]string

	// Prior is the state that must be restored, captured at discovery. Kept as
	// a map so a new resource kind does not need a new struct threaded through
	// the state file, and so an unrecognised field survives a version change
	// rather than being silently dropped on restore.
	Prior map[string]any

	// HasInstanceStore marks an instance whose local NVMe is erased by a stop.
	// EBS survives; instance store does not, and the API gives no warning.
	HasInstanceStore bool

	// EstimatedHourlyUSD is what stopping it saves, where it can be known
	// cheaply. Zero means unknown, not free.
	EstimatedHourlyUSD float64
}

func (r Resource) Ref() string {
	if r.Name != "" && r.Name != r.ID {
		return fmt.Sprintf("%s (%s)", r.Name, r.ID)
	}
	return r.ID
}

// Action is one reversible change.
type Action struct {
	Resource Resource
	Phase    Phase
	// Op describes the change in the words a person would use, because it is
	// printed in the confirmation prompt and read under pressure.
	Op string
	// Warning is printed in red and requires acknowledgement. Empty for the
	// ordinary case.
	Warning string
}

// Refusal is a resource the planner deliberately left alone. These are as
// important as the actions: a plan that silently omits things is a plan nobody
// can check, and "why is that still running" is the first question asked.
type Refusal struct {
	Resource Resource
	Reason   string
}

// Plan is what will happen, in order, with everything that will not.
type Plan struct {
	ID        string
	CreatedAt time.Time
	Account   string
	Regions   []string
	Actions   []Action
	Refusals  []Refusal
	// AckRequired lists warnings that must be acknowledged before firing.
	AckRequired []string
}

// ByPhase returns the actions in execution order, grouped.
func (p Plan) ByPhase() [][]Action {
	groups := make([][]Action, 4)
	for _, a := range p.Actions {
		if int(a.Phase) < len(groups) {
			groups[a.Phase] = append(groups[a.Phase], a)
		}
	}
	for i := range groups {
		sort.SliceStable(groups[i], func(x, y int) bool {
			return groups[i][x].Resource.Ref() < groups[i][y].Resource.Ref()
		})
	}
	return groups
}

// BlastRadius is how much of the account this touches — the number a person
// checks before typing yes.
type BlastRadius struct {
	Total     int
	ByKind    map[Kind]int
	TouchesDB bool
	DataLoss  bool // an instance-store instance is in the plan

	// SavingsUSD is the hourly total across resources that have an estimate.
	SavingsUSD float64
	// SavingsByKind breaks that down, so a plan dominated by one kind is
	// visible rather than hidden inside a single number.
	SavingsByKind map[Kind]float64
	// UnpricedByKind counts resources with no defensible estimate. Reported
	// separately and never as zero: a zero saving is a claim that stopping the
	// thing is free, which is a different statement from not knowing.
	UnpricedByKind map[Kind]int
}

func (p Plan) BlastRadius() BlastRadius {
	b := BlastRadius{
		ByKind:         map[Kind]int{},
		SavingsByKind:  map[Kind]float64{},
		UnpricedByKind: map[Kind]int{},
	}
	for _, a := range p.Actions {
		b.Total++
		b.ByKind[a.Resource.Kind]++
		if a.Resource.EstimatedHourlyUSD > 0 {
			b.SavingsUSD += a.Resource.EstimatedHourlyUSD
			b.SavingsByKind[a.Resource.Kind] += a.Resource.EstimatedHourlyUSD
		} else {
			b.UnpricedByKind[a.Resource.Kind]++
		}
		if a.Phase == PhaseData {
			b.TouchesDB = true
		}
		if a.Resource.HasInstanceStore {
			b.DataLoss = true
		}
	}
	return b
}

func (p Plan) IsEmpty() bool { return len(p.Actions) == 0 }

// --- what is never touched ---------------------------------------------------

// NeverTouch is the set of things this tool will not act on under any flag.
//
// It is a deny list rather than an allow list on purpose. An allow list grows
// by accident: someone adds a resource kind, forgets the exclusion, and a cost
// tool deletes a bucket. These are the things whose loss is unrecoverable, so
// the rule is absolute and there is no option to override it.
var NeverTouch = []string{
	"s3", "efs", "fsx", "dynamodb", "glacier", "backup",
	"ebs-volume", "snapshot", "ami", "route53-zone", "iam", "kms",
	"cloudtrail", "config", "secretsmanager", "ssm-parameter",
}

// IsNeverTouch reports whether a service or resource identifier names something
// in the protected set. Matched on the ARN service field and on the free-form
// kind, so it catches both "arn:aws:s3:::bucket" and a hand-built resource.
func IsNeverTouch(s string) bool {
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "arn:") {
		parts := strings.SplitN(low, ":", 4)
		if len(parts) >= 3 {
			low = parts[2]
		}
	}
	for _, n := range NeverTouch {
		if low == n || strings.HasPrefix(low, n+"-") || strings.HasPrefix(low, n+":") {
			return true
		}
	}
	return false
}

// --- restore -----------------------------------------------------------------

// Snapshot is what makes the whole thing safe: the exact prior state of every
// resource the plan will touch, written durably *before* anything is changed.
//
// Without it, restore is someone guessing what the desired count used to be.
type Snapshot struct {
	PlanID    string     `json:"plan_id"`
	CreatedAt time.Time  `json:"created_at"`
	Account   string     `json:"account"`
	Regions   []string   `json:"regions"`
	Entries   []Entry    `json:"entries"`
	Fired     *time.Time `json:"fired_at,omitempty"`
	Restored  *time.Time `json:"restored_at,omitempty"`
}

type Entry struct {
	Kind   Kind           `json:"kind"`
	ID     string         `json:"id"`
	ARN    string         `json:"arn,omitempty"`
	Name   string         `json:"name,omitempty"`
	Region string         `json:"region"`
	Phase  Phase          `json:"phase"`
	Prior  map[string]any `json:"prior"`
	// Result records what actually happened, so a partial fire can be restored
	// without re-running against resources that were never changed.
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

const (
	ResultChanged   = "changed"
	ResultSkipped   = "skipped"
	ResultFailed    = "failed"
	ResultUnchanged = "unchanged"
)

// RDSAutoRestart is the trap that makes stopping a database different from
// stopping anything else: AWS restarts a stopped RDS instance by itself after
// seven days. A kill switch that silently un-kills a week later is worse than
// no kill switch, because nobody is watching by then.
const RDSAutoRestart = 7 * 24 * time.Hour

// RestoreDeadline is when a stopped database will come back on its own, or the
// zero time when the entry is not a database.
func (e Entry) RestoreDeadline(firedAt time.Time) (time.Time, bool) {
	if e.Kind != KindRDSInstance && e.Kind != KindRDSCluster {
		return time.Time{}, false
	}
	return firedAt.Add(RDSAutoRestart), true
}

// Changed returns the entries a restore has to act on.
func (s Snapshot) Changed() []Entry {
	var out []Entry
	for _, e := range s.Entries {
		if e.Result == ResultChanged {
			out = append(out, e)
		}
	}
	return out
}

// PriorInt reads an integer out of the recorded prior state.
//
// JSON round-trips numbers as float64, so a desired count of 3 comes back as
// 3.0 and a naive type assertion to int fails — silently restoring zero, which
// would leave the account down after a "successful" restore.
func PriorInt(prior map[string]any, key string) (int, bool) {
	v, ok := prior[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	}
	return 0, false
}

func PriorString(prior map[string]any, key string) (string, bool) {
	v, ok := prior[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
