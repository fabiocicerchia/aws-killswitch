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
