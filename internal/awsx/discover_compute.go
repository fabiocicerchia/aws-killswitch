// Discovery of the compute phase: Lambda reservations, ECS desired counts, ASG
// bounds and loose EC2 instances, each recorded with what a restore needs.

package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

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
			rs, err := c.servicesIn(ctx, cluster)
			if err != nil {
				return nil, err
			}
			out = append(out, rs...)
		}
	}
	return out, nil
}

// servicesIn records the desired count of every service in one cluster, along
// with the cluster and service names the restore will need to address it.
func (c *Clients) servicesIn(ctx context.Context, cluster string) ([]model.Resource, error) {
	var out []model.Resource
	pager := ecs.NewListServicesPaginator(c.ECS, &ecs.ListServicesInput{Cluster: aws.String(cluster)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, batch := range chunk(page.ServiceArns, 10) { // DescribeServices caps at 10
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
