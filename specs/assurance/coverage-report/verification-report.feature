@domain:assurance @area:coverage-report @from:verify/report
Feature: Reporting a verification run
  Gives the operator a summary of a fire/restore verification run that they can paste into an issue: what it covered, what diverged, and whether the run counts as a pass.

  Background:
    Given a verification run of aws-killswitch against a scratch account has finished

  @id:assurance.coverage-report.001 @rule:assurance.report.every-supported-kind-listed @verified @source:internal/verify/report.go#sortedKinds @test:internal/verify/verify_test.go#TestKindsTheAccountDoesNotHoldAreNamedAsUnexercised
  Scenario: Every supported resource kind gets a row, even when nothing of that kind was found
    Given the scratch account held only ECS services
    When the operator reads the verification report
    Then the report has a row for every supported resource kind, in alphabetical order
    And each row shows how many were found, planned and refused
    And kinds the account did not hold show 0 found, 0 planned, 0 refused
