package awsx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// blockListener swaps the default action for a fixed 503. Instant, reversible,
// and it leaves the target group and everything behind it untouched.
func blockListener(ctx context.Context, c *Clients, arn string) error {
	_, err := c.ELB.ModifyListener(ctx, &elasticloadbalancingv2.ModifyListenerInput{
		ListenerArn: aws.String(arn),
		DefaultActions: []elbtypes.Action{{
			Type: elbtypes.ActionTypeEnumFixedResponse,
			FixedResponseConfig: &elbtypes.FixedResponseActionConfig{
				StatusCode:  aws.String("503"),
				ContentType: aws.String("text/plain"),
				MessageBody: aws.String("Service temporarily unavailable (cost control)"),
			},
		}},
	})
	return err
}

func restoreListener(ctx context.Context, c *Clients, en model.Entry) error {
	raw, ok := en.Prior["default_actions"]
	if !ok {
		return errors.New("no recorded default actions; restoring would guess at the routing")
	}
	actions, err := decodeActions(raw)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		return errors.New("recorded default actions are empty; refusing to leave the listener with no route")
	}
	_, err = c.ELB.ModifyListener(ctx, &elasticloadbalancingv2.ModifyListenerInput{
		ListenerArn: aws.String(en.ID), DefaultActions: actions,
	})
	return err
}

// The listener's default actions are stored as JSON rather than as a bespoke
// struct, so a field this tool does not know about still survives the round
// trip and comes back on restore.
func encodeActions(actions []elbtypes.Action) any {
	b, err := json.Marshal(actions)
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil
	}
	return generic
}

func decodeActions(raw any) ([]elbtypes.Action, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var actions []elbtypes.Action
	if err := json.Unmarshal(b, &actions); err != nil {
		return nil, fmt.Errorf("recorded listener actions are unreadable: %w", err)
	}
	return actions, nil
}

func restoreDistribution(ctx context.Context, c *Clients, en model.Entry) error {
	on, ok := model.PriorBool(en.Prior, "enabled")
	if !ok {
		return errors.New("no recorded enabled state; refusing to guess")
	}
	if !on {
		// It was already disabled when we found it, so the planner refused
		// it and nothing was done. Re-disabling would be harmless but the
		// state file should never have carried it here.
		return nil
	}
	return setDistributionEnabled(ctx, c, en.ID, true)
}

// setDistributionEnabled flips a distribution on or off.
//
// CloudFront has no "disable" call: the whole config is read, one field is
// changed, and the config is written back with the ETag that came with it. The
// ETag is re-fetched here rather than recorded at discovery on purpose — it
// changes on every update, and a stale one fails with PreconditionFailed. The
// read is also what makes this safe: everything else in the config goes back
// exactly as it was found, including changes made between fire and restore.
func setDistributionEnabled(ctx context.Context, c *Clients, id string, enabled bool) error {
	if c.CloudFront == nil {
		return errors.New("no CloudFront client on this executor")
	}
	cur, err := c.CloudFront.GetDistributionConfig(ctx,
		&cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
	if err != nil {
		return err
	}
	if cur.DistributionConfig == nil {
		return fmt.Errorf("distribution %s returned no config", id)
	}
	if aws.ToBool(cur.DistributionConfig.Enabled) == enabled {
		return nil // already there; an update would only burn a propagation cycle
	}
	cfg := *cur.DistributionConfig
	cfg.Enabled = aws.Bool(enabled)
	_, err = c.CloudFront.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id: aws.String(id), IfMatch: cur.ETag, DistributionConfig: &cfg,
	})
	return err
}

func restoreStage(ctx context.Context, c *Clients, en model.Entry) error {
	rate, okRate := model.PriorFloat(en.Prior, "rate_limit")
	burst, okBurst := model.PriorInt(en.Prior, "burst_limit")
	if !okRate || !okBurst {
		return errors.New("no usable recorded throttle; refusing to guess")
	}
	return throttleStage(ctx, c, en.Prior, rate, burst)
}

// throttleStage sets the stage-wide default method throttle.
//
// A rate of -1 means "there was no stage default, so the account limit applied"
// — the state discovery records for a stage that never had one. Restoring that
// removes the override rather than writing a number AWS never had, which is the
// difference between putting it back and pinning it to today's account limit.
func throttleStage(ctx context.Context, c *Clients, prior map[string]any, rate float64, burst int) error {
	apiID, okAPI := model.PriorString(prior, "rest_api_id")
	stage, okStage := model.PriorString(prior, "stage_name")
	if !okAPI || !okStage {
		return errors.New("recorded state names no API or stage")
	}

	var ops []apigwtypes.PatchOperation
	if rate < 0 || burst < 0 {
		ops = []apigwtypes.PatchOperation{
			{Op: apigwtypes.OpRemove, Path: aws.String("/*/*/throttling/rateLimit")},
			{Op: apigwtypes.OpRemove, Path: aws.String("/*/*/throttling/burstLimit")},
		}
	} else {
		ops = []apigwtypes.PatchOperation{
			{
				Op:    apigwtypes.OpReplace,
				Path:  aws.String("/*/*/throttling/rateLimit"),
				Value: aws.String(strconv.FormatFloat(rate, 'f', -1, 64)),
			},
			{
				Op:    apigwtypes.OpReplace,
				Path:  aws.String("/*/*/throttling/burstLimit"),
				Value: aws.String(strconv.Itoa(burst)),
			},
		}
	}
	_, err := c.APIGateway.UpdateStage(ctx, &apigateway.UpdateStageInput{
		RestApiId: aws.String(apiID), StageName: aws.String(stage), PatchOperations: ops,
	})
	return err
}
