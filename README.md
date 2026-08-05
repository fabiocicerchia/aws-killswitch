# aws-killswitch

[![CI](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/ci.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/ci.yml)
[![Code Quality](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/code-quality.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/code-quality.yml)
[![Security](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/security.yml/badge.svg)](https://github.com/fabiocicerchia/aws-killswitch/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-Apache_2.0-blue.svg)](LICENSE)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/fabiocicerchia/aws-killswitch/badge)](https://securityscorecards.dev/viewer/?uri=github.com/fabiocicerchia/aws-killswitch)

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

## Documentation

Full docs live in [`docs/`](docs/). Runnable examples live in [`examples/`](examples/).

## License

Apache-2.0 — see [LICENSE](LICENSE).
