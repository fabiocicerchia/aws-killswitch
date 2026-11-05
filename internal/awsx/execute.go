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

// Apply makes one change. Every kind handled here has a matching case in
// Restore -- an apply with no undo is a change this tool does not make.
func (e *Executor) Apply(ctx context.Context, a model.Action) error {
	c, err := e.client(a.Resource.Region)
	if err != nil {
		return err
	}
	r := a.Resource
	switch r.Kind {
	case model.KindALBListener:
		return blockListener(ctx, c, r.ID)
	case model.KindLambda:
		return throttleLambda(ctx, c, r.ID)
	case model.KindECSService:
		return scaleService(ctx, c, r.Prior, 0)
	case model.KindASG:
		return scaleASG(ctx, c, r.ID, 0, 0, 0)
	case model.KindEC2Instance:
		return stopInstance(ctx, c, r.ID)
	case model.KindEKSNodegroup:
		return scaleNodegroup(ctx, c, r.Prior, 0, 0)
	case model.KindCloudFront:
		return setDistributionEnabled(ctx, c, r.ID, false)
	case model.KindAPIGatewayStage:
		// 0 rps, 0 burst. Both are needed: a burst allowance with a zero rate
		// still lets a burst through.
		return throttleStage(ctx, c, r.Prior, 0, 0)
	case model.KindNATGateway:
		return deleteNATGateway(ctx, c, r.ID)
	case model.KindRDSInstance:
		return stopDBInstance(ctx, c, r.ID, e.FinalSnapshot)
	case model.KindRDSCluster:
		return stopDBCluster(ctx, c, r.ID)
	}
	return fmt.Errorf("no apply implemented for %s", r.Kind)
}

// Restore undoes one change from the recorded prior state, not from what
// the account looks like now: the account is the thing being repaired.
func (e *Executor) Restore(ctx context.Context, en model.Entry) error {
	c, err := e.client(en.Region)
	if err != nil {
		return err
	}
	switch en.Kind {
	case model.KindALBListener:
		return restoreListener(ctx, c, en)
	case model.KindLambda:
		return restoreLambda(ctx, c, en)
	case model.KindECSService:
		return restoreService(ctx, c, en)
	case model.KindASG:
		return restoreASG(ctx, c, en)
	case model.KindEC2Instance:
		return startInstance(ctx, c, en.ID)
	case model.KindEKSNodegroup:
		return restoreNodegroup(ctx, c, en)
	case model.KindCloudFront:
		return restoreDistribution(ctx, c, en)
	case model.KindAPIGatewayStage:
		return restoreStage(ctx, c, en)
	case model.KindNATGateway:
		return restoreNATGateway(ctx, c, en)
	case model.KindRDSInstance:
		return startDBInstance(ctx, c, en.ID)
	case model.KindRDSCluster:
		return startDBCluster(ctx, c, en.ID)
	}
	return fmt.Errorf("no restore implemented for %s", en.Kind)
}
