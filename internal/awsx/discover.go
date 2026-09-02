// Package awsx is everything that talks to AWS: finding what is running, and
// changing it.
//
// Discovery records the prior state of each resource at the moment it is found,
// because that record is what makes the change reversible. It is deliberately
// read-only — nothing in this file mutates anything, so it is safe to run
// against production at any time, and `plan` does exactly that.
package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

type Clients struct {
	Region     string
	EC2        *ec2.Client
	ECS        *ecs.Client
	Lambda     *lambda.Client
	RDS        *rds.Client
	ASG        *autoscaling.Client
	ELB        *elasticloadbalancingv2.Client
	EKS        *eks.Client
	APIGateway *apigateway.Client
	// CloudFront is global, not regional. It is set on exactly one Clients so
	// a multi-region run does not find every distribution once per region and
	// plan the same disable five times.
	CloudFront *cloudfront.Client
}

func NewClients(cfg aws.Config, region string) *Clients {
	c := cfg.Copy()
	if region != "" {
		c.Region = region
	}
	return &Clients{
		Region: c.Region,
		EC2:    ec2.NewFromConfig(c), ECS: ecs.NewFromConfig(c),
		Lambda: lambda.NewFromConfig(c), RDS: rds.NewFromConfig(c),
		ASG: autoscaling.NewFromConfig(c), ELB: elasticloadbalancingv2.NewFromConfig(c),
		EKS: eks.NewFromConfig(c), APIGateway: apigateway.NewFromConfig(c),
	}
}

// WithCloudFront marks this Clients as the one that also walks CloudFront.
//
// CloudFront has no regions. Whoever builds the per-region set calls this on
// exactly one of them; without it, a run across five regions would discover
// every distribution five times and plan five identical disables.
func (c *Clients) WithCloudFront(cfg aws.Config) *Clients {
	g := cfg.Copy()
	// The service is global but the SDK still needs an endpoint region, and
	// us-east-1 is the one CloudFront's control plane answers on.
	g.Region = "us-east-1"
	c.CloudFront = cloudfront.NewFromConfig(g)
	return c
}

// Discover walks the account read-only. Per-service failures are collected
// rather than returned: a missing permission on one service should degrade the
// plan and say so, not leave the operator with nothing during an incident.
func (c *Clients) Discover(ctx context.Context) ([]model.Resource, []error) {
	var all []model.Resource
	var errs []error

	for name, fn := range map[string]func(context.Context) ([]model.Resource, error){
		"elbv2":       c.listeners,
		"lambda":      c.functions,
		"ecs":         c.services,
		"autoscaling": c.autoScalingGroups,
		"ec2":         c.instances,
		"natgateway":  c.natGateways,
		"rds":         c.databases,
		"eks":         c.nodegroups,
		"apigateway":  c.apiStages,
		"cloudfront":  c.distributions,
	} {
		rs, err := fn(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s in %s: %w", name, c.Region, err))
			continue
		}
		all = append(all, rs...)
	}
	return all, errs
}

// --- ingress -----------------------------------------------------------------

// listeners finds load balancer listeners that currently forward traffic.
//
// Only forwarding listeners are candidates: one already returning a fixed
// response is either already blocked or is a redirect, and recording *that* as
// the prior state would restore a block.
func (c *Clients) listeners(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	lbPager := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(c.ELB, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	for lbPager.HasMorePages() {
		page, err := lbPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, lb := range page.LoadBalancers {
			ls, err := c.ELB.DescribeListeners(ctx, &elasticloadbalancingv2.DescribeListenersInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			})
			if err != nil {
				return nil, err
			}
			for _, l := range ls.Listeners {
				if !forwards(l) {
					continue
				}
				tags, err := c.elbTags(ctx, aws.ToString(l.ListenerArn))
				if err != nil {
					return nil, err
				}
				out = append(out, model.Resource{
					ID:     aws.ToString(l.ListenerArn),
					ARN:    aws.ToString(l.ListenerArn),
					Kind:   model.KindALBListener,
					Name:   fmt.Sprintf("%s:%d", aws.ToString(lb.LoadBalancerName), aws.ToInt32(l.Port)),
					Region: c.Region, Tags: tags,
					Prior: map[string]any{
						"default_actions": encodeActions(l.DefaultActions),
						"load_balancer":   aws.ToString(lb.LoadBalancerName),
					},
				})
			}
		}
	}
	return out, nil
}

