// Discovery of the ingress phase: the load balancer listeners that currently
// forward traffic, and the tags that decide whether they are in scope.

package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
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
