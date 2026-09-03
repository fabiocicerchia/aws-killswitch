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
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
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
