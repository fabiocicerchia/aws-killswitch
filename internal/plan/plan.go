// Package plan turns a pile of discovered resources into an ordered, reversible
// sequence of changes — and, just as importantly, a list of everything it will
// not touch and why.
//
// Pure: no AWS, no clock beyond what is passed in, no filesystem. That is what
// makes the safety rules testable, and these are rules nobody wants to discover
// are wrong by firing them.
package plan

import (
	"fmt"
	"sort"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
)

type Input struct {
	Account   string
	Regions   []string
	Resources []model.Resource
	Now       time.Time
	PlanID    string
}

// Build applies the policy to the inventory. It never returns an error: an
// inventory that yields no actions is a valid, empty plan, and the refusals
// explain it.
func Build(in Input, p policy.Policy) model.Plan {
	out := model.Plan{
		ID: in.PlanID, CreatedAt: in.Now,
		Account: in.Account, Regions: in.Regions,
	}

	for _, r := range in.Resources {
		// The absolute rule first, before scope, before anything. A resource
		// whose loss is unrecoverable is not eligible however it is tagged and
		// whatever flags were passed.
		if model.IsNeverTouch(string(r.Kind)) || model.IsNeverTouch(r.ARN) {
			out.Refusals = append(out.Refusals, model.Refusal{
				Resource: r, Reason: "protected kind: this tool never touches stateful storage",
			})
			continue
		}
		if ok, why := p.InScope(r.Tags); !ok {
			out.Refusals = append(out.Refusals, model.Refusal{Resource: r, Reason: why})
			continue
		}

		act, refusal, ok := actionFor(r, p)
		if !ok {
			out.Refusals = append(out.Refusals, refusal)
			continue
		}
		out.Actions = append(out.Actions, act)
	}

	sortPlan(&out)
	out.AckRequired = acknowledgements(out, p)
	return out
}

// refuse is the other half of actionFor's answer: leave this resource alone,
// and record why so the plan can print it.
func refuse(r model.Resource, reason string) (model.Action, model.Refusal, bool) {
	return model.Action{}, model.Refusal{Resource: r, Reason: reason}, false
}

// actionFor decides what stopping one resource means, or refuses it.
func actionFor(r model.Resource, p policy.Policy) (model.Action, model.Refusal, bool) {
	switch r.Kind {
	case model.KindALBListener:
		return model.Action{
			Resource: r, Phase: model.PhaseIngress,
			Op: "return a fixed 503 instead of forwarding to the target group",
		}, model.Refusal{}, true

	case model.KindLambda:
		// The cleanest stop in AWS: instant, exact, and there is nothing to
		// lose. In-flight invocations finish; new ones are throttled.
		return model.Action{
			Resource: r, Phase: model.PhaseCompute,
			Op: "set reserved concurrency to 0",
		}, model.Refusal{}, true

	case model.KindECSService:
		if alreadyZero(r.Prior, "desired_count") {
			return refuse(r, "already at zero")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseCompute,
			Op: "set desired count to 0",
		}, model.Refusal{}, true

	case model.KindASG:
		if alreadyZero(r.Prior, "desired_capacity") {
			return refuse(r, "already at zero")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseCompute,
			Op: "set min/desired/max to 0",
		}, model.Refusal{}, true

	case model.KindEC2Instance:
		return instanceAction(r, p)

	case model.KindEKSNodegroup:
		if n, ok := model.PriorInt(r.Prior, "desired_size"); ok && n == 0 {
			return refuse(r, "already at zero")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseCompute,
			// maxSize is left alone: EKS refuses a nodegroup whose max is 0, and
			// scaling to zero only needs min and desired. Restoring a value the
			// API would have rejected is not a restore.
			Op: "set min/desired node count to 0",
		}, model.Refusal{}, true

	case model.KindCloudFront:
		if on, ok := model.PriorBool(r.Prior, "enabled"); ok && !on {
			return refuse(r, "already disabled")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseIngress,
			Op: "disable the distribution",
			// Worth saying out loud: a disable is not instant, and someone
			// watching the bill needs to know why nothing changed for a while.
			Warning: r.Ref() + ": CloudFront takes minutes to propagate a disable to every edge, and returns errors to users while it does",
		}, model.Refusal{}, true

	case model.KindAPIGatewayStage:
		if rate, ok := model.PriorFloat(r.Prior, "rate_limit"); ok && rate == 0 {
			return refuse(r, "already throttled to zero")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseIngress,
			Op:      "throttle the stage to 0 requests/second",
			Warning: r.Ref() + ": callers get 429, not a connection error — a client with retries will keep trying",
		}, model.Refusal{}, true

	case model.KindNATGateway:
		if !p.DeleteNATGateways {
			return refuse(r, "NAT gateways can only be deleted, not stopped; set delete_nat_gateways to include them")
		}
		return model.Action{
			Resource: r, Phase: model.PhaseNetwork,
			Op:      "delete the NAT gateway",
			Warning: r.Ref() + ": deletion is not a stop — restore recreates it with a new address and repoints route tables",
		}, model.Refusal{}, true

	case model.KindRDSInstance, model.KindRDSCluster:
		return databaseAction(r, p)
	}

	return refuse(r, "unsupported kind "+string(r.Kind))
}

