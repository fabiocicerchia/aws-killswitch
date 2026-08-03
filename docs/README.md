# Documentation

- [Getting Started](getting-started.md) — the policy you have to write first,
  the state bucket, a read-only plan, and firing at a scratch account.
- [Architecture](architecture.md) — the four stages, the write-before-change
  rule between the third and the fourth, and the invariants that follow.
- [IAM policy (read-only)](iam-plan-readonly.json) — enough for `plan` and
  `spend`, and nothing that can change anything.
- [IAM policy (fire)](iam-fire.json) — includes the explicit `Deny` on
  stateful storage, so the tool cannot touch it even if the code were wrong.

The [README](../README.md) covers what it does and refuses to do. These pages
cover firing it without regretting it.
