package awsx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

func deleteNATGateway(ctx context.Context, c *Clients, id string) error {
	_, err := c.EC2.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(id)})
	return err
}

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

	// A record with no routes restores the gateway and repoints nothing,
	// which is the right outcome for a gateway that had no routes.
	routes, _ := en.Prior["routes"].([]any) //nolint:errcheck // see above
	return repointRoutes(ctx, c, newID, routes)
}

// repointRoutes sends every route that used the old gateway at the new one.
// Recreating the gateway without this leaves private subnets with no egress
// behind a restore that reported success.
func repointRoutes(ctx context.Context, c *Clients, newID string, routes []any) error {
	for _, raw := range routes {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rtb, _ := model.PriorString(r, "route_table_id")
		dst, _ := model.PriorString(r, "destination")
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
