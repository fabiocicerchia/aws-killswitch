# Architecture

Four stages, and a hard rule between the third and the fourth.

```
awsx.Discover ──► plan.Build ──► state.PutVerified ──► awsx.Executor
  (read-only)       (pure)        (must succeed)         (mutates)
```

Nothing in the fourth stage runs until the third has written the restore record
and read it back.

## Packages

| Package | Responsibility |
|---|---|
| `internal/model` | The vocabulary: kinds, phases, the never-touch set, the snapshot format. |
| `internal/policy` | Scope, protection, thresholds. Refuses to validate an unscoped policy. |
| `internal/plan` | Decides what happens and in what order. Pure. |
| `internal/state` | Snapshot persistence: S3, local, and both together. |
| `internal/engine` | Executes a plan and enforces the write-first rule. |
| `internal/awsx` | The only package that talks to AWS. |
| `internal/audit` | Append-only record of everything attempted. |
| `internal/trip` | One discover → plan → fire cycle, with nothing in it that belongs to a particular way of being invoked. |

## The invariants

**The restore record is written before anything changes, and verified.**
`state.PutVerified` writes, reads back, and compares the entry count. A store
that accepts a write and silently loses it is the one failure that cannot be
recovered from, and it is cheap to rule out. If verification fails, the fire is
abandoned and nothing has been touched.

**The record is updated after every change, not at the end.** A fire interrupted
by a dropped connection, an expired token or a `ctrl-C` must still leave an
exact record of which resources were changed. If the record can no longer be
written mid-fire, the engine stops rather than changing more things it can no
longer undo.

**Ingress before compute; the reverse on restore.** Draining a fleet while
traffic still arrives means a target-tracking policy fights the teardown.
Bringing traffic back before compute means the first request lands on nothing.

**Stateful storage is unreachable.** `model.NeverTouch` is a deny list, checked
before scope and before any flag. A deny list rather than an allow list because
allow lists grow by accident: someone adds a resource kind, forgets the
exclusion, and a cost tool deletes a bucket.

## Why `internal/plan` is pure

Every safety decision is made there — what is refused, in what order things
happen, what requires acknowledgement — and none of it touches AWS. That is what
lets the rules be tested: that a protected tag beats `scope.everything`, that an
instance with local NVMe is refused, that an ASG is scaled before a loose
instance is stopped so the group does not replace it.

## Adding a resource kind

1. A `Kind` in `internal/model`, and a phase for it.
2. A case in `plan.actionFor` — including what makes it *refuse*, which is
   usually the more important half. A kind whose refusal is more than a line
   gets its own function next to it, as `instanceAction` and `databaseAction`
   already do.
3. Discovery in the matching `internal/awsx/discover_<phase>.go`, recording enough prior state to
   restore it exactly.
4. `Apply` and `Restore` in `internal/awsx/execute.go`.
5. A test in `internal/plan` for the refusal, and one for the ordering if it
   interacts with an existing kind.

Step 3 is where mistakes hide. The question to answer is not "what do I need to
stop this" but "what would I need to know to put it back exactly as it was" —
Lambda is the example: recording the reserved concurrency is not enough, because
a function that had *no* reservation must have the reservation removed rather
than set to a number.

## The restore is the product

Stopping an account is easy and useless on its own. What makes this safe to fire
at two in the morning is that the exact prior state — every desired count,
concurrency reservation, ASG bound and listener rule — is written durably
**before anything changes**, and updated after every single change.

If the snapshot cannot be written, the fire is abandoned and nothing is touched.
An account that is still expensive is a problem; an account that is stopped with
no record of how to start it is an outage of unknown length.

That invariant, and the ones below, are what the tests are about.

## What the saving estimate means

The plan prints an hourly saving per kind and in total, from list-price
constants in `internal/awsx/pricing.go` — not from the Pricing API. The trip
path runs when an account is already on fire; adding a network call and a
failure mode to it, to refine a number nobody acts on in the moment, is the
wrong trade.

Read the figure as a floor, not a quote:

- **us-east-1, on-demand, Linux.** Other regions run roughly 5-25% higher, and
  Windows or commercial-database licensing can double a figure.
- **Compute only.** Storage, data transfer, provisioned IOPS and snapshots keep
  costing while an instance is stopped, and none of them are modelled.
- **RDS is priced single-AZ.** A Multi-AZ deployment bills roughly double.

One price per instance family is enough because on-demand pricing is linear in
vCPU within a family: each size step doubles both. Families that break that
pattern are left out of the table rather than approximated.

A kind, family or size the table does not know reports **not estimated**, and
is counted separately from the total. That distinction is deliberate: folding
an unpriced resource in as zero would assert that stopping it is free, which is
a different claim from not knowing what it costs.

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

## What it covers

| | Stop | Restore |
|---|---|---|
| ALB/NLB listener | fixed 503 response | prior default actions, verbatim |
| Lambda | reserved concurrency 0 | prior reservation, or *removed* if there was none |
| ECS service | desired count 0 | prior count |
| Auto Scaling group | min/desired/max 0 | all three prior bounds |
| EC2 instance | stop (EBS kept) | start |
| EKS managed nodegroup | min/desired node count 0 | prior min and desired |
| CloudFront distribution | disable | re-enable |
| API Gateway stage | stage-wide throttle to 0 rps | prior rate and burst, or the override *removed* if there was none |
| NAT gateway | delete (opt-in) | recreate with the same Elastic IP, and repoint every route |
| RDS instance / cluster | stop (opt-in) | start |

Four of those are worth calling out. A Lambda that had *no* reservation must have
the reservation removed on restore, not set to a number — recording which case
it was is the difference between restoring it and quietly changing its
behaviour. **An API Gateway stage is the same shape of problem**: a stage with
no default throttle is running at the *account* limit, and "restoring" it to
today's account limit as a number would pin it there forever — so the absence is
recorded as `-1` and restore removes the override instead of writing one.

And recreating a NAT gateway without repointing the route tables
leaves private subnets with no egress and a restore that reported success, so
the routes are recorded at discovery and replayed.

**An EKS nodegroup's `maxSize` is deliberately not touched.** EKS rejects a
nodegroup whose max is 0, so zeroing it would fail the fire outright; and a
value nobody changed needs no putting back. Scaling to zero only needs min and
desired.

**CloudFront is global**, so exactly one region's client walks it — otherwise a
run across five regions finds every distribution five times and plans five
identical disables. Its resources record `region: "global"`, which the executor
routes deliberately rather than looking up a region that does not exist. And a
distribution is disabled by reading the whole config, changing one field, and
writing it back with the ETag that came with the read: the ETag is re-fetched
at fire time rather than recorded at discovery, because it changes on every
update and a stale one fails with `PreconditionFailed`.
