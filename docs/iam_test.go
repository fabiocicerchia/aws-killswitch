// Package docs holds no code; this test keeps the IAM policies shipped here
// honest about the promises their own Comment fields make.
package docs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// actions accepts IAM's string-or-array form of Action.
type actions []string

func (a *actions) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = actions{one}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(a))
}

type statement struct {
	Sid    string
	Effect string
	Action actions
}

type policy struct {
	Version   string
	Statement []statement
}

func load(t *testing.T, name string) policy {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var p policy
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return p
}

func allowed(p policy) []string {
	var out []string
	for _, s := range p.Statement {
		if s.Effect == "Allow" {
			out = append(out, s.Action...)
		}
	}
	return out
}

func TestEveryPolicyIsWellFormed(t *testing.T) {
	files, _ := filepath.Glob("iam-*.json")
	if len(files) != 4 {
		t.Fatalf("want 4 iam-*.json policies, found %v", files)
	}
	for _, f := range files {
		p := load(t, f)
		if p.Version != "2012-10-17" {
			t.Errorf("%s: Version = %q, want 2012-10-17", f, p.Version)
		}
		if len(p.Statement) == 0 {
			t.Errorf("%s: no statements", f)
		}
		for i, s := range p.Statement {
			if s.Effect != "Allow" && s.Effect != "Deny" {
				t.Errorf("%s statement %d: Effect = %q", f, i, s.Effect)
			}
			if len(s.Action) == 0 {
				t.Errorf("%s statement %d: no actions", f, i)
			}
			for _, a := range s.Action {
				if svc, op, ok := strings.Cut(a, ":"); !ok || svc == "" || op == "" {
					t.Errorf("%s statement %d: malformed action %q", f, i, a)
				}
			}
		}
	}
}

func TestReadonlyPolicyCannotChangeAnything(t *testing.T) {
	readVerbs := []string{"Describe", "List", "Get", "GET"}
	for _, a := range allowed(load(t, "iam-plan-readonly.json")) {
		_, op, _ := strings.Cut(a, ":")
		ok := false
		for _, v := range readVerbs {
			ok = ok || strings.HasPrefix(op, v)
		}
		if !ok {
			t.Errorf("read-only policy allows %q", a)
		}
	}
}

func TestFirePolicyDeletesOnlyNatGateways(t *testing.T) {
	p := load(t, "iam-fire.json")
	for _, a := range allowed(p) {
		_, op, _ := strings.Cut(a, ":")
		if strings.HasPrefix(op, "Delete") && a != "ec2:DeleteNatGateway" && a != "lambda:DeleteFunctionConcurrency" {
			t.Errorf("fire policy allows destructive %q", a)
		}
	}
	denied := map[string]bool{}
	for _, s := range p.Statement {
		if s.Effect == "Deny" {
			for _, a := range s.Action {
				denied[a] = true
			}
		}
	}
	for _, a := range []string{"s3:DeleteObject", "ec2:DeleteVolume", "ec2:DeleteSnapshot", "rds:DeleteDBInstance", "dynamodb:DeleteTable"} {
		if !denied[a] {
			t.Errorf("fire policy does not explicitly deny %q", a)
		}
	}
}

func TestOnlyLambdaAssumesTheExecutionRole(t *testing.T) {
	b, err := os.ReadFile("iam-lambda-trust.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Statement []struct {
			Effect    string
			Action    actions
			Principal json.RawMessage
		}
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Statement) != 1 {
		t.Fatalf("trust policy has %d statements, want 1", len(p.Statement))
	}
	s := p.Statement[0]
	if s.Effect != "Allow" || len(s.Action) != 1 || s.Action[0] != "sts:AssumeRole" {
		t.Errorf("trust statement = %s %v, want Allow sts:AssumeRole", s.Effect, s.Action)
	}
	var principal map[string]string
	if err := json.Unmarshal(s.Principal, &principal); err != nil || len(principal) != 1 || principal["Service"] != "lambda.amazonaws.com" {
		t.Errorf("trust principal = %s, want only Service lambda.amazonaws.com", s.Principal)
	}
}

func TestLambdaExtraCannotChangeAResource(t *testing.T) {
	for _, a := range allowed(load(t, "iam-lambda-extra.json")) {
		if !strings.HasPrefix(a, "logs:") && !strings.HasPrefix(a, "s3:List") && !strings.HasPrefix(a, "s3:Get") {
			t.Errorf("lambda-extra policy allows %q beyond its own logs and reads", a)
		}
	}
}