func forwards(l elbtypes.Listener) bool {
	for _, a := range l.DefaultActions {
		if a.Type == elbtypes.ActionTypeEnumForward {
			return true
		}
	}
	return false
}

func (c *Clients) elbTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ELB.DescribeTags(ctx, &elasticloadbalancingv2.DescribeTagsInput{ResourceArns: []string{arn}})
	if err != nil {
		return nil, err
	}
	tags := map[string]string{}
	for _, d := range out.TagDescriptions {
		for _, t := range d.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// --- compute -----------------------------------------------------------------

func (c *Clients) functions(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	pager := lambda.NewListFunctionsPaginator(c.Lambda, &lambda.ListFunctionsInput{})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, f := range page.Functions {
			arn := aws.ToString(f.FunctionArn)
			tagsOut, err := c.Lambda.ListTags(ctx, &lambda.ListTagsInput{Resource: &arn})
			if err != nil {
				return nil, err
			}
			conc, err := c.Lambda.GetFunctionConcurrency(ctx, &lambda.GetFunctionConcurrencyInput{
				FunctionName: f.FunctionName,
			})
			if err != nil {
				return nil, err
			}
			prior := map[string]any{}
			// A function with no reserved concurrency is the common case, and
			// restoring it means *removing* the reservation rather than setting
			// a number. The distinction is recorded, not inferred.
			if conc.ReservedConcurrentExecutions != nil {
				prior["reserved_concurrency"] = int(*conc.ReservedConcurrentExecutions)
				prior["had_reservation"] = true
			} else {
				prior["had_reservation"] = false
			}
			if v, ok := prior["reserved_concurrency"]; ok && v == 0 {
				continue // already throttled to nothing
			}
			out = append(out, model.Resource{
				ID: aws.ToString(f.FunctionName), ARN: arn, Kind: model.KindLambda,
				Name: aws.ToString(f.FunctionName), Region: c.Region,
				Tags: tagsOut.Tags, Prior: prior,
			})
		}
	}
	return out, nil
}

func (c *Clients) services(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	clusters := ecs.NewListClustersPaginator(c.ECS, &ecs.ListClustersInput{})
	for clusters.HasMorePages() {
		cpage, err := clusters.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cluster := range cpage.ClusterArns {
			svcPager := ecs.NewListServicesPaginator(c.ECS, &ecs.ListServicesInput{Cluster: aws.String(cluster)})
			for svcPager.HasMorePages() {
				spage, err := svcPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, batch := range chunk(spage.ServiceArns, 10) { // DescribeServices caps at 10
					desc, err := c.ECS.DescribeServices(ctx, &ecs.DescribeServicesInput{
						Cluster: aws.String(cluster), Services: batch,
						Include: []ecstypesInclude{"TAGS"},
					})
					if err != nil {
						return nil, err
					}
					for _, s := range desc.Services {
						tags := map[string]string{}
						for _, t := range s.Tags {
							tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
						}
						out = append(out, model.Resource{
							ID: aws.ToString(s.ServiceArn), ARN: aws.ToString(s.ServiceArn),
							Kind: model.KindECSService, Name: aws.ToString(s.ServiceName),
							Region: c.Region, Tags: tags,
							Prior: map[string]any{
								"desired_count": int(s.DesiredCount),
								"cluster":       cluster,
								"service":       aws.ToString(s.ServiceName),
							},
						})
					}
				}
			}
		}
	}
	return out, nil
}

