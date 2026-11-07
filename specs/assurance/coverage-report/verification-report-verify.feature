@domain:assurance @area:coverage-report @from:verify/verify
Feature: Reading a verification report
  Operators paste the verification summary into public issues, so it has to show which resource kinds were really exercised and its counts and findings must never reveal anything that identifies the account.

  Background:
    Given the operator has run a verification against a scratch account

  @id:assurance.coverage-report.002 @rule:assurance.verify-report.unexercised-kinds-named @verified @source:internal/verify/verify.go#AllKinds @test:internal/verify/verify_test.go#TestKindsTheAccountDoesNotHoldAreNamedAsUnexercised
  Scenario: Kinds the scratch account did not hold are reported as not exercised
    Given the scratch account holds only an ECS service
    When the operator reads the verification report
    Then the report marks the other 10 supported kinds as NOT EXERCISED
    And every supported kind appears in the report, including those with zero counts

  @id:assurance.coverage-report.003 @rule:assurance.verify-report.no-identifying-data @verified @source:internal/verify/verify.go#refFor @test:internal/verify/verify_test.go#TestNothingIdentifyingReachesTheReport @test:internal/verify/verify_test.go#TestAReferenceIsStableWithinAReportAndMeaninglessOutsideIt
  Scenario: The verification report never reveals ARNs, account ids or resource names
    Given an EC2 instance named prod-db-primary in account 123456789012 whose stop changes nothing
    When the operator reads the verification report
    Then the finding refers to the instance as ec2-instance#1
    And the report contains no ARN, instance id, account id or resource name
    And the report still names the ec2-instance kind
