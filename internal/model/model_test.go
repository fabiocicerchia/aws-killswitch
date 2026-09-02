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

// --- prior-state accessors for the new kinds --------------------------------
//
// These decide whether a restore puts back the real prior value or a guess, so
// each one refuses rather than coercing. A restore that reports success and
// leaves the account down is the worst failure this tool has.

func TestPriorBoolOnlyAcceptsARealBool(t *testing.T) {
	prior := map[string]any{
		"enabled": true, "off": false,
		"stringy": "true", "numeric": 1,
	}
	if v, ok := PriorBool(prior, "enabled"); !ok || !v {
		t.Errorf("enabled = %v, %v", v, ok)
	}
	if v, ok := PriorBool(prior, "off"); !ok || v {
		t.Errorf("off = %v, %v", v, ok)
	}
	// "true" and 1 look like yes and are not. Coercing them is how a
	// distribution stays dark after a restore says it succeeded.
	for _, key := range []string{"stringy", "numeric", "missing"} {
		if _, ok := PriorBool(prior, key); ok {
			t.Errorf("%s was accepted as a bool", key)
		}
	}
}

func TestPriorFloatKeepsFractionalRatesAndRefusesNonsense(t *testing.T) {
	prior := map[string]any{
		"rate": 10.5, "whole": 100, "none": -1.0,
		"negative": -5.0, "text": "10", "nan": math.NaN(),
		"inf": math.Inf(1),
	}
	// 10.5 requests/second is a legal API Gateway throttle, so this cannot
	// round-trip through an int.
	if v, ok := PriorFloat(prior, "rate"); !ok || v != 10.5 {
		t.Errorf("rate = %v, %v", v, ok)
	}
	if v, ok := PriorFloat(prior, "whole"); !ok || v != 100 {
		t.Errorf("whole = %v, %v", v, ok)
	}
	// -1 is the sentinel for "there was no stage default", and restore reads it
	// as "remove the override" — so it must not come back as a usable rate.
	for _, key := range []string{"none", "negative", "text", "nan", "inf", "missing"} {
		if _, ok := PriorFloat(prior, key); ok {
			t.Errorf("%s was accepted as a rate", key)
		}
	}
}

func TestNewKindsAreCountedInTheBlastRadius(t *testing.T) {
	// The blast radius is the number someone checks before typing yes. A kind
	// missing from it is a resource that gets stopped without being counted.
	p := Plan{Actions: []Action{
		{Resource: Resource{Kind: KindCloudFront, ID: "E1"}, Phase: PhaseIngress},
		{Resource: Resource{Kind: KindAPIGatewayStage, ID: "a/prod"}, Phase: PhaseIngress},
		{Resource: Resource{Kind: KindEKSNodegroup, ID: "c/ng"}, Phase: PhaseCompute},
	}}
	b := p.BlastRadius()
	if b.Total != 3 {
		t.Fatalf("total = %d, want 3", b.Total)
	}
	for _, k := range []Kind{KindCloudFront, KindAPIGatewayStage, KindEKSNodegroup} {
		if b.ByKind[k] != 1 {
			t.Errorf("%s counted %d times", k, b.ByKind[k])
		}
	}
}
