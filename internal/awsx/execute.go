package awsx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// Executor applies and undoes changes. Every Apply has a matching Restore, and
// neither ever deletes anything except a NAT gateway, which cannot be stopped.
type Executor struct {
	byRegion      map[string]*Clients
	FinalSnapshot bool
}

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
		_, err := c.EC2.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{r.ID}})
		return err
	case model.KindEKSNodegroup:
		return scaleNodegroup(ctx, c, r.Prior, 0, 0)
	case model.KindCloudFront:
		return setDistributionEnabled(ctx, c, r.ID, false)
	case model.KindAPIGatewayStage:
		// 0 rps, 0 burst. Both are needed: a burst allowance with a zero rate
		// still lets a burst through.
		return throttleStage(ctx, c, r.Prior, 0, 0)
	case model.KindNATGateway:
		_, err := c.EC2.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(r.ID)})
		return err
	case model.KindRDSInstance:
		in := &rds.StopDBInstanceInput{DBInstanceIdentifier: aws.String(r.ID)}
		if e.FinalSnapshot {
			in.DBSnapshotIdentifier = aws.String(snapshotName(r.ID))
		}
		_, err := c.RDS.StopDBInstance(ctx, in)
		return err
	case model.KindRDSCluster:
		_, err := c.RDS.StopDBCluster(ctx, &rds.StopDBClusterInput{DBClusterIdentifier: aws.String(r.ID)})
		return err
	}
	return fmt.Errorf("no apply implemented for %s", r.Kind)
}

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
		n, ok := model.PriorInt32(en.Prior, "desired_count")
		if !ok {
			return errors.New("no usable recorded desired count; refusing to guess")
		}
		return scaleService(ctx, c, en.Prior, n)
	case model.KindASG:
		min, okMin := model.PriorInt32(en.Prior, "min_size")
		des, okDes := model.PriorInt32(en.Prior, "desired_capacity")
		max, okMax := model.PriorInt32(en.Prior, "max_size")
		if !okMin || !okDes || !okMax {
			return errors.New("incomplete or unusable recorded capacity; refusing to guess")
		}
		return scaleASG(ctx, c, en.ID, min, des, max)
	case model.KindEC2Instance:
		_, err := c.EC2.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{en.ID}})
		return err
	case model.KindEKSNodegroup:
		min, okMin := model.PriorInt32(en.Prior, "min_size")
		des, okDes := model.PriorInt32(en.Prior, "desired_size")
		if !okMin || !okDes {
			return errors.New("incomplete or unusable recorded node counts; refusing to guess")
		}
		return scaleNodegroup(ctx, c, en.Prior, min, des)
	case model.KindCloudFront:
		on, ok := model.PriorBool(en.Prior, "enabled")
		if !ok {
			return errors.New("no recorded enabled state; refusing to guess")
		}
		if !on {
			// It was already disabled when we found it, so the planner refused
			// it and nothing was done. Re-disabling would be harmless but the
			// state file should never have carried it here.
			return nil
		}
		return setDistributionEnabled(ctx, c, en.ID, true)
	case model.KindAPIGatewayStage:
		rate, okRate := model.PriorFloat(en.Prior, "rate_limit")
		burst, okBurst := model.PriorInt(en.Prior, "burst_limit")
		if !okRate || !okBurst {
			return errors.New("no usable recorded throttle; refusing to guess")
		}
		return throttleStage(ctx, c, en.Prior, rate, burst)
	case model.KindNATGateway:
		return restoreNATGateway(ctx, c, en)
	case model.KindRDSInstance:
		_, err := c.RDS.StartDBInstance(ctx, &rds.StartDBInstanceInput{DBInstanceIdentifier: aws.String(en.ID)})
		return err
	case model.KindRDSCluster:
		_, err := c.RDS.StartDBCluster(ctx, &rds.StartDBClusterInput{DBClusterIdentifier: aws.String(en.ID)})
		return err
	}
	return fmt.Errorf("no restore implemented for %s", en.Kind)
}

// --- ingress -----------------------------------------------------------------

// blockListener swaps the default action for a fixed 503. Instant, reversible,
// and it leaves the target group and everything behind it untouched.
func blockListener(ctx context.Context, c *Clients, arn string) error {
	_, err := c.ELB.ModifyListener(ctx, &elasticloadbalancingv2.ModifyListenerInput{
		ListenerArn: aws.String(arn),
		DefaultActions: []elbtypes.Action{{
			Type: elbtypes.ActionTypeEnumFixedResponse,
			FixedResponseConfig: &elbtypes.FixedResponseActionConfig{
				StatusCode:  aws.String("503"),
				ContentType: aws.String("text/plain"),
				MessageBody: aws.String("Service temporarily unavailable (cost control)"),
			},
		}},
	})
	return err
}

