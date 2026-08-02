package awsx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// Spend reads month-to-date unblended cost, which is the number a threshold is
// usually written against.
//
// Cost Explorer lags by several hours to a day. That is fine for "the bill is
// running away" and useless for "something spiked ten minutes ago" — a spike
// needs a CloudWatch alarm on a billing metric, or Budgets, to trigger this
// tool rather than this tool doing the polling. Said plainly here because a
// threshold that silently checks stale data is worse than no threshold.
type Spend struct {
	MonthToDateUSD float64
	Forecast       float64
	Start, End     time.Time
	Stale          bool
}

func MonthToDate(ctx context.Context, c *costexplorer.Client, now time.Time) (Spend, error) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	// Cost Explorer's end date is exclusive, and it rejects a range that ends
	// before it starts — so the first of the month needs tomorrow, not today.
	end := now.AddDate(0, 0, 1).Truncate(24 * time.Hour)

	out, err := c.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		Granularity: cetypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
	})
	if err != nil {
		return Spend{}, err
	}

	s := Spend{Start: start, End: end}
	for _, r := range out.ResultsByTime {
		amt, ok := r.Total["UnblendedCost"]
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(aws.ToString(amt.Amount), 64)
		if err != nil {
			return Spend{}, fmt.Errorf("unreadable cost amount %q: %w", aws.ToString(amt.Amount), err)
		}
		s.MonthToDateUSD += v
		if r.Estimated {
			// Cost Explorer marks the current period estimated until it
			// finalises. Worth surfacing, not worth refusing to act on.
			s.Stale = true
		}
	}
	return s, nil
}
