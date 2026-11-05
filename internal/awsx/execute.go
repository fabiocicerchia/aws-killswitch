package awsx

import (
	"context"
	"errors"
	"fmt"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// Executor applies and undoes changes. Every Apply has a matching Restore, and
// neither ever deletes anything except a NAT gateway, which cannot be stopped.
type Executor struct {
	byRegion      map[string]*Clients
	FinalSnapshot bool
}

// NewExecutor takes the clients keyed by region. A resource in a region
// that is not in the map fails loudly rather than being skipped.
func NewExecutor(clients map[string]*Clients) *Executor {
	return &Executor{byRegion: clients}
}

func (e *Executor) client(region string) (*Clients, error) {
	// CloudFront has no region, so its resources are recorded as "global" and
	// there is no entry under that key. Route them to whichever client was
	// given the global service, rather than failing on a region that was never
	// meant to be looked up.
	if region == model.GlobalRegion {
		for _, c := range e.byRegion {
			if c.CloudFront != nil {
				return c, nil
			}
		}
		return nil, errors.New("no client configured for the global services (CloudFront)")
	}
	c, ok := e.byRegion[region]
	if !ok {
		return nil, fmt.Errorf("no client configured for region %s", region)
	}
	return c, nil
}

// Apply makes one change. Every kind in appliers has a matching entry in
// restorers -- an apply with no undo is a change this tool does not make.
func (e *Executor) Apply(ctx context.Context, a model.Action) error {
	c, err := e.client(a.Resource.Region)
	if err != nil {
		return err
	}
	apply, ok := appliers[a.Resource.Kind]
	if !ok {
		return fmt.Errorf("no apply implemented for %s", a.Resource.Kind)
	}
	return apply(ctx, c, a.Resource, e.FinalSnapshot)
}

// Restore undoes one change from the recorded prior state, not from what
// the account looks like now: the account is the thing being repaired.
func (e *Executor) Restore(ctx context.Context, en model.Entry) error {
	c, err := e.client(en.Region)
	if err != nil {
		return err
	}
	restore, ok := restorers[en.Kind]
	if !ok {
		return fmt.Errorf("no restore implemented for %s", en.Kind)
	}
	return restore(ctx, c, en)
}

type applyFunc func(ctx context.Context, c *Clients, r model.Resource, finalSnapshot bool) error

var appliers = map[model.Kind]applyFunc{
	model.KindALBListener: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return blockListener(ctx, c, r.ID)
	},
	model.KindLambda: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return throttleLambda(ctx, c, r.ID)
	},
	model.KindECSService: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return scaleService(ctx, c, r.Prior, 0)
	},
	model.KindASG: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return scaleASG(ctx, c, r.ID, 0, 0, 0)
	},
	model.KindEC2Instance: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return stopInstance(ctx, c, r.ID)
	},
	model.KindEKSNodegroup: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return scaleNodegroup(ctx, c, r.Prior, 0, 0)
	},
	model.KindCloudFront: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return setDistributionEnabled(ctx, c, r.ID, false)
	},
	model.KindAPIGatewayStage: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		// 0 rps, 0 burst. Both are needed: a burst allowance with a zero rate
		// still lets a burst through.
		return throttleStage(ctx, c, r.Prior, 0, 0)
	},
	model.KindNATGateway: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return deleteNATGateway(ctx, c, r.ID)
	},
	model.KindRDSInstance: func(ctx context.Context, c *Clients, r model.Resource, finalSnapshot bool) error {
		return stopDBInstance(ctx, c, r.ID, finalSnapshot)
	},
	model.KindRDSCluster: func(ctx context.Context, c *Clients, r model.Resource, _ bool) error {
		return stopDBCluster(ctx, c, r.ID)
	},
}

var restorers = map[model.Kind]func(ctx context.Context, c *Clients, en model.Entry) error{
	model.KindALBListener: restoreListener,
	model.KindLambda:      restoreLambda,
	model.KindECSService:  restoreService,
	model.KindASG:         restoreASG,
	model.KindEC2Instance: func(ctx context.Context, c *Clients, en model.Entry) error {
		return startInstance(ctx, c, en.ID)
	},
	model.KindEKSNodegroup:    restoreNodegroup,
	model.KindCloudFront:      restoreDistribution,
	model.KindAPIGatewayStage: restoreStage,
	model.KindNATGateway:      restoreNATGateway,
	model.KindRDSInstance: func(ctx context.Context, c *Clients, en model.Entry) error {
		return startDBInstance(ctx, c, en.ID)
	},
	model.KindRDSCluster: func(ctx context.Context, c *Clients, en model.Entry) error {
		return startDBCluster(ctx, c, en.ID)
	},
}
