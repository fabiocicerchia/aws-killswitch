# aws-killswitch

[![CI](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/ci.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/ci.yml)
[![Code Quality](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/code-quality.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/code-quality.yml)
[![Security](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/security.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-Apache_2.0-blue.svg)](LICENSE)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/fabiocicerchia/aws-killswitch/badge)](https://securityscorecards.dev/viewer/?uri=github.com/fabiocicerchia/aws-killswitch)
[![CI carbon](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/fabiocicerchia/aws-killswitch/gh-pages/badge.json)](.github/workflows/carbon-badge.yml)

**Stop an AWS account spending, without losing anything.** Cuts ingress, drains
compute, and records exactly how to put it all back.

```console
$ aws-killswitch plan
plan ks-20260802-021500 — account 123456789012, regions eu-west-1

INGRESS — stop the traffic that drives the spend, before draining what serves it
  alb-listener   api-prod:443                     return a fixed 503 instead of forwarding to the target group

COMPUTE — drain compute; EBS is kept, nothing is deleted
  lambda         image-resize                     set reserved concurrency to 0
  ecs-service    checkout-api                     set desired count to 0
  asg            workers-prod                     set min/desired/max to 0

NOT TOUCHED (4)
  ec2-instance   build-cache (i-0a1b2c)           has instance-store volumes, which a stop erases; set allow_instance_store_loss to accept that
  rds-instance   orders-prod                      database: excluded unless include_databases is set
  lambda         cron-billing                     tagged killswitch:protect
  ecs-service    checkout-staging                 Env=staging, scope wants prod

4 resources would change

Nothing has changed. Run `fire --yes` to apply.
```

## Install

```sh
go install github.com/fabiocicerchia/aws-killswitch/cmd/aws-killswitch@latest
```

Or from a checkout:

```sh
make build      # -> ./bin/
```

## Use

```sh
aws-killswitch plan                        # read-only, safe against production
aws-killswitch fire                        # still a dry run
aws-killswitch fire --yes                  # actually stop it
aws-killswitch status                      # what is stopped, and any deadlines
aws-killswitch restore ks-20260802-021500 --yes
aws-killswitch spend --threshold 500       # exit 3 when month-to-date is over
```

`killswitch.json`:

```json
{
  "scope": { "tags": { "Env": "dev" }, "regions": ["eu-west-1"] },
  "state_uri": "s3://my-ops-bucket/killswitch",
  "confirm_above": 25,
  "include_databases": false,
  "delete_nat_gateways": false,
  "allow_instance_store_loss": false
}
```

Snapshots go to S3 *and* a local copy, and both must accept the write. S3 is in
the never-touch set on purpose: the kill switch must not be able to destroy its
own restore record. Turn on bucket versioning for the same reason.

## Proving the restore works

A killswitch nobody dares press is worthless, and "the API returned no error"
is not evidence that anything stopped. `aws-killswitch verify` fires, **reads
the account back**, restores, and reads it back again — reporting every place
what was planned and what actually happened diverged:

```sh
aws-killswitch verify --config killswitch.json --plan-only   # coverage, changes nothing
aws-killswitch verify --config killswitch.json --yes         # the real cycle
```

The second read is the point. `fire` succeeding means the calls were accepted;
it does not mean the desired count is actually zero, the listener actually
blocked, or the concurrency actually pinned. Only reading the account back can
tell you that, and only comparing against the *pre-fire* state can tell you the
restore put back what was there rather than merely something different.

**Scratch accounts only** — it fires for real.
[`examples/scratch-account/`](examples/scratch-account/) is a Terraform module
that seeds one minimal resource of every supported kind, so a run has something
to exercise; a kind the account does not hold is reported as `NOT EXERCISED`,
because "no divergence" over a kind that was never present is not evidence of
anything.

The report is counts and kinds and nothing else, so it can be pasted into an
issue: ARNs, account ids and resource names have nowhere in the report type to
live, and a test asserts they cannot reach the output even when they are the
subject of a finding.

## Documentation

Full docs live in [`docs/`](docs/). Runnable examples live in [`examples/`](examples/).

## License

Apache-2.0 — see [LICENSE](LICENSE).