func restoreListener(ctx context.Context, c *Clients, en model.Entry) error {
	raw, ok := en.Prior["default_actions"]
	if !ok {
		return errors.New("no recorded default actions; restoring would guess at the routing")
	}
	actions, err := decodeActions(raw)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		return errors.New("recorded default actions are empty; refusing to leave the listener with no route")
	}
	_, err = c.ELB.ModifyListener(ctx, &elasticloadbalancingv2.ModifyListenerInput{
		ListenerArn: aws.String(en.ID), DefaultActions: actions,
	})
	return err
}

// The listener's default actions are stored as JSON rather than as a bespoke
// struct, so a field this tool does not know about still survives the round
// trip and comes back on restore.
func encodeActions(actions []elbtypes.Action) any {
	b, err := json.Marshal(actions)
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil
	}
	return generic
}

func decodeActions(raw any) ([]elbtypes.Action, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var actions []elbtypes.Action
	if err := json.Unmarshal(b, &actions); err != nil {
		return nil, fmt.Errorf("recorded listener actions are unreadable: %w", err)
	}
	return actions, nil
}

// --- compute -----------------------------------------------------------------

func throttleLambda(ctx context.Context, c *Clients, name string) error {
	_, err := c.Lambda.PutFunctionConcurrency(ctx, &lambda.PutFunctionConcurrencyInput{
		FunctionName: aws.String(name), ReservedConcurrentExecutions: aws.Int32(0),
	})
	return err
}

// restoreLambda has two different undos, and picking the wrong one silently
// changes the function's behaviour: a function that had no reservation must
// have the reservation *removed*, not set back to some number.
func restoreLambda(ctx context.Context, c *Clients, en model.Entry) error {
	had, _ := en.Prior["had_reservation"].(bool)
	if !had {
		_, err := c.Lambda.DeleteFunctionConcurrency(ctx, &lambda.DeleteFunctionConcurrencyInput{
			FunctionName: aws.String(en.ID),
		})
		return err
	}
	n, ok := model.PriorInt32(en.Prior, "reserved_concurrency")
	if !ok {
		return errors.New("recorded a reservation but not a usable value; refusing to guess")
	}
	_, err := c.Lambda.PutFunctionConcurrency(ctx, &lambda.PutFunctionConcurrencyInput{
		FunctionName: aws.String(en.ID), ReservedConcurrentExecutions: aws.Int32(n),
	})
	return err
}

func scaleService(ctx context.Context, c *Clients, prior map[string]any, count int32) error {
	cluster, ok := model.PriorString(prior, "cluster")
	if !ok {
		return errors.New("no recorded cluster for this service")
	}
	name, ok := model.PriorString(prior, "service")
	if !ok {
		return errors.New("no recorded service name")
	}
	_, err := c.ECS.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(name), DesiredCount: aws.Int32(count),
	})
	return err
}

// scaleASG sets all three bounds. Order matters: min must never exceed max, and
// AWS rejects the update rather than reconciling, so going down sets max last
// and coming back sets max first.
func scaleASG(ctx context.Context, c *Clients, name string, min, desired, max int32) error {
	in := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(name),
		MinSize:              aws.Int32(min),
		DesiredCapacity:      aws.Int32(desired),
		MaxSize:              aws.Int32(max),
	}
	_, err := c.ASG.UpdateAutoScalingGroup(ctx, in)
	return err
}

// --- network -----------------------------------------------------------------

// restoreNATGateway recreates the gateway and repoints every route that used
// it. Recreating alone would leave private subnets with no egress and a plan
// that reported success.
func restoreNATGateway(ctx context.Context, c *Clients, en model.Entry) error {
	subnet, ok := model.PriorString(en.Prior, "subnet_id")
	if !ok {
		return errors.New("no recorded subnet; cannot recreate")
	}
	in := &ec2.CreateNatGatewayInput{SubnetId: aws.String(subnet)}
	if alloc, ok := model.PriorString(en.Prior, "allocation_id"); ok && alloc != "" {
		// The Elastic IP was disassociated rather than released, so reusing the
		// allocation gives back the same public address.
		in.AllocationId = aws.String(alloc)
	}
	if ct, ok := model.PriorString(en.Prior, "connectivity_type"); ok && ct != "" {
		in.ConnectivityType = ec2types.ConnectivityType(ct)
	}
	created, err := c.EC2.CreateNatGateway(ctx, in)
	if err != nil {
		return err
	}
	newID := aws.ToString(created.NatGateway.NatGatewayId)

	waiter := ec2.NewNatGatewayAvailableWaiter(c.EC2)
	if err := waiter.Wait(ctx, &ec2.DescribeNatGatewaysInput{NatGatewayIds: []string{newID}}, 10*time.Minute); err != nil {
		return fmt.Errorf("created %s but it did not become available: %w", newID, err)
	}

	routes, _ := en.Prior["routes"].([]any)
	for _, raw := range routes {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rtb, _ := r["route_table_id"].(string)
		dst, _ := r["destination"].(string)
		if rtb == "" || dst == "" {
			continue
		}
		if _, err := c.EC2.ReplaceRoute(ctx, &ec2.ReplaceRouteInput{
			RouteTableId: aws.String(rtb), DestinationCidrBlock: aws.String(dst),
			NatGatewayId: aws.String(newID),
		}); err != nil {
			// The route may have been removed when the gateway went; create it.
			if _, cerr := c.EC2.CreateRoute(ctx, &ec2.CreateRouteInput{
				RouteTableId: aws.String(rtb), DestinationCidrBlock: aws.String(dst),
				NatGatewayId: aws.String(newID),
			}); cerr != nil {
				return fmt.Errorf("recreated %s but could not repoint %s in %s: %w", newID, dst, rtb, cerr)
			}
		}
	}
	return nil
}

