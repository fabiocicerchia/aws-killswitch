@domain:assurance @area:round-trip-verify @from:verify/verify
Feature: Verifying a fire and restore round trip
  Operators run a real fire and restore against a scratch account and read the account back, so they can trust the killswitch before an incident instead of trusting that the API accepted the calls.

  Background:
    Given a scratch account whose policy puts every resource in scope
    And the operator has started a verification run for one region

  @id:assurance.round-trip-verify.005 @rule:assurance.round-trip.failed-stop-does-not-halt-fire @verified @source:internal/verify/verify.go#Run @test:internal/verify/verify_test.go#TestAFailedApplyIsCountedAndTheRunContinues
  Scenario: One resource that fails to stop does not stop the rest
    Given an ECS service and a Lambda function
    And stopping the ECS service fails with AccessDenied
    When the operator runs the verification
    Then the report counts 1 failed stop
    And the Lambda function is stopped anyway and counted as changed
    And the verification does not pass

  @id:assurance.round-trip-verify.006 @rule:assurance.round-trip.restore-halts-on-first-failure @inferred @source:internal/verify/verify.go#Run @questionable
  Scenario: Verification stops restoring at the first resource that fails to restore
    Given the killswitch has stopped several resources during the verification
    And restoring one of them fails
    When the operator runs the verification
    Then the report counts the restores done so far and 1 failed
    And the report's errors include one beginning 'restore: restoring'
    And resources after the failed one are left stopped
    And no restore read-back is reported

  @id:assurance.round-trip-verify.007 @rule:assurance.round-trip.plan-only-changes-nothing @verified @source:internal/verify/verify.go#Run @test:internal/verify/verify_test.go#TestSkipFirePlansAndTouchesNothing
  Scenario: A plan-only verification reports the plan and changes nothing
    Given an ECS service that is running
    When the operator runs the verification in plan-only mode
    Then the report shows the ECS service as planned
    And the report shows 0 changed
    And the ECS service is still running

  @id:assurance.round-trip-verify.008 @rule:assurance.round-trip.empty-plan-is-an-error @inferred @source:internal/verify/verify.go#Run
  Scenario: A scratch account with nothing to stop is refused rather than passed
    Given a scratch account holding no resource the policy would stop
    When the operator runs the verification
    Then the operator sees 'nothing to fire: the scratch account holds no resource this policy would touch'
    And nothing is fired or restored
