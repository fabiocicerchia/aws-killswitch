package awsx

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

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
	had, _ := model.PriorBool(en.Prior, "had_reservation")
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

func restoreService(ctx context.Context, c *Clients, en model.Entry) error {
	n, ok := model.PriorInt32(en.Prior, "desired_count")
	if !ok {
		return errors.New("no usable recorded desired count; refusing to guess")
	}
	return scaleService(ctx, c, en.Prior, n)
}

// restoreASG needs all three bounds or none: a partial restore would leave the
// group with a capacity nobody chose, so a missing or unusable value refuses
// rather than substituting a default.
func restoreASG(ctx context.Context, c *Clients, en model.Entry) error {
	min, okMin := model.PriorInt32(en.Prior, "min_size")
	des, okDes := model.PriorInt32(en.Prior, "desired_capacity")
	max, okMax := model.PriorInt32(en.Prior, "max_size")
	if !okMin || !okDes || !okMax {
		return errors.New("incomplete or unusable recorded capacity; refusing to guess")
	}
	return scaleASG(ctx, c, en.ID, min, des, max)
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

func stopInstance(ctx context.Context, c *Clients, id string) error {
	_, err := c.EC2.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{id}})
	return err
}

func startInstance(ctx context.Context, c *Clients, id string) error {
	_, err := c.EC2.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{id}})
	return err
}

func restoreNodegroup(ctx context.Context, c *Clients, en model.Entry) error {
	min, okMin := model.PriorInt32(en.Prior, "min_size")
	des, okDes := model.PriorInt32(en.Prior, "desired_size")
	if !okMin || !okDes {
		return errors.New("incomplete or unusable recorded node counts; refusing to guess")
	}
	return scaleNodegroup(ctx, c, en.Prior, min, des)
}

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
