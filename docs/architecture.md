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
   usually the more important half.
3. Discovery in `internal/awsx/discover.go`, recording enough prior state to
   restore it exactly.
4. `Apply` and `Restore` in `internal/awsx/execute.go`.
5. A test in `internal/plan` for the refusal, and one for the ordering if it
   interacts with an existing kind.

Step 3 is where mistakes hide. The question to answer is not "what do I need to
stop this" but "what would I need to know to put it back exactly as it was" —
Lambda is the example: recording the reserved concurrency is not enough, because
a function that had *no* reservation must have the reservation removed rather
than set to a number.
