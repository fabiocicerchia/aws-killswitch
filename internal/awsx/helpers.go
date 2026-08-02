package awsx

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// ecstypesInclude keeps the DescribeServices call readable at the point of use.
type ecstypesInclude = ecstypes.ServiceField

func ec2Tags(tags []ec2types.Tag) map[string]string {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

func rdsTags(tags []rdstypes.Tag) map[string]string {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

// nameOf prefers the Name tag, because that is what people call the thing, and
// falls back to the id so a plan is never printed with a blank row.
func nameOf(tags map[string]string, fallback string) string {
	if v, ok := tags["Name"]; ok && v != "" {
		return v
	}
	return fallback
}

func chunk[T any](in []T, n int) [][]T {
	var out [][]T
	for len(in) > n {
		out = append(out, in[:n])
		in = in[n:]
	}
	if len(in) > 0 {
		out = append(out, in)
	}
	return out
}
