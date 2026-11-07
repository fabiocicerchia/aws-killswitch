# Specifications of aws-killswitch

What this code does, one atomic Gherkin scenario per behaviour, read from the code by code-atlas.
Tags: `@verified` a test asserts it · `@inferred` read from the code only · `@gap` an entry point no test covers · `@human` written or amended by a person, never rewritten.

## Audit and accountability
An answerable record of what the tool did, when, and by whom.

- **Audit log**: [Keeping an answerable audit log of what the killswitch did](accountability/audit-log/audit-log.feature) (4)
- **Required IAM permissions**: [Publishing the IAM policies the tool needs](accountability/required-permissions/published-iam-policies.feature) (1), [Least-privilege IAM policies for planning, firing and the Lambda trigger](accountability/required-permissions/shipped-iam-policies.feature) (6)

## Assurance and verification
Proving that fire followed by restore returns the account to exactly where it started.

- **Verification report**: [Reading a verification report](assurance/coverage-report/verification-report-verify-test.feature) (1), [Reading a verification report](assurance/coverage-report/verification-report-verify.feature) (2), [Reporting a verification run](assurance/coverage-report/verification-report.feature) (1)
- **Round-trip verification**: [Verifying a scratch account from the command line](assurance/round-trip-verify/cli-verify-scratch-account.feature) (4), [Verifying a fire and restore round trip](assurance/round-trip-verify/round-trip-verification-verify-test.feature) (4), [Verifying a fire and restore round trip](assurance/round-trip-verify/round-trip-verification.feature) (4)
- **Scratch test account**: [Provisioning a scratch account for verification](assurance/scratch-account/scratch-account.feature) (4)

## Automated triggering
Firing the killswitch without a human at the keyboard when spending crosses a line.

- **Serverless trigger**: [Firing the killswitch from a budget alert](automation/lambda-trigger/lambda-budget-trigger.feature) (7)
- **Trip cycle**: [Refusing a duplicate trip after a recent fire](automation/trip-cycle/trip-cooldown.feature) (3), [Running one killswitch trip](automation/trip-cycle/trip-cycle.feature) (8)

## Command-line experience
How operators drive the tool from a terminal.

- **Commands and flags**: [Firing and restoring from the command line](cli/commands/cli-fire-and-restore.feature) (8)
- **Configuration and credentials**: [Loading the policy, credentials and restore-record location](cli/configuration-wiring/cli-policy-and-state-location.feature) (4)
- **Exit codes**: [Invocation errors and exit codes](cli/exit-codes/cli-exit-codes.feature) (4)
- **Output rendering**: [Reading a shutdown plan](cli/output-rendering/cli-plan-output.feature) (5), [Naming discovered resources in the plan](cli/output-rendering/resource-display-names.feature) (1)

## Cost visibility
Showing what the account is spending and what a fire would save.

- **Current spend**: [Checking month-to-date spend against a threshold](cost/current-spend/cli-spend-threshold.feature) (4), [Reporting month-to-date spend](cost/current-spend/month-to-date-spend.feature) (4)
- **Savings estimates**: [Estimating the hourly saving of a plan](cost/savings-estimates/hourly-savings-estimates.feature) (5), [Estimating the hourly saving of a shutdown](cost/savings-estimates/savings-estimates.feature) (5)

## Resource discovery
Finding everything running in the account that costs money or accepts traffic, and recording its prior state.

- **Data store discovery**: [Finding databases that can be paused](discovery/data-discovery/database-discovery.feature) (4)
- **Ingress discovery**: [Finding the public entry points that let traffic in](discovery/ingress-discovery/ingress-discovery.feature) (3)
- **Account and region scanning**: [Scanning the account across regions](discovery/multi-region-scan/account-region-scan.feature) (1)
- **Network discovery**: [Discovering NAT gateways and the routes that depend on them](discovery/network-discovery/nat-gateway-discovery.feature) (6)

## Firing the killswitch
Executing an approved plan to cut ingress and drain compute while keeping every change reversible.

- **Cutting ingress**: [Cutting and restoring public entry points](firing/cut-ingress/cutting-ingress.feature) (7)
- **Draining compute**: [Draining and restoring compute workloads](firing/drain-compute/drain-compute.feature) (4)
- **Execution engine**: [Firing a shutdown plan without losing the way back](firing/execution-engine/firing-a-plan.feature) (3), [Firing the killswitch with a restore record kept current](firing/execution-engine/firing-the-killswitch.feature) (6)
- **Pausing data and network services**: [Pausing and resuming databases](firing/pause-data-network/pausing-databases.feature) (3)

## Shutdown planning
Turning discovered resources into a reviewed, ordered, reversible proposal before anything changes.

- **Action catalog**: [Applying and undoing each stop action](planning/action-catalog/applying-and-undoing-stop-actions.feature) (2)
- **Plan building**: [Building a shutdown plan that is safe to run and to reverse](planning/plan-building/plan-building.feature) (5), [Building a shutdown plan](planning/plan-building/shutdown-plan-building.feature) (4)
- **Protected resources**: [Never touching stateful storage](planning/protected-resources/never-touch-stateful-storage.feature) (3), [Protecting resources with the protect tag](planning/protected-resources/protect-tag.feature) (1), [Protecting resources from the killswitch](planning/protected-resources/protected-and-skipped-resources.feature) (4), [Deciding what the killswitch leaves alone](planning/protected-resources/protected-resources.feature) (3)
- **Scope policy**: [Writing down what an account allows before an incident](planning/scope-policy/scope-policy-opt-ins.feature) (3), [Scoping what the killswitch may hit](planning/scope-policy/scope-policy.feature) (1)

## Restore and recovery
Recording exactly how to put everything back, and putting it back.

- **Fire status**: [Listing past fires](recovery/fire-status/cli-fire-status.feature) (2), [Warning before AWS restarts a stopped database](recovery/fire-status/database-auto-restart-warning.feature) (1), [Warning when AWS will restart a stopped database](recovery/fire-status/database-restart-deadlines.feature) (1)
- **Restoring an account**: [Deleting and recreating NAT gateways](recovery/restore-run/nat-gateway-delete-and-recreate.feature) (6), [Restoring only to the exact recorded prior state](recovery/restore-run/restore-refuses-to-guess-prior-state.feature) (4), [Restoring an account after a fire](recovery/restore-run/restoring-an-account-engine-test.feature) (1), [Restoring an account from its restore record](recovery/restore-run/restoring-an-account.feature) (4), [Restoring exactly what was recorded](recovery/restore-run/restoring-recorded-state.feature) (3)
- **Restore snapshot**: [Recording how to put the account back before firing](recovery/restore-snapshot/restore-record-before-fire.feature) (3), [Keeping a trustworthy restore record](recovery/restore-snapshot/restore-record.feature) (3)
- **State storage backends**: [Keeping restore records in S3](recovery/state-storage/s3-restore-records.feature) (1)