// alreadyZero reports that a resource is already at the capacity the plan would
// set it to. ECS and ASG both need this and have to agree on it, or one of them
// proposes a change that does nothing and pads the blast radius a person is
// about to read under pressure.
func alreadyZero(prior map[string]any, key string) bool {
	n, ok := model.PriorInt(prior, key)
	return ok && n == 0
}

// instanceAction decides a loose EC2 instance. The instance-store refusal is
// the reason this is its own function: stopping such an instance erases its
// local NVMe, the API gives no warning, so it is refused unless the policy has
// accepted that loss in writing — and even then it needs acknowledgement.
func instanceAction(r model.Resource, p policy.Policy) (model.Action, model.Refusal, bool) {
	if s, ok := model.PriorString(r.Prior, "state"); ok && s != "running" {
		return refuse(r, "not running ("+s+")")
	}
	if r.HasInstanceStore && !p.AllowInstanceStoreLoss {
		// The API stops it happily and says nothing. Refusing by default is
		// the only way this does not eventually erase someone's scratch
		// data during a cost incident.
		return refuse(r, "has instance-store volumes, which a stop erases; set allow_instance_store_loss to accept that")
	}
	act := model.Action{
		Resource: r, Phase: model.PhaseCompute,
		Op: "stop the instance (EBS is kept)",
	}
	if r.HasInstanceStore {
		act.Warning = fmt.Sprintf("%s: instance-store data will be lost permanently", r.Ref())
	}
	return act, model.Refusal{}, true
}

// databaseAction decides an RDS instance or cluster. Databases are out unless
// the policy opts in, and even then every one carries the seven-day warning:
// AWS restarts a stopped database by itself, so a kill switch nobody is
// watching un-kills a week later.
func databaseAction(r model.Resource, p policy.Policy) (model.Action, model.Refusal, bool) {
	if !p.IncludeDatabases {
		return refuse(r, "database: excluded unless include_databases is set")
	}
	if s, ok := model.PriorString(r.Prior, "status"); ok && s != "available" {
		return refuse(r, "not available ("+s+")")
	}
	op := "stop the database"
	if p.FinalSnapshot {
		op = "snapshot, then stop the database"
	}
	return model.Action{
		Resource: r, Phase: model.PhaseData, Op: op,
		Warning: fmt.Sprintf("%s: AWS restarts a stopped database by itself after 7 days", r.Ref()),
	}, model.Refusal{}, true
}

// sortPlan puts the actions in the order they will run: by phase, then by kind
// within a phase, then by name so two runs of the same plan read identically.
//
// Kind order inside compute is not arbitrary either. Lambda first because it is
// instantaneous and gives immediate relief; ASGs before loose instances because
// an ASG will otherwise replace an instance stopped underneath it.
func sortPlan(p *model.Plan) {
	rank := map[model.Kind]int{
		// Ingress. CloudFront first: it is the outermost edge and the slowest to
		// take effect, so starting it early means it has propagated by the time
		// the rest is done. API Gateway before the ALB for the same reason it
		// sits in front of one.
		model.KindCloudFront:      0,
		model.KindAPIGatewayStage: 1,
		model.KindALBListener:     2,
		// Compute. Lambda first because it is instantaneous. Then the two
		// managed groups before loose instances, because a group replaces an
		// instance stopped underneath it — an EKS nodegroup does this exactly as
		// an ASG does, and for the same reason.
		model.KindLambda:       3,
		model.KindECSService:   4,
		model.KindEKSNodegroup: 5,
		model.KindASG:          6,
		model.KindEC2Instance:  7,
		model.KindNATGateway:   8,
		model.KindRDSCluster:   9,
		model.KindRDSInstance:  10,
	}
	sort.SliceStable(p.Actions, func(i, j int) bool {
		a, b := p.Actions[i], p.Actions[j]
		if a.Phase != b.Phase {
			return a.Phase < b.Phase
		}
		if rank[a.Resource.Kind] != rank[b.Resource.Kind] {
			return rank[a.Resource.Kind] < rank[b.Resource.Kind]
		}
		return a.Resource.Ref() < b.Resource.Ref()
	})
	sort.SliceStable(p.Refusals, func(i, j int) bool {
		return p.Refusals[i].Resource.Ref() < p.Refusals[j].Resource.Ref()
	})
}

// acknowledgements collects everything the operator must confirm before this
// runs. Presented as a list rather than buried per-resource, because the point
// is that they are read.
func acknowledgements(p model.Plan, pol policy.Policy) []string {
	var acks []string
	for _, a := range p.Actions {
		if a.Warning != "" {
			acks = append(acks, a.Warning)
		}
	}
	b := p.BlastRadius()
	if b.Total > pol.Threshold() {
		acks = append(acks, fmt.Sprintf(
			"this plan changes %d resources, over the confirm_above threshold of %d", b.Total, pol.Threshold()))
	}
	return acks
}

// RestoreOrder reverses the plan: bring back what serves traffic before letting
// traffic in, or the first request arrives at nothing and the restore looks
// like a failure.
func RestoreOrder(entries []model.Entry) []model.Entry {
	out := append([]model.Entry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Phase != out[j].Phase {
			return out[i].Phase > out[j].Phase
		}
		return out[i].ID < out[j].ID
	})
	return out
}
