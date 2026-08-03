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
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

type Clients struct {
	Region string
	EC2    *ec2.Client
	ECS    *ecs.Client
	Lambda *lambda.Client
	RDS    *rds.Client
	ASG    *autoscaling.Client
	ELB    *elasticloadbalancingv2.Client
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
	}
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
				out = append(out, model.Resource{
					ID: aws.ToString(inst.InstanceId), Kind: model.KindEC2Instance,
					Name: nameOf(tags, aws.ToString(inst.InstanceId)), Region: c.Region,
					Tags: tags, HasInstanceStore: hasStore,
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
				EstimatedHourlyUSD: 0.045,
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
			out = append(out, model.Resource{
				ID: aws.ToString(db.DBInstanceIdentifier), ARN: aws.ToString(db.DBInstanceArn),
				Kind: model.KindRDSInstance, Name: aws.ToString(db.DBInstanceIdentifier),
				Region: c.Region, Tags: rdsTags(db.TagList),
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
