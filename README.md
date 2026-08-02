# aws-killswitch

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

## The restore is the product

Stopping an account is easy and useless on its own. What makes this safe to fire
at two in the morning is that the exact prior state — every desired count,
concurrency reservation, ASG bound and listener rule — is written durably
**before anything changes**, and updated after every single change.

If the snapshot cannot be written, the fire is abandoned and nothing is touched.
An account that is still expensive is a problem; an account that is stopped with
no record of how to start it is an outage of unknown length.

That invariant, and the ones below, are what the tests are about.

## What it will not do

**It never deletes.** Everything is a stop, a scale to zero, or a throttle. The
sole exception is a NAT gateway, which AWS provides no way to stop, and that is
opt-in and says so.

**Stateful storage is untouchable.** S3, EFS, FSx, DynamoDB, EBS volumes,
snapshots, AMIs, backup vaults, KMS, IAM, CloudTrail. Not behind a flag — there
is no flag. The shipped IAM policy adds an explicit `Deny` on those actions so
the tool cannot do it even if the code were wrong.

**It refuses without a policy.** No config, no action. A cost tool that defaults
to "the whole account" is one typo away from being the outage.

**Anything tagged `killswitch:protect` is out**, whatever the scope says, and
only an explicit `false`/`no`/`0` turns that off. A resource tagged protected
with an empty value was tagged for a reason.

## Three traps it handles

**Databases restart themselves.** AWS starts a stopped RDS instance back up
after seven days. A kill switch that silently un-kills a week later is worse
than none, because nobody is watching by then. Databases are excluded by
default; when included, the plan warns, and `status` counts down:

```
ks-20260802-021500  fired 2026-08-02T02:15:00Z  6 changed of 6
    ! rds-instance orders-prod: AWS restarts this in 5.0 days (2026-08-09T02:15:00Z)
```

**Stopping an instance erases its instance store.** EBS survives, local NVMe
does not, and nothing in the stop API warns you. Refused by default; allowing it
still requires acknowledging the loss per instance.

**Ingress goes before compute.** Drain the fleet while traffic still arrives and
a target-tracking policy fights the teardown while health checks alarm. Restore
runs the reverse — compute back before traffic is let in, or the first request
lands on nothing.

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

## Triggering it

`spend` reads month-to-date cost from Cost Explorer and exits 3 above the
threshold, which is enough for a cron:

```sh
aws-killswitch spend --threshold 500 || aws-killswitch fire --yes
```

**But Cost Explorer lags by hours to a day.** That is fine for "the bill is
running away over a week" and useless for "something spiked twenty minutes ago".
For a fast trip, drive the CLI from an AWS Budgets action or a CloudWatch alarm
on `EstimatedCharges` rather than polling from here. The tool says so at runtime
rather than letting a stale number look authoritative.

## What it covers

| | Stop | Restore |
|---|---|---|
| ALB/NLB listener | fixed 503 response | prior default actions, verbatim |
| Lambda | reserved concurrency 0 | prior reservation, or *removed* if there was none |
| ECS service | desired count 0 | prior count |
| Auto Scaling group | min/desired/max 0 | all three prior bounds |
| EC2 instance | stop (EBS kept) | start |
| NAT gateway | delete (opt-in) | recreate with the same Elastic IP, and repoint every route |
| RDS instance / cluster | stop (opt-in) | start |

Two of those are worth calling out. A Lambda that had *no* reservation must have
the reservation removed on restore, not set to a number — recording which case
it was is the difference between restoring it and quietly changing its
behaviour. And recreating a NAT gateway without repointing the route tables
leaves private subnets with no egress and a restore that reported success, so
the routes are recorded at discovery and replayed.

## IAM

Two policies in `docs/`, deliberately split: `iam-plan-readonly.json` is
everything `plan`, `status` and `spend` need and cannot change a thing —
attach it widely. `iam-fire.json` is the write half, tag-conditioned, with an
explicit `Deny` on every destructive action.

## Status

- [x] Ingress cut, compute drained, in that order
- [x] Snapshot written and read back before anything changes; updated per action
- [x] Never-touch set, protect tag, tag scoping, blast-radius threshold
- [x] The seven-day RDS fuse tracked and counted down
- [x] Instance-store loss refused by default
- [x] NAT gateway restore that repoints routes and keeps the address
- [x] Append-only audit log
- [ ] **No successful run against a real AWS account.** The planner, the state
      machine and the ordering are tested. Discovery has been executed against
      the live AWS endpoints and degrades correctly per service when a call is
      refused — but it has never run with credentials that work, and **no fire
      or restore has ever executed**. Point it at a scratch account first.
- [ ] EKS managed nodegroups, CloudFront, API Gateway stage throttling
- [ ] A Budgets-action Lambda wrapper, so the fast trip does not need a cron
- [ ] Cost estimates beyond NAT gateways

## Development

```sh
make test   # go test ./...
make lint   # vet + gofmt
```

`internal/plan` decides what happens and is pure. `internal/engine` enforces the
ordering and the write-first rule. `internal/awsx` is the only package that
talks to AWS. The first two hold the tests, because they hold the decisions.

## License

Apache-2.0 — see [LICENSE](LICENSE).
