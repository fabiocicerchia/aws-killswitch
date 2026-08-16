package model

import (
	"math"
	"testing"
)

func TestBlastRadiusSeparatesEstimatedFromUnpriced(t *testing.T) {
	p := Plan{Actions: []Action{
		{Resource: Resource{Kind: KindNATGateway, ID: "nat-1", EstimatedHourlyUSD: 0.045}},
		{Resource: Resource{Kind: KindNATGateway, ID: "nat-2", EstimatedHourlyUSD: 0.045}},
		{Resource: Resource{Kind: KindEC2Instance, ID: "i-1", EstimatedHourlyUSD: 0.192}},
		// No estimate: an unrecognised instance family. Must be counted as
		// unpriced, NOT summed as zero — zero would claim it is free.
		{Resource: Resource{Kind: KindEC2Instance, ID: "i-2"}},
		{Resource: Resource{Kind: KindASG, ID: "asg-1"}},
	}}

	b := p.BlastRadius()
	if got, want := b.SavingsUSD, 0.045+0.045+0.192; math.Abs(got-want) > 1e-9 {
		t.Errorf("total = %v, want %v", got, want)
	}
	if got := b.SavingsByKind[KindNATGateway]; math.Abs(got-0.09) > 1e-9 {
		t.Errorf("nat = %v, want 0.09", got)
	}
	if got := b.UnpricedByKind[KindEC2Instance]; got != 1 {
		t.Errorf("unpriced ec2 = %d, want 1", got)
	}
	if got := b.UnpricedByKind[KindASG]; got != 1 {
		t.Errorf("unpriced asg = %d, want 1", got)
	}
	if _, ok := b.SavingsByKind[KindASG]; ok {
		t.Error("a kind with no estimate must not appear in SavingsByKind at all")
	}
}
