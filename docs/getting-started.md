# Getting Started

## Prerequisites

Go 1.24 or newer to build. AWS credentials with at least the read-only policy in
[`iam-plan-readonly.json`](iam-plan-readonly.json).

## Build

```sh
make build      # ./bin/aws-killswitch
```

## Write a policy first

The tool refuses to run without one. That is deliberate: a cost tool that
defaults to the whole account is one typo away from being the outage.

```json
{
  "scope": { "tags": { "Env": "dev" }, "regions": ["eu-west-1"] },
  "state_uri": "s3://my-ops-bucket/killswitch",
  "confirm_above": 25
}
```

Save it as `killswitch.json`. Start with a scope you would be comfortable
stopping right now — a dev tag — and widen it only after a real run.

## Set up the state bucket

Snapshots are what make a fire reversible, so the bucket holding them should
outlive anything else going wrong:

```sh
aws s3 mb s3://my-ops-bucket
aws s3api put-bucket-versioning --bucket my-ops-bucket \
  --versioning-configuration Status=Enabled
```

Versioning matters because a corrupted or truncated snapshot is the one failure
this tool cannot recover from. S3 is in the never-touch set for the same reason:
the kill switch must not be able to destroy its own restore record.

## First run — read only

```sh
./bin/aws-killswitch plan
```

This changes nothing and is safe against production. Read the whole output,
including the `NOT TOUCHED` section — that is where you find out that the thing
you expected to be stopped is out of scope, or protected, or a database.

## Protect what must keep running

Tag it. This beats every scope and there is no flag that overrides it:

```sh
aws ec2 create-tags --resources i-0123456789 \
  --tags Key=killswitch:protect,Value=true
```

## Fire

```sh
./bin/aws-killswitch fire          # still a dry run
./bin/aws-killswitch fire --yes    # actually stops things
```

Do this against a scratch account before you do it against anything else. No
fire has ever been executed against a live account — see the Status section of
the README.

## Put it back

```sh
./bin/aws-killswitch status
./bin/aws-killswitch restore ks-20260802-021500 --yes
```

`status` also counts down the seven days before AWS restarts any stopped
database by itself.

## Wiring it to a threshold

```sh
./bin/aws-killswitch spend --threshold 500 || ./bin/aws-killswitch fire --yes
```

Cost Explorer lags by hours, so this catches slow burn and nothing sudden. For a
fast trip, invoke the CLI from an AWS Budgets action or a CloudWatch alarm on
`EstimatedCharges` instead of polling.

## Triggering it

`spend` reads month-to-date cost from Cost Explorer and exits 3 above the
threshold, which is enough for a cron:

```sh
aws-killswitch spend --threshold 500 || aws-killswitch fire --yes
```

**But Cost Explorer lags by hours to a day.** That is fine for "the bill is
running away over a week" and useless for "something spiked twenty minutes ago".
A cron is also only ever as fast as its interval: the worst case is a full
interval of spend before anything happens, and shortening the interval trades
cost for latency without ever getting below it.

## The fast trip: a Budgets action

`cmd/killswitch-lambda` is the same tool behind a Lambda handler, so the trip is
driven by the spend signal itself rather than by a clock. Everything about what
gets stopped lives in `internal/trip`, which the CLI uses too — there is no
second implementation, because the path that only runs at three in the morning
during an incident is the one nobody would notice was wrong.

### Build and deploy

```sh
make lambda          # bin/killswitch-lambda.zip, runtime provided.al2023, arm64

aws lambda create-function \
  --function-name aws-killswitch \
  --runtime provided.al2023 --architectures arm64 --handler bootstrap \
  --role arn:aws:iam::123456789012:role/aws-killswitch \
  --timeout 300 \
  --zip-file fileb://bin/killswitch-lambda.zip \
  --environment "Variables={KILLSWITCH_POLICY=$(cat killswitch.json | jq -c .)}"
```

Give it a **generous timeout** — 300s is a reasonable floor. Discovery across
many regions is not fast, and a handler killed mid-fire leaves a snapshot
written with only part of it applied. That is recoverable, because the snapshot
goes in before the first change, but it is not what you want to find.

