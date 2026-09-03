// Package awsx is everything that talks to AWS: finding what is running, and
// changing it.
//
// Discovery records the prior state of each resource at the moment it is found,
// because that record is what makes the change reversible. It is deliberately
// read-only — nothing in the discover_*.go files mutates anything, so it is
// safe to run against production at any time, and `plan` does exactly that.
package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
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