func snapshotName(id string) string {
	return fmt.Sprintf("killswitch-%s-%d", id, time.Now().UTC().Unix())
}

// --- EKS, CloudFront, API Gateway --------------------------------------------

// scaleNodegroup writes min and desired on a managed node group.
//
// maxSize is deliberately not touched. EKS rejects a nodegroup whose max is 0,
// so a scale-to-zero that also zeroed max would fail outright; and on restore,
// a max nobody changed needs no putting back.
func scaleNodegroup(ctx context.Context, c *Clients, prior map[string]any, min, desired int32) error {
	cluster, okCluster := model.PriorString(prior, "cluster_name")
	name, okName := model.PriorString(prior, "nodegroup")
	if !okCluster || !okName {
		return errors.New("recorded state names no cluster or nodegroup")
	}
	_, err := c.EKS.UpdateNodegroupConfig(ctx, &eks.UpdateNodegroupConfigInput{
		ClusterName:   aws.String(cluster),
		NodegroupName: aws.String(name),
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			MinSize:     aws.Int32(min),
			DesiredSize: aws.Int32(desired),
		},
	})
	return err
}

// setDistributionEnabled flips a distribution on or off.
//
// CloudFront has no "disable" call: the whole config is read, one field is
// changed, and the config is written back with the ETag that came with it. The
// ETag is re-fetched here rather than recorded at discovery on purpose — it
// changes on every update, and a stale one fails with PreconditionFailed. The
// read is also what makes this safe: everything else in the config goes back
// exactly as it was found, including changes made between fire and restore.
func setDistributionEnabled(ctx context.Context, c *Clients, id string, enabled bool) error {
	if c.CloudFront == nil {
		return errors.New("no CloudFront client on this executor")
	}
	cur, err := c.CloudFront.GetDistributionConfig(ctx,
		&cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
	if err != nil {
		return err
	}
	if cur.DistributionConfig == nil {
		return fmt.Errorf("distribution %s returned no config", id)
	}
	if aws.ToBool(cur.DistributionConfig.Enabled) == enabled {
		return nil // already there; an update would only burn a propagation cycle
	}
	cfg := *cur.DistributionConfig
	cfg.Enabled = aws.Bool(enabled)
	_, err = c.CloudFront.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id: aws.String(id), IfMatch: cur.ETag, DistributionConfig: &cfg,
	})
	return err
}

// throttleStage sets the stage-wide default method throttle.
//
// A rate of -1 means "there was no stage default, so the account limit applied"
// — the state discovery records for a stage that never had one. Restoring that
// removes the override rather than writing a number AWS never had, which is the
// difference between putting it back and pinning it to today's account limit.
func throttleStage(ctx context.Context, c *Clients, prior map[string]any, rate float64, burst int) error {
	apiID, okAPI := model.PriorString(prior, "rest_api_id")
	stage, okStage := model.PriorString(prior, "stage_name")
	if !okAPI || !okStage {
		return errors.New("recorded state names no API or stage")
	}

	var ops []apigwtypes.PatchOperation
	if rate < 0 || burst < 0 {
		ops = []apigwtypes.PatchOperation{
			{Op: apigwtypes.OpRemove, Path: aws.String("/*/*/throttling/rateLimit")},
			{Op: apigwtypes.OpRemove, Path: aws.String("/*/*/throttling/burstLimit")},
		}
	} else {
		ops = []apigwtypes.PatchOperation{
			{
				Op:    apigwtypes.OpReplace,
				Path:  aws.String("/*/*/throttling/rateLimit"),
				Value: aws.String(strconv.FormatFloat(rate, 'f', -1, 64)),
			},
			{
				Op:    apigwtypes.OpReplace,
				Path:  aws.String("/*/*/throttling/burstLimit"),
				Value: aws.String(strconv.Itoa(burst)),
			},
		}
	}
	_, err := c.APIGateway.UpdateStage(ctx, &apigateway.UpdateStageInput{
		RestApiId: aws.String(apiID), StageName: aws.String(stage), PatchOperations: ops,
	})
	return err
}