The policy travels in `KILLSWITCH_POLICY` as the file's contents, so the
function is one artifact with no bucket to read at fire time — one fewer thing
that can be unreachable in the minute it matters. `KILLSWITCH_POLICY_PATH` is
the alternative if you would rather ship it inside the package.

### Wire the Budgets action

Budgets actions deliver through SNS, so: budget → SNS topic → Lambda
subscription.

```sh
aws sns create-topic --name aws-killswitch-trip
aws sns subscribe --topic-arn arn:aws:sns:us-east-1:123456789012:aws-killswitch-trip \
  --protocol lambda --notification-endpoint arn:aws:lambda:eu-west-1:123456789012:function:aws-killswitch
aws lambda add-permission --function-name aws-killswitch \
  --statement-id sns-trip --action lambda:InvokeFunction \
  --principal sns.amazonaws.com \
  --source-arn arn:aws:sns:us-east-1:123456789012:aws-killswitch-trip
```

Then point a budget's notification at that topic. Budgets is a us-east-1
service; the function can live wherever you like.

### Test it without waiting for real spend

This is the part worth doing before you need it. A direct invocation with
`dry_run` exercises the whole path — credentials, permissions, every region's
discovery, the planner — and changes nothing:

```sh
aws lambda invoke --function-name aws-killswitch \
  --payload '{"dry_run": true}' --cli-binary-format raw-in-base64-out /dev/stdout
```

```json
{"fired":false,"dry_run":true,"plan_id":"20260902-141500","changed":14,
 "trigger":"direct invocation","account_id":"123456789012"}
```

A non-empty `warnings` array is a service that refused a Describe call: the plan
degrades and says so rather than silently omitting those resources. Fix those
before you rely on it.

To rehearse the SNS shape as well, invoke with a Records envelope — the handler
does not parse the message body, so any string will do:

```sh
aws lambda invoke --function-name aws-killswitch --cli-binary-format raw-in-base64-out \
  --payload '{"dry_run":true,"Records":[{"Sns":{"Subject":"AWS Budgets","MessageId":"test-1","Message":"{}"}}]}' \
  /dev/stdout
```

### It will not double-fire

A Budgets action is not delivered once. The same threshold can notify
repeatedly, and a retried invocation is normal rather than exceptional.

Without a guard, the second delivery would discover an account that is *already
stopped*, plan almost nothing, and write a second snapshot whose recorded prior
state is the stopped one. Restoring that snapshot would put the account back
exactly as it was after the first fire: down.

So a fire inside the **one-hour cooldown** is refused, and the response says so:

```json
{"fired":false,"skipped":"already fired 15m ago; inside the 1h0m0s cooldown"}
```

The check reads the state store, not process memory — every Lambda invocation is
a cold start with no memory of the last one, and the snapshot is the only thing
both share. A snapshot that has been *restored* does not hold the cooldown open,
because the account is running again and a new spike is a real event. And if the
store cannot be read at all, the fire is allowed: unreadable state is not
permission to double-fire, but a killswitch that will not press is worse than
one that presses twice.

Two other things the handler will not do unattended: fire a plan carrying
acknowledgement-required warnings (instance-store data loss, NAT deletion) —
pass `{"force": true}` if that is genuinely what you want — and fire without a
usable state store, because a killswitch with no restore path is the thing this
repo exists not to be.

## IAM

Policies in `docs/`, deliberately split:

| File | What it is |
| --- | --- |
| `iam-plan-readonly.json` | everything `plan`, `status` and `spend` need, and nothing that can change a resource — attach it widely |
| `iam-fire.json` | the write half, tag-conditioned, with an explicit `Deny` on every destructive action |
| `iam-lambda-trust.json` | the execution role's trust policy |
| `iam-lambda-extra.json` | what the Lambda needs on top of the other two: its own logs, and `s3:ListBucket` for the cooldown check |

Attach all four to the Lambda's execution role; the first two are enough for a
person at a terminal.

## Development

```sh
make test   # go test ./...
make lint   # vet + gofmt
```

`internal/plan` decides what happens and is pure. `internal/engine` enforces the
ordering and the write-first rule. `internal/awsx` is the only package that
talks to AWS. The first two hold the tests, because they hold the decisions.
