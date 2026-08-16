package awsx

import "strings"

// List-price estimates for what stopping a resource saves per hour.
//
// These are constants, not Pricing API lookups. The trip path is the one that
// runs when an account is already on fire; adding a network call and a failure
// mode to it, to refine a number nobody acts on in the moment, is the wrong
// trade. Live pricing is worth revisiting only if the error here proves
// material.
//
// Two limits stated plainly, because a confident wrong number is worse than an
// admitted unknown:
//
//   - **us-east-1, on-demand, Linux.** Other regions run roughly 5-25% higher;
//     Windows and commercial-database licensing can double a figure. The saving
//     is therefore a floor for most accounts, not a quote.
//   - **Compute only.** Storage, data transfer, provisioned IOPS and snapshots
//     keep costing while an instance is stopped, and none of them are here.
//
// A kind or a family that is not in these tables reports *not estimated*, and
// the caller must render that as "unknown" rather than folding a zero into a
// total. Zero is a claim; an absent estimate is not.

// ec2FamilyLarge is the us-east-1 on-demand Linux price of the `.large` size of
// each family, in USD/hour. Source: AWS EC2 On-Demand pricing, checked
// 2026-08. Families are listed only where the whole family shares one curve.
var ec2FamilyLarge = map[string]float64{
	"t3":  0.0832,
	"t3a": 0.0752,
	"t4g": 0.0672,
	"m5":  0.096,
	"m5a": 0.086,
	"m6i": 0.096,
	"m6a": 0.0864,
	"m7i": 0.1008,
	"m7g": 0.0816,
	"c5":  0.085,
	"c5a": 0.077,
	"c6i": 0.085,
	"c6a": 0.0765,
	"c7i": 0.08925,
	"c7g": 0.0725,
	"r5":  0.126,
	"r5a": 0.113,
	"r6i": 0.126,
	"r6a": 0.1134,
	"r7i": 0.1323,
	"r7g": 0.1071,
}

// rdsFamilyLarge is the same idea for RDS, single-AZ, on-demand. Source: AWS
// RDS On-Demand pricing (MySQL/PostgreSQL engines), checked 2026-08. Oracle and
// SQL Server carry licence cost this does not model, which is why they are
// absent rather than approximated.
var rdsFamilyLarge = map[string]float64{
	"db.t3":  0.136,
	"db.t4g": 0.128,
	"db.m5":  0.342,
	"db.m6i": 0.342,
	"db.m6g": 0.308,
	"db.m7g": 0.324,
	"db.r5":  0.48,
	"db.r6i": 0.48,
	"db.r6g": 0.432,
	"db.r7g": 0.4536,
}

// sizeFactor is the multiplier from `.large` to another size within a family.
//
// EC2 and RDS on-demand pricing is linear in vCPU within a family: every step
// up doubles the size and the price. That is what makes one price per family
// enough, and it is checked against the published tables rather than assumed —
// where a family breaks the pattern it is left out of the map above.
var sizeFactor = map[string]float64{
	"nano":     0.125,
	"micro":    0.25,
	"small":    0.5,
	"medium":   0.5, // t-family medium is half a large, not a quarter
	"large":    1,
	"xlarge":   2,
	"2xlarge":  4,
	"3xlarge":  6,
	"4xlarge":  8,
	"6xlarge":  12,
	"8xlarge":  16,
	"9xlarge":  18,
	"12xlarge": 24,
	"16xlarge": 32,
	"18xlarge": 36,
	"24xlarge": 48,
	"metal":    32, // whole host; family-dependent, so the roughest entry here
}

// NAT gateways bill per hour regardless of traffic, at one flat rate. Source:
// AWS VPC pricing, us-east-1, checked 2026-08. This is the headline idle cost
// in most accounts and it holds nothing, which is why it was the first kind
// estimated.
const natGatewayHourlyUSD = 0.045

// EC2HourlyUSD estimates the saving from stopping one instance.
//
// The bool is the point of the signature: false means "no defensible estimate",
// which the caller must show as unknown. A new instance family that is not in
// the table reads as unknown rather than as free.
func EC2HourlyUSD(instanceType string) (float64, bool) {
	return familyPrice(instanceType, ec2FamilyLarge)
}

// RDSHourlyUSD estimates the saving from stopping one database instance.
//
// Single-AZ. A Multi-AZ deployment bills roughly double, and discovery does not
// currently record which it is — so this understates those, in the same
// direction as every other limit here.
func RDSHourlyUSD(instanceClass string) (float64, bool) {
	return familyPrice(instanceClass, rdsFamilyLarge)
}

// NATGatewayHourlyUSD is flat, so it needs no lookup — kept as a function for
// symmetry with the others, since callers switch over kinds.
func NATGatewayHourlyUSD() (float64, bool) { return natGatewayHourlyUSD, true }

func familyPrice(name string, table map[string]float64) (float64, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	idx := strings.LastIndex(name, ".")
	if idx <= 0 || idx == len(name)-1 {
		return 0, false
	}
	family, size := name[:idx], name[idx+1:]
	base, ok := table[family]
	if !ok {
		return 0, false
	}
	factor, ok := sizeFactor[size]
	if !ok {
		return 0, false
	}
	return base * factor, true
}
