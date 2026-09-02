// Discovery of the network phase: NAT gateways, and every route pointing at
// one — a NAT gateway cannot be stopped, so putting it back means recreating
// it and repointing those routes.

package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

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
