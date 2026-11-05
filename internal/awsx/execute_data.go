package awsx

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

func stopDBInstance(ctx context.Context, c *Clients, id string, finalSnapshot bool) error {
	in := &rds.StopDBInstanceInput{DBInstanceIdentifier: aws.String(id)}
	if finalSnapshot {
		in.DBSnapshotIdentifier = aws.String(snapshotName(id))
	}
	_, err := c.RDS.StopDBInstance(ctx, in)
	return err
}

func startDBInstance(ctx context.Context, c *Clients, id string) error {
	_, err := c.RDS.StartDBInstance(ctx, &rds.StartDBInstanceInput{DBInstanceIdentifier: aws.String(id)})
	return err
}

func stopDBCluster(ctx context.Context, c *Clients, id string) error {
	_, err := c.RDS.StopDBCluster(ctx, &rds.StopDBClusterInput{DBClusterIdentifier: aws.String(id)})
	return err
}

func startDBCluster(ctx context.Context, c *Clients, id string) error {
	_, err := c.RDS.StartDBCluster(ctx, &rds.StartDBClusterInput{DBClusterIdentifier: aws.String(id)})
	return err
}

func snapshotName(id string) string {
	// A snapshot name has to be unique per attempt, which is what the clock
	// is for here; nothing reads it back as a time.
	return fmt.Sprintf("killswitch-%s-%d", id, time.Now().UTC().Unix()) //nolint:forbidigo // see above
}
