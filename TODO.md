# TODO

Open items only. Completed work is dropped from here — the CHANGELOG
is the record of what shipped.

- [ ] **No successful run against a real AWS account.** The planner, the state
      machine and the ordering are tested. Discovery has been executed against
      the live AWS endpoints and degrades correctly per service when a call is
      refused — but it has never run with credentials that work, and **no fire
      or restore has ever executed**. Point it at a scratch account first.
- [ ] EKS managed nodegroups, CloudFront, API Gateway stage throttling
- [ ] A Budgets-action Lambda wrapper, so the fast trip does not need a cron
- [ ] Cost estimates beyond NAT gateways
