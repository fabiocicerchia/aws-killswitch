// Discovery of the data phase: RDS instances and clusters. Opt-in, and the one
// phase whose stop expires by itself after seven days.

package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

func (c *Clients) databases(ctx context.Context) ([]model.Resource, error) {
	var out []model.Resource

	instPager := rds.NewDescribeDBInstancesPaginator(c.RDS, &rds.DescribeDBInstancesInput{})
	for instPager.HasMorePages() {
		page, err := instPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, db := range page.DBInstances {
			// A cluster member cannot be stopped on its own; the cluster is the
			// unit. Stopping one would fail, or worse, half-succeed.
			if aws.ToString(db.DBClusterIdentifier) != "" {
				continue
			}
			price, _ := RDSHourlyUSD(aws.ToString(db.DBInstanceClass))
			out = append(out, model.Resource{
				ID: aws.ToString(db.DBInstanceIdentifier), ARN: aws.ToString(db.DBInstanceArn),
				Kind: model.KindRDSInstance, Name: aws.ToString(db.DBInstanceIdentifier),
				Region: c.Region, Tags: rdsTags(db.TagList),
				EstimatedHourlyUSD: price,
				Prior: map[string]any{
					"status": aws.ToString(db.DBInstanceStatus),
					"class":  aws.ToString(db.DBInstanceClass),
				},
			})
		}
	}

	clusterPager := rds.NewDescribeDBClustersPaginator(c.RDS, &rds.DescribeDBClustersInput{})
	for clusterPager.HasMorePages() {
		page, err := clusterPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cl := range page.DBClusters {
			out = append(out, model.Resource{
				ID: aws.ToString(cl.DBClusterIdentifier), ARN: aws.ToString(cl.DBClusterArn),
				Kind: model.KindRDSCluster, Name: aws.ToString(cl.DBClusterIdentifier),
				Region: c.Region, Tags: rdsTags(cl.TagList),
				Prior: map[string]any{
					"status": aws.ToString(cl.Status),
					"engine": aws.ToString(cl.Engine),
				},
			})
		}
	}
	return out, nil
}