func (c *Clients) autoScalingGroups(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	pager := autoscaling.NewDescribeAutoScalingGroupsPaginator(c.ASG, &autoscaling.DescribeAutoScalingGroupsInput{})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, g := range page.AutoScalingGroups {
			out = append(out, model.Resource{
				ID: aws.ToString(g.AutoScalingGroupName), ARN: aws.ToString(g.AutoScalingGroupARN),
				Kind: model.KindASG, Name: aws.ToString(g.AutoScalingGroupName),
				Region: c.Region, Tags: asgTags(g.Tags),
				Prior: map[string]any{
					"desired_capacity": int(aws.ToInt32(g.DesiredCapacity)),
					"min_size":         int(aws.ToInt32(g.MinSize)),
					"max_size":         int(aws.ToInt32(g.MaxSize)),
				},
			})
		}
	}
	return out, nil
}

func asgTags(tags []asgtypes.TagDescription) map[string]string {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

// instances finds standalone running instances, and works out whether stopping
// one erases anything.
//
// Instances belonging to an Auto Scaling group are skipped: the group is the
// thing to scale, and stopping a member directly just makes the group replace
// it.
func (c *Clients) instances(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	pager := ec2.NewDescribeInstancesPaginator(c.EC2, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{Name: aws.String("instance-state-name"), Values: []string{"running"}}},
	})
	storeCache := map[string]bool{}
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range page.Reservations {
			for _, inst := range r.Instances {
				tags := ec2Tags(inst.Tags)
				if _, managed := tags["aws:autoscaling:groupName"]; managed {
					continue
				}
				hasStore, err := c.instanceStore(ctx, string(inst.InstanceType), storeCache)
				if err != nil {
					return nil, err
				}
				// Unknown family or size leaves this zero, which the plan
				// renders as "not estimated" rather than as free.
				price, _ := EC2HourlyUSD(string(inst.InstanceType))
				out = append(out, model.Resource{
					ID: aws.ToString(inst.InstanceId), Kind: model.KindEC2Instance,
					Name: nameOf(tags, aws.ToString(inst.InstanceId)), Region: c.Region,
					Tags: tags, HasInstanceStore: hasStore,
					EstimatedHourlyUSD: price,
					Prior: map[string]any{
						"state": string(inst.State.Name),
						"type":  string(inst.InstanceType),
					},
				})
			}
		}
	}
	return out, nil
}

// instanceStore asks whether the instance type has local NVMe. A stop erases
// it, EBS survives, and nothing in the stop API mentions the difference.
func (c *Clients) instanceStore(ctx context.Context, instanceType string, cache map[string]bool) (bool, error) {
	if v, ok := cache[instanceType]; ok {
		return v, nil
	}
	out, err := c.EC2.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
	})
	if err != nil {
		return false, err
	}
	has := false
	for _, t := range out.InstanceTypes {
		if aws.ToBool(t.InstanceStorageSupported) {
			has = true
		}
	}
	cache[instanceType] = has
	return has, nil
}

// --- network -----------------------------------------------------------------

// natGateways records not just the gateway but every route pointing at it,
// because a NAT gateway cannot be stopped — only deleted — and putting it back
// means recreating it *and* repointing those routes.
//
// The Elastic IP survives deletion (it is disassociated, not released), so
// recreating with the same allocation preserves the public address.
func (c *Clients) natGateways(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	pager := ec2.NewDescribeNatGatewaysPaginator(c.EC2, &ec2.DescribeNatGatewaysInput{
		Filter: []ec2types.Filter{{Name: aws.String("state"), Values: []string{"available"}}},
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, n := range page.NatGateways {
			id := aws.ToString(n.NatGatewayId)
			routes, err := c.routesVia(ctx, id)
			if err != nil {
				return nil, err
			}
			prior := map[string]any{
				"subnet_id":         aws.ToString(n.SubnetId),
				"vpc_id":            aws.ToString(n.VpcId),
				"connectivity_type": string(n.ConnectivityType),
				"routes":            routes,
			}
			if len(n.NatGatewayAddresses) > 0 {
				prior["allocation_id"] = aws.ToString(n.NatGatewayAddresses[0].AllocationId)
			}
			tags := ec2Tags(n.Tags)
			out = append(out, model.Resource{
				ID: id, Kind: model.KindNATGateway, Name: nameOf(tags, id),
				Region: c.Region, Tags: tags, Prior: prior,
				// The headline idle cost in most accounts, and it holds nothing.
				EstimatedHourlyUSD: natGatewayHourlyUSD,
			})
		}
	}
	return out, nil
}

func (c *Clients) routesVia(ctx context.Context, natID string) ([]any, error) {
	out, err := c.EC2.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{{Name: aws.String("route.nat-gateway-id"), Values: []string{natID}}},
	})
	if err != nil {
		return nil, err
	}
	var routes []any
	for _, rt := range out.RouteTables {
		for _, r := range rt.Routes {
			if aws.ToString(r.NatGatewayId) != natID {
				continue
			}
			routes = append(routes, map[string]any{
				"route_table_id": aws.ToString(rt.RouteTableId),
				"destination":    aws.ToString(r.DestinationCidrBlock),
			})
		}
	}
	return routes, nil
}

