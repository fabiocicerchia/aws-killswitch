# TODO

Open items only. Completed work is dropped from here — the CHANGELOG
is the record of what shipped.

- [ ] **No successful run against a real AWS account.** The planner, the state
      machine and the ordering are tested. Discovery has been executed against
      the live AWS endpoints and degrades correctly per service when a call is
      refused — but it has never run with credentials that work, and **no fire
      or restore has ever executed**. Point it at a scratch account first. Three
      kinds and a Lambda front end have been added since that was last true, so
      there is more unproven surface now, not less.
- [ ] Cost estimates beyond NAT gateways
