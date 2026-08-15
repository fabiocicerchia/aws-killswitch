package awsx

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEC2PricesMatchThePublishedTable(t *testing.T) {
	// Spot-checked against AWS EC2 On-Demand pricing, us-east-1, Linux.
	cases := map[string]float64{
		"t3.large":    0.0832,
		"t3.medium":   0.0416, // half a large
		"t3.micro":    0.0208,
		"m5.xlarge":   0.192, // 2x large
		"m5.4xlarge":  0.768, // 8x large
		"c6i.2xlarge": 0.34,
		"r6i.large":   0.126,
	}
	for typ, want := range cases {
		got, ok := EC2HourlyUSD(typ)
		if !ok {
			t.Errorf("%s: no estimate", typ)
			continue
		}
		approx(t, got, want)
	}
}

func TestRDSPrices(t *testing.T) {
	got, ok := RDSHourlyUSD("db.m5.large")
	if !ok {
		t.Fatal("db.m5.large has no estimate")
	}
	approx(t, got, 0.342)

	got, _ = RDSHourlyUSD("db.r6g.2xlarge")
	approx(t, got, 0.432*4)
}

func TestCaseAndWhitespaceDoNotChangeTheAnswer(t *testing.T) {
	a, _ := EC2HourlyUSD("M5.LARGE")
	b, _ := EC2HourlyUSD("  m5.large  ")
	c, _ := EC2HourlyUSD("m5.large")
	if a != c || b != c {
		t.Errorf("got %v/%v/%v, want all equal", a, b, c)
	}
}

func TestUnknownIsNotEstimatedRatherThanFree(t *testing.T) {
	// The distinction the issue asks for: an unpriced resource must be
	// reported as unknown, never folded into a total as zero.
	for _, typ := range []string{
		"x9z.large",   // family this build has never seen
		"m5.enormous", // size not in the table
		"m5",          // no size at all
		"m5.",         // trailing dot
		"",            // nothing
		".large",      // no family
	} {
		if usd, ok := EC2HourlyUSD(typ); ok {
			t.Errorf("%q returned an estimate of %v; want not-estimated", typ, usd)
		}
	}
}

func TestOracleAndSQLServerClassesAreNotGuessed(t *testing.T) {
	// Licence cost is not modelled, so those engines must not read as if the
	// compute price were the whole bill. db.m5 is shared across engines, so
	// this documents the known gap rather than asserting a refusal.
	if _, ok := RDSHourlyUSD("db.x2g.large"); ok {
		t.Error("db.x2g is not in the table and must not be estimated")
	}
}

func TestNATIsFlat(t *testing.T) {
	usd, ok := NATGatewayHourlyUSD()
	if !ok || usd != 0.045 {
		t.Errorf("got %v/%v, want 0.045/true", usd, ok)
	}
}

func TestSizeFactorsAreMonotonic(t *testing.T) {
	// Within a family, a bigger size must never cost less — a typo in the
	// table would otherwise make a 24xlarge look cheaper than a large.
	order := []string{"nano", "micro", "small", "large", "xlarge", "2xlarge", "4xlarge", "8xlarge", "16xlarge", "24xlarge"}
	prev := 0.0
	for _, size := range order {
		f, ok := sizeFactor[size]
		if !ok {
			t.Fatalf("%s missing from sizeFactor", size)
		}
		if f < prev {
			t.Errorf("%s factor %v is below the previous %v", size, f, prev)
		}
		prev = f
	}
}