// --- data --------------------------------------------------------------------

func (c *Clients) databases(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource

	instPager := rds.NewDescribeDBInstancesPaginator(c.RDS, &rds.DescribeDBInstancesInput{})
	for instPager.HasMorePages() {
		page, err := instPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, db := range page.DBInstances {
			// A cluster member cannot be stopped on its own; the cluster is the
			// unit. Stopping one would fail, or worse, half-succeed.
			if aws.ToString(db.DBClusterIdentifier) != "" {
				continue
			}
			price, _ := RDSHourlyUSD(aws.ToString(db.DBInstanceClass))
			out = append(out, model.Resource{
				ID: aws.ToString(db.DBInstanceIdentifier), ARN: aws.ToString(db.DBInstanceArn),
				Kind: model.KindRDSInstance, Name: aws.ToString(db.DBInstanceIdentifier),
				Region: c.Region, Tags: rdsTags(db.TagList),
				EstimatedHourlyUSD: price,
				Prior: map[string]any{
					"status": aws.ToString(db.DBInstanceStatus),
					"class":  aws.ToString(db.DBInstanceClass),
				},
			})
		}
	}

	clusterPager := rds.NewDescribeDBClustersPaginator(c.RDS, &rds.DescribeDBClustersInput{})
	for clusterPager.HasMorePages() {
		page, err := clusterPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cl := range page.DBClusters {
			out = append(out, model.Resource{
				ID: aws.ToString(cl.DBClusterIdentifier), ARN: aws.ToString(cl.DBClusterArn),
				Kind: model.KindRDSCluster, Name: aws.ToString(cl.DBClusterIdentifier),
				Region: c.Region, Tags: rdsTags(cl.TagList),
				Prior: map[string]any{
					"status": aws.ToString(cl.Status),
					"engine": aws.ToString(cl.Engine),
				},
			})
		}
	}
	return out, nil
}

// --- EKS, CloudFront, API Gateway --------------------------------------------

// nodegroups finds managed node groups, which are ASGs the EKS control plane
// owns. Scaling the underlying ASG directly does not work: the control plane
// reconciles it straight back, so the nodegroup itself is the thing to scale.
func (c *Clients) nodegroups(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	clusters := eks.NewListClustersPaginator(c.EKS, &eks.ListClustersInput{})
	for clusters.HasMorePages() {
		page, err := clusters.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cluster := range page.Clusters {
			groups := eks.NewListNodegroupsPaginator(c.EKS,
				&eks.ListNodegroupsInput{ClusterName: aws.String(cluster)})
			for groups.HasMorePages() {
				gp, err := groups.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, name := range gp.Nodegroups {
					r, err := c.nodegroup(ctx, cluster, name)
					if err != nil {
						return nil, err
					}
					if r != nil {
						out = append(out, *r)
					}
				}
			}
		}
	}
	return out, nil
}

