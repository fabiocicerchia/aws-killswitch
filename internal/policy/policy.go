// Package policy is what the operator is allowed to hit, decided before the
// incident rather than during it.
//
// The default is deliberately useless: with no scope configured the planner
// refuses everything. A cost tool that defaults to "the whole account" is one
// typo away from being the outage, and the first time anyone reads the config
// carefully is after it has fired.
package policy

import (
	"errors"
	"fmt"
	"strings"
)

// ProtectTag marks a resource as untouchable. Checked before anything else, and
// there is no flag that overrides it — a team that tags a resource protected
// has made a decision the incident does not get to revisit.
const ProtectTag = "killswitch:protect"

type Policy struct {
	// Scope selects what may be touched. Empty means nothing.
	Scope Scope `json:"scope"`

	// IncludeDatabases opts into stopping RDS. Off by default: databases are
	// where the data is, stopping one has a seven-day fuse, and compute plus
	// egress is where runaway spend actually lives.
	IncludeDatabases bool `json:"include_databases"`
	// FinalSnapshot takes a snapshot as part of stopping a database.
	FinalSnapshot bool `json:"final_snapshot"`

	// AllowInstanceStoreLoss permits stopping instances whose local NVMe is
	// erased by the stop. Off by default; the API will not warn you.
	AllowInstanceStoreLoss bool `json:"allow_instance_store_loss"`

	// DeleteNATGateways opts into removing NAT gateways. They bill by the hour
	// and hold no state, but restoring one means recreating it and repointing
	// route tables — so it is a real change, not a stop.
	DeleteNATGateways bool `json:"delete_nat_gateways"`

	// ConfirmAbove requires an explicit force flag when the plan touches more
	// than this many resources. Zero means the default.
	ConfirmAbove int `json:"confirm_above"`

	// StateURI is where the snapshot goes. S3 is the sensible answer and is in
	// the never-touch set, so the kill switch cannot destroy its own restore.
	StateURI string `json:"state_uri"`
}

type Scope struct {
	// Tags that a resource must carry to be in scope. All must match.
	Tags map[string]string `json:"tags"`
	// Regions to search. Empty means the caller's configured region only.
	Regions []string `json:"regions"`
	// Everything disables tag scoping. It exists because some accounts are
	// genuinely single-purpose, but it has to be written down in a config file
	// rather than typed as a flag under pressure.
	Everything bool `json:"everything"`
}

const DefaultConfirmAbove = 25

func (p Policy) Threshold() int {
	if p.ConfirmAbove > 0 {
		return p.ConfirmAbove
	}
	return DefaultConfirmAbove
}

// Validate rejects a policy that would do something the operator probably did
// not intend. Called before discovery, so a mistake costs nothing.
func (p Policy) Validate() error {
	if !p.Scope.Everything && len(p.Scope.Tags) == 0 {
		return errors.New("no scope: set scope.tags to select what may be stopped, or scope.everything if this account really is single-purpose")
	}
	if p.Scope.Everything && len(p.Scope.Tags) > 0 {
		return errors.New("scope.everything and scope.tags are mutually exclusive — pick one")
	}
	for k := range p.Scope.Tags {
		if strings.EqualFold(k, ProtectTag) {
			return fmt.Errorf("scope.tags cannot select on %s; that tag only ever excludes", ProtectTag)
		}
	}
	if p.FinalSnapshot && !p.IncludeDatabases {
		return errors.New("final_snapshot has no effect without include_databases")
	}
	return nil
}

// InScope reports whether a resource may be considered, and why not when it may
// not. The reason is surfaced in the plan output — a resource that is quietly
// absent is the thing someone spends an hour looking for afterwards.
func (p Policy) InScope(tags map[string]string) (bool, string) {
	if isProtected(tags) {
		return false, "tagged " + ProtectTag
	}
	if p.Scope.Everything {
		return true, ""
	}
	for k, want := range p.Scope.Tags {
		got, ok := lookupFold(tags, k)
		if !ok {
			return false, "no " + k + " tag"
		}
		if !strings.EqualFold(got, want) {
			return false, fmt.Sprintf("%s=%s, scope wants %s", k, got, want)
		}
	}
	return true, ""
}

func isProtected(tags map[string]string) bool {
	v, ok := lookupFold(tags, ProtectTag)
	if !ok {
		return false
	}
	// Any value other than an explicit falsehood protects. A resource tagged
	// `killswitch:protect=` with an empty value was tagged for a reason, and
	// guessing that the reason was "no" is the wrong way to be wrong.
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

func lookupFold(tags map[string]string, key string) (string, bool) {
	if v, ok := tags[key]; ok {
		return v, true
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}
