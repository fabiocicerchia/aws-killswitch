// Discovery of the ingress phase: the load balancer listeners that currently
// forward traffic, and the tags that decide whether they are in scope.

package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

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
