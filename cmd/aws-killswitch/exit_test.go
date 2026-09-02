package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/fabiocicerchia/aws-killswitch/internal/policy"
)

// Each case runs the real code path and asks what the process would exit with,
// because the number is the whole contract for a Budgets action or a CI job
// that cannot read an error message.

func TestMissingPolicyFileExitsNoInput(t *testing.T) {
	_, err := loadPolicy(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil {
		t.Fatal("a missing policy file must be an error")
	}
	if got := exitCode(err); got != exitNoInput {
		t.Errorf("exit code %d, want %d (EX_NOINPUT)", got, exitNoInput)
	}
}

func TestUnparseablePolicyExitsDataErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "killswitch.json")
	if err := os.WriteFile(path, []byte(`{"scope": `), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadPolicy(path)
	if err == nil {
		t.Fatal("truncated JSON must be an error")
	}
	if got := exitCode(err); got != exitDataErr {
		t.Errorf("exit code %d, want %d (EX_DATAERR)", got, exitDataErr)
	}
}

func TestUnscopedPolicyExitsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "killswitch.json")
	if err := os.WriteFile(path, []byte(`{"scope": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := policyFor(options{configPath: path})
	if err == nil {
		t.Fatal("a policy with no scope must be refused")
	}
	if got := exitCode(err); got != exitConfig {
		t.Errorf("exit code %d, want %d (EX_CONFIG)", got, exitConfig)
	}
}

func TestUnusableStateURIExitsConfig(t *testing.T) {
	pol := policy.Policy{StateURI: "gs://not-s3/x"}
	_, err := buildStore(context.Background(), aws.Config{}, pol, t.TempDir())
	if err == nil {
		t.Fatal("a state_uri that is not s3:// must be refused")
	}
	if got := exitCode(err); got != exitConfig {
		t.Errorf("exit code %d, want %d (EX_CONFIG)", got, exitConfig)
	}
}

func TestUnknownCommandExitsUsage(t *testing.T) {
	err := run(context.Background(), "detonate", nil, options{})
	if err == nil {
		t.Fatal("an unknown command must be an error")
	}
	if got := exitCode(err); got != exitUsage {
		t.Errorf("exit code %d, want %d (EX_USAGE)", got, exitUsage)
	}
}

// An error nobody classified must not inherit a meaning by accident.
func TestUnclassifiedErrorExitsFail(t *testing.T) {
	if got := exitCode(errors.New("the store went away mid-fire")); got != exitFail {
		t.Errorf("exit code %d, want %d", got, exitFail)
	}
}
