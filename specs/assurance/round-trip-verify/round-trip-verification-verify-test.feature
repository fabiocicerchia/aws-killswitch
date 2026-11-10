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

  @id:assurance.round-trip-verify.013 @rule:assurance.round-trip.empty-account-is-not-a-pass @inferred @source:internal/verify/verify.go#Run
  Scenario: A verification over an account with nothing to stop fails instead of passing
    Given the scratch account holds no resource the policy would stop
    When the operator runs a verification against the scratch account
    Then the operator sees "nothing to fire: the scratch account holds no resource this policy would touch"
    And nothing in the account is changed
    And the command exits with a failure
