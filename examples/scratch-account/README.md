# Verifying against a scratch account

The planner, the state machine and the ordering are unit-tested. Discovery has
run against live endpoints and degrades correctly when a call is refused. But
**nothing had ever run with credentials that work**, so no fire and no restore
had ever executed — and unit tests cannot catch an API contract mismatch, which
is the failure this guards against.

> **Nothing here has been run.** The Terraform was written from the provider's
> documented schemas and never applied; `aws-killswitch verify` is unit-tested
> against an in-memory account and has never spoken to AWS. The first person to
> run this should expect to fix an argument or two — and should fix them here.

## What `verify` does

```text
discover ──► plan ──► fire ──► DISCOVER AGAIN ──► did each action land?
    │                                                    │
    └── pre-state ◄──────── DISCOVER AGAIN ◄── restore ◄──┘
                                  │
                            is everything back?
```

**The second read is the whole point.** `fire` returning no error means the API
accepted the calls. It does not mean the desired count is actually zero, the
listener actually blocked, or the concurrency actually pinned. Every action is
checked against what the account reads back — and after the restore, against
what was there before anything was touched.

## Running it

This creates real resources and **costs real money while they exist** — a NAT
gateway alone is roughly $32/month before data charges, and the RDS instances
and the EKS nodegroup are not free either.

```sh
cd examples/scratch-account
terraform init && terraform apply

aws-killswitch plan   --config killswitch.json   # read it first
aws-killswitch verify --config killswitch.json --yes

terraform destroy
```

`--plan-only` reports coverage without changing anything, which is the safe way
to check the policy is scoped as you think before the first real run.

`--settle` (default 90s) is how long the account is given before it is read
back. AWS is eventually consistent on most of these — an ECS service reports
its old desired count for a moment — and a verifier that read too early would
report a divergence that is only its own impatience.

## Safety

Everything the module creates carries `killswitch = scratch`, and the policy
scopes on exactly that tag. A mistyped profile therefore reaches nothing this
module did not make. It is still a fire: **use an account you would not mind
losing.**

## The report

Counts and kinds, and nothing else. The output is meant to be pasted into an
issue, so ARNs, account ids and resource names must not travel with it — and
the guarantee is structural rather than a filter: the report type has nowhere
to put them, and a test asserts that an ARN, an account id and a resource name
cannot reach the text even when they are the subject of a finding.

```text
kind                         found  planned  refused
----------------------------------------------------
alb-listener                     1        1        0
apigateway-stage                 1        1        0
asg                              1        1        0
...
fire     11 changed, 0 failed, 11 verified by a second read
restore  11 restored, 0 failed

NOT EXERCISED — the account held none of these, so this run says
nothing about them:
  ...
```

The **NOT EXERCISED** list is as important as the findings. "No divergence" over
a kind that was never present is not evidence of anything, and a report that
quietly omitted it would read like a pass.

Every divergence printed is its own bug to file, and `verify` exits non-zero
when there are any — so this can be run against a scratch account from CI.
