# Basic Example

What it shows: the narrowest useful policy, a plan that changes nothing, and
the restore — in that order, because that is the order that keeps this safe.

## The policy

[`killswitch.json`](killswitch.json) scopes to `Env=dev` in one region. The
tool **refuses to run without a policy**, and this is the shape of the first
one you should write:

```json
{
  "scope": { "tags": { "Env": "dev" }, "regions": ["eu-west-1"] },
  "state_uri": "s3://my-ops-bucket/killswitch",
  "confirm_above": 25
}
```

Start with a scope you would be comfortable stopping *right now*. Widening it
is a one-line change you can make after a real fire and a real restore;
narrowing it after an incident is not a thing you get to do.

## Set up the state bucket first

```sh
aws s3 mb s3://my-ops-bucket
aws s3api put-bucket-versioning --bucket my-ops-bucket \
  --versioning-configuration Status=Enabled
```

Versioning matters because a truncated snapshot is the one failure this tool
cannot recover from. S3 is in the never-touch set for the same reason: the kill
switch must not be able to destroy its own restore record.

## Plan — this changes nothing

```sh
aws-killswitch -c killswitch.json plan
```

```text
plan ks-20260802-021500 — account 123456789012, regions eu-west-1

INGRESS — stop the traffic that drives the spend, before draining what serves it
  alb-listener   api-dev:443            return a fixed 503 instead of forwarding

COMPUTE — drain compute; EBS is kept, nothing is deleted
  ecs-service    checkout-dev           set desired count to 0
  asg            workers-dev            set min/desired/max to 0

NOT TOUCHED (4)
  ec2-instance   build-cache (i-0a1b2c) has instance-store volumes, which a stop erases
  rds-instance   orders-dev             database: excluded unless include_databases is set
  lambda         cron-billing           tagged killswitch:protect
  ecs-service    checkout-staging       Env=staging, scope wants dev

4 resources would change

Nothing has changed. Run `fire --yes` to apply.
```

**Read the `NOT TOUCHED` section, not the top one.** That is where you find out
the thing you expected to be stopped is out of scope, protected, or a database
— which is the finding that matters before you rely on this at 2am.

## Protect what must keep running

```sh
aws ec2 create-tags --resources i-0123456789 \
  --tags Key=killswitch:protect,Value=true
```

This beats every scope, and only an explicit `false`/`no`/`0` turns it off. A
resource tagged protected with an empty value was tagged for a reason.

## Fire, against a scratch account

```sh
aws-killswitch -c killswitch.json fire          # still a dry run
aws-killswitch -c killswitch.json fire --yes    # actually stops things
```

The restore record is written and **read back** before anything changes. If
that verification fails, the fire is abandoned and nothing has been touched —
an account that is still expensive is a problem; an account that is stopped
with no record of how to start it is an outage of unknown length.

## Put it back — the half that matters

```sh
aws-killswitch -c killswitch.json status
aws-killswitch -c killswitch.json restore ks-20260802-021500 --yes
```

Do this in the same sitting as your first fire. A kill switch whose restore has
never been exercised is a switch nobody will be willing to pull.

`status` also counts down the seven days before AWS restarts any stopped
database by itself:

```text
ks-20260802-021500  fired 2026-08-02T02:15:00Z  6 changed of 6
    ! rds-instance orders-dev: AWS restarts this in 5.0 days (2026-08-09T02:15:00Z)
```

## Wire it to a threshold

```sh
aws-killswitch -c killswitch.json spend --threshold 500 \
  || aws-killswitch -c killswitch.json fire --yes
```

Cost Explorer lags by hours, so this catches slow burn and nothing sudden. For
a fast trip, invoke the CLI from an AWS Budgets action or a CloudWatch alarm on
`EstimatedCharges` instead of polling.

## Two things this example does not do

**It does not include databases.** `include_databases` stays `false` here.
AWS restarts a stopped RDS instance after seven days, so a kill switch that
silently un-kills a week later is worse than none — turn it on deliberately,
and watch the `status` countdown.

**It does not accept instance-store loss.** `allow_instance_store_loss` stays
`false`, so instances with local NVMe are refused rather than stopped. Stopping
them erases those volumes; that has to be an explicit decision.
