package docs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go/middleware"

	"github.com/fabiocicerchia/aws-killswitch/internal/awsx"
	"github.com/fabiocicerchia/aws-killswitch/internal/model"
	"github.com/fabiocicerchia/aws-killswitch/internal/verify"
)

// iamPrefix maps an SDK service ID to the prefix IAM uses for its actions.
var iamPrefix = map[string]string{
	"API Gateway":               "apigateway",
	"Auto Scaling":              "autoscaling",
	"CloudFront":                "cloudfront",
	"Cost Explorer":             "ce",
	"EC2":                       "ec2",
	"ECS":                       "ecs",
	"EKS":                       "eks",
	"Elastic Load Balancing v2": "elasticloadbalancing",
	"Lambda":                    "lambda",
	"RDS":                       "rds",
	"STS":                       "sts",
}

var errStop = errors.New("recorded, not sent")

// recordingConfig builds real SDK clients whose every call is recorded as the
// IAM action it needs and then stopped before it leaves the process.
func recordingConfig(calls map[string]bool) aws.Config {
	return aws.Config{
		Region: "us-east-1",
		APIOptions: []func(*middleware.Stack) error{func(s *middleware.Stack) error {
			// Serialize, not Initialize: the service and operation names are
			// only on the context once Initialize has run.
			return s.Serialize.Add(middleware.SerializeMiddlewareFunc("record",
				func(ctx context.Context, _ middleware.SerializeInput, _ middleware.SerializeHandler) (middleware.SerializeOutput, middleware.Metadata, error) {
					svc, op := awsmiddleware.GetServiceID(ctx), awsmiddleware.GetOperationName(ctx)
					// API Gateway is authorised by HTTP verb, not operation name.
					if svc == "API Gateway" {
						op = map[bool]string{true: "GET", false: "PATCH"}[strings.HasPrefix(op, "Get")]
					}
					calls[iamPrefix[svc]+":"+op] = true
					return middleware.SerializeOutput{}, middleware.Metadata{}, errStop
				}), middleware.Before)
		}},
	}
}

// TestPoliciesGrantEveryCallTheToolMakes drives the real executor and
// discovery code for every kind and checks each AWS call it reaches is
// allowed: a policy that misses one fails mid-fire, or worse, mid-restore.
func TestPoliciesGrantEveryCallTheToolMakes(t *testing.T) {
	ctx := context.Background()
	readCalls, writeCalls := map[string]bool{}, map[string]bool{}

	cfg := recordingConfig(readCalls)
	c := awsx.NewClients(cfg, "us-east-1").WithCloudFront(cfg)
	c.Discover(ctx)
	awsx.AccountID(ctx, cfg)
	awsx.MonthToDate(ctx, costexplorer.NewFromConfig(cfg), time.Now())

	wcfg := recordingConfig(writeCalls)
	ex := awsx.NewExecutor(map[string]*awsx.Clients{
		"us-east-1": awsx.NewClients(wcfg, "us-east-1").WithCloudFront(wcfg),
	})
	// Just enough recorded state for each restore to reach its AWS call.
	prior := map[string]any{
		"cluster": "c", "service": "s", "desired_count": 1,
		"min_size": 1, "max_size": 1, "desired_capacity": 1, "desired_size": 1,
		"had_reservation": true, "reserved_concurrency": 5, "enabled": true,
		"cluster_name": "c", "nodegroup": "n",
		"rest_api_id": "a", "stage_name": "s", "rate_limit": 10.0, "burst_limit": 5,
		"subnet_id": "subnet-1",
	}
	for _, k := range verify.AllKinds {
		region := "us-east-1"
		if k == model.KindCloudFront {
			region = model.GlobalRegion
		}
		ex.Apply(ctx, model.Action{Resource: model.Resource{Kind: k, ID: "id", Region: region, Prior: prior}})
		ex.Restore(ctx, model.Entry{Kind: k, ID: "id", Region: region, Prior: prior})
	}

	readonly, fire := map[string]bool{}, map[string]bool{}
	for _, a := range allowed(load(t, "iam-plan-readonly.json")) {
		readonly[a] = true
	}
	for _, a := range allowed(load(t, "iam-fire.json")) {
		fire[a] = true
	}
	for a := range readCalls {
		if !readonly[a] {
			t.Errorf("plan/status/spend call %q is not in iam-plan-readonly.json", a)
		}
	}
	for a := range writeCalls {
		if !readonly[a] && !fire[a] {
			t.Errorf("fire/restore call %q is in neither iam-plan-readonly.json nor iam-fire.json", a)
		}
	}
	// Guard against the recorder silently seeing nothing.
	if len(readCalls) < 10 || len(writeCalls) < 10 {
		t.Fatalf("recorded too few calls: read %v, write %v", readCalls, writeCalls)
	}
	t.Logf("checked %d read and %d write calls", len(readCalls), len(writeCalls))
}
