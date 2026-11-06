package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
	"github.com/fabiocicerchia/aws-killswitch/internal/state"
)

const (
	// localDirName is the per-user directory holding the local snapshot copy and
	// the audit log, under $HOME.
	localDirName  = ".aws-killswitch"
	auditFileName = "audit.jsonl"
)

// policyFor loads the policy file, applies the -state override, and validates
// the result. Validation happens before discovery, so a mistake costs nothing.
func policyFor(o options) (policy.Policy, error) {
	pol, err := loadPolicy(o.configPath)
	if err != nil {
		return policy.Policy{}, err
	}
	if o.stateURI != "" {
		pol.StateURI = o.stateURI
	}
	if err := pol.Validate(); err != nil {
		return policy.Policy{}, failWith(exitConfig, fmt.Errorf("%s: %w", o.configPath, err))
	}
	return pol, nil
}

// warnAll reports discovery failures without returning them: a missing
// permission on one service should degrade the plan and say so, not leave the
// operator with nothing during an incident.
func warnAll(errs []error) {
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}
}

func loadPolicy(path string) (policy.Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy.Policy{}, failWith(exitNoInput, fmt.Errorf(
				"no policy at %s — this tool will not act without one, because the default would otherwise be the whole account.\n"+
					"Write one:\n\n%s", path, samplePolicy))
		}
		return policy.Policy{}, err
	}
	var p policy.Policy
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return policy.Policy{}, failWith(exitDataErr, fmt.Errorf("%s: %w", path, err))
	}
	return p, nil
}

const samplePolicy = `{
  "scope": { "tags": { "Env": "dev" }, "regions": ["eu-west-1"] },
  "state_uri": "s3://my-ops-bucket/killswitch",
  "confirm_above": 25,
  "include_databases": false,
  "delete_nat_gateways": false
}
`

func buildStore(cfg aws.Config, pol policy.Policy, localDir string) (state.Store, error) {
	local := state.Local{Dir: localDir}
	if pol.StateURI == "" {
		fmt.Fprintf(os.Stderr,
			"warning: no state_uri — snapshots are only in %s. "+
				"Lose that directory and the restore record goes with it.\n", localDir)
		return local, nil
	}
	bucket, prefix, ok := state.ParseURI(pol.StateURI)
	if !ok {
		return nil, failWith(exitConfig, fmt.Errorf("state_uri must be s3://bucket/prefix, got %q", pol.StateURI))
	}
	// Durable store first, so a rebuilt laptop still finds the record.
	return state.Multi{Stores: []state.Store{
		state.S3{Client: s3.NewFromConfig(cfg), Bucket: bucket, Prefix: prefix},
		local,
	}}, nil
}

func (o options) auditPathOrDefault() string {
	if o.auditPath != "" {
		return o.auditPath
	}
	return filepath.Join(o.localDir, auditFileName)
}

func defaultLocalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return localDirName
	}
	return filepath.Join(home, localDirName)
}
