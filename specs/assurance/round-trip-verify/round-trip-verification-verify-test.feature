@domain:assurance @area:round-trip-verify @from:verify/verify-test
Feature: Verifying a fire and restore round trip
  Lets an operator prove, in a scratch account, that a fire really stops each resource and a restore really puts it back, by reading the account back after each step instead of trusting the API calls.

  Background:
    Given a scratch account whose policy puts everything in scope, with databases, instance-store loss and NAT gateway deletion opted in

  @id:assurance.round-trip-verify.009 @rule:assurance.round-trip.clean-cycle-passes @verified @source:internal/verify/verify.go#Run @test:internal/verify/verify_test.go#TestACleanCycleFiresRestoresAndReportsNoDivergence
  Scenario: A cycle where every action lands and every resource comes back passes
    Given the scratch account holds an ECS service with 3 desired tasks, a Lambda function and an auto-scaling group, all running
    When the operator runs a verification against the scratch account
    Then the report's fire line reads "3 changed, 0 failed, 3 verified by a second read"
    And the report reads "No divergence between what was planned and what the account read back."
    And every resource in the account is running again with its pre-fire settings
    And the verification passes

  @id:assurance.round-trip-verify.010 @rule:assurance.round-trip.failed-stop-fails-run-without-halting-fire @verified @source:internal/verify/verify.go#Run @test:internal/verify/verify_test.go#TestAFailedApplyIsCountedAndTheRunContinues
  Scenario: A stop that fails is counted and the rest of the fire still goes ahead
    Given the scratch account holds an ECS service and a Lambda function
    And stopping the ECS service is refused with AccessDenied
    When the operator runs a verification against the scratch account
    Then the report counts 1 failed stop
    And the Lambda function is stopped anyway and counted as changed
    And the verification fails

  @id:assurance.round-trip-verify.011 @rule:assurance.round-trip.restore-failure-stops-restore @inferred @source:internal/verify/verify.go#Run @questionable
  Scenario: A restore that fails stops the remaining restores and skips the final check
    Given the scratch account holds several resources that were stopped by the verification
    And bringing one of them back is refused
    When the operator runs a verification against the scratch account
    Then the report lists an error starting "restore: restoring" that names the resource
    And resources later in restore order are left stopped
    And no restore divergences are listed, because the account is not read back
    And the verification fails

  @id:assurance.round-trip-verify.013 @rule:assurance.round-trip.empty-account-is-not-a-pass @inferred @source:internal/verify/verify.go#Run
  Scenario: A verification over an account with nothing to stop fails instead of passing
    Given the scratch account holds no resource the policy would stop
    When the operator runs a verification against the scratch account
    Then the operator sees "nothing to fire: the scratch account holds no resource this policy would touch"
    And nothing in the account is changed
    And the command exits with a failure
