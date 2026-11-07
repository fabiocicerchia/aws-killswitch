@domain:assurance @area:round-trip-verify @from:cmd/aws-killswitch
Feature: Verifying a scratch account from the command line
  Lets an operator or CI job prove fire-and-restore works on a scratch account, refusing to run casually and failing loudly on divergence.

  Background:
    Given a valid policy file scopes a scratch account

  @id:assurance.round-trip-verify.001 @rule:cli.verify.requires-explicit-consent @inferred @source:cmd/aws-killswitch/main.go#cmdVerify @questionable
  Scenario: Verify refuses to run without --yes or --plan-only
    Given the policy names one region
    When the operator runs verify with neither --yes nor --plan-only
    Then the error "verify fires and restores for real: pass --yes, or --plan-only to see what it would do" is shown
    And nothing is fired
    And the process exits 1

  @id:assurance.round-trip-verify.002 @rule:cli.verify.single-region-only @inferred @source:cmd/aws-killswitch/main.go#cmdVerify
  Scenario: Verify refuses a policy that names more than one region
    Given the policy names regions eu-west-1 and us-east-1
    When the operator runs verify --yes
    Then the error "verify runs one region at a time; this policy names 2" is shown
    And nothing is fired
    And the process exits 1

  @id:assurance.round-trip-verify.003 @rule:cli.verify.announces-target-first @inferred @source:cmd/aws-killswitch/main.go#cmdVerify
  Scenario: Verify names the account and region before acting
    Given the policy names region eu-west-1 in account 123456789012
    When the operator runs verify --yes
    Then "aws-killswitch verify — account 123456789012, region eu-west-1" is printed to stderr
    And "This FIRES and then RESTORES, for real. Use a scratch account." is printed before any change

  @id:assurance.round-trip-verify.004 @rule:cli.verify.divergence-fails-run @inferred @source:cmd/aws-killswitch/main.go#cmdVerify
  Scenario: A verification that finds divergences fails the process
    Given a resource did not read back as it was before the fire
    When the operator runs verify --yes
    Then the verification report is printed, as JSON with --json
    And the error "verification found divergences; each is its own bug to file" is shown
    And the process exits 1