func (c *Clients) nodegroup(ctx context.Context, cluster, name string) (*model.Resource, error) {
	d, err := c.EKS.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
		ClusterName: aws.String(cluster), NodegroupName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	ng := d.Nodegroup
	if ng == nil || ng.ScalingConfig == nil {
		return nil, nil
	}
	// A Fargate profile or a self-managed group has no scaling config and does
	// not arrive here; a nodegroup mid-update does, and scaling one of those
	// races the update. Leave it to the operator rather than fighting EKS.
	if ng.Status != "" && ng.Status != "ACTIVE" && ng.Status != "DEGRADED" {
		return nil, nil
	}
	return &model.Resource{
		// The cluster is part of the identity: two clusters can each have a
		// nodegroup called "default", and restore has to know which.
		ID:   cluster + "/" + name,
		ARN:  aws.ToString(ng.NodegroupArn),
		Kind: model.KindEKSNodegroup, Name: name, Region: c.Region,
		Tags: ng.Tags,
		Prior: map[string]any{
			"cluster_name": cluster,
			"nodegroup":    name,
			"desired_size": int(aws.ToInt32(ng.ScalingConfig.DesiredSize)),
			"min_size":     int(aws.ToInt32(ng.ScalingConfig.MinSize)),
			"max_size":     int(aws.ToInt32(ng.ScalingConfig.MaxSize)),
		},
	}, nil
}

// distributions finds enabled CloudFront distributions. Nil client means this
// is not the region that walks the global services.
func (c *Clients) distributions(ctx context.Context) ([]model.Resource, error) {
	if c.CloudFront == nil {
		return nil, nil
	}
	var out []model.Resource
	pager := cloudfront.NewListDistributionsPaginator(c.CloudFront, &cloudfront.ListDistributionsInput{})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if page.DistributionList == nil {
			continue
		}
		for _, d := range page.DistributionList.Items {
			id := aws.ToString(d.Id)
			out = append(out, model.Resource{
				ID: id, ARN: aws.ToString(d.ARN),
				Kind: model.KindCloudFront,
				Name: aws.ToString(d.DomainName),
				// Recorded as global rather than as whichever region happened
				// to find it, so a restore does not look for it in one region.
				Region: model.GlobalRegion,
				Prior: map[string]any{
					"enabled": aws.ToBool(d.Enabled),
					"comment": aws.ToString(d.Comment),
				},
			})
		}
	}
	return out, nil
}

// apiStages finds REST API stages that are not already throttled to zero.
//
// The stage's *default* method throttle is the lever: one write, one value to
// put back, and it applies to every method under the stage. Per-method
// overrides are left exactly as they are — restoring a stage-wide default is
// exact, and rewriting individual methods would not be.
func (c *Clients) apiStages(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource
	apis := apigateway.NewGetRestApisPaginator(c.APIGateway, &apigateway.GetRestApisInput{})
	for apis.HasMorePages() {
		page, err := apis.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, api := range page.Items {
			apiID := aws.ToString(api.Id)
			stages, err := c.APIGateway.GetStages(ctx,
				&apigateway.GetStagesInput{RestApiId: aws.String(apiID)})
			if err != nil {
				return nil, err
			}
			for _, st := range stages.Item {
				out = append(out, apiStage(c.Region, apiID, aws.ToString(api.Name), st))
			}
		}
	}
	return out, nil
}

func apiStage(region, apiID, apiName string, st apigwtypes.Stage) model.Resource {
	name := aws.ToString(st.StageName)
	// An absent default throttle is not "no limit": it is the account limit,
	// which is what restore has to put back. Recorded as -1 so the two cases
	// stay distinguishable in a JSON state file, where a missing key and a zero
	// look the same after a round trip.
	rate, burst := -1.0, -1
	if s, ok := st.MethodSettings["*/*"]; ok {
		rate = s.ThrottlingRateLimit
		burst = int(s.ThrottlingBurstLimit)
	}
	label := apiName
	if label == "" {
		label = apiID
	}
	return model.Resource{
		ID:     apiID + "/" + name,
		Kind:   model.KindAPIGatewayStage,
		Name:   label + " " + name,
		Region: region,
		Tags:   st.Tags,
		Prior: map[string]any{
			"rest_api_id": apiID,
			"stage_name":  name,
			"rate_limit":  rate,
			"burst_limit": burst,
		},
	}
}

// AccountID is the account these credentials belong to, or "unknown".
//
// Never an error: it is a label on a plan, and failing a cost incident because
// STS was slow would be the wrong trade.
func AccountID(ctx context.Context, cfg aws.Config) string {
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "unknown"
	}
	return aws.ToString(out.Account)
}
