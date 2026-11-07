@domain:assurance @area:coverage-report @from:verify/verify-test
Feature: Reading a verification report
  Gives an operator a verification summary they can paste into a public issue, which shows what the run did not cover and whose counts and findings leave out anything that identifies the account.

  Background:
    Given a scratch account whose policy puts everything in scope

  @id:assurance.coverage-report.004 @rule:assurance.coverage.report-carries-no-identifiers @verified @source:internal/verify/verify.go#refFor @test:internal/verify/verify_test.go#TestNothingIdentifyingReachesTheReport @test:internal/verify/verify_test.go#TestAReferenceIsStableWithinAReportAndMeaninglessOutsideIt @questionable
  Scenario: A divergence is reported by kind and per-report index, never by ARN, account id or name
    Given the scratch account 123456789012 holds an EC2 instance i-0abc123def456 named prod-db-primary
    And stopping the instance is accepted but changes nothing
    When the operator runs a verification against the scratch account
    Then the divergence names the instance as "ec2-instance#1"
    And the report does not contain the instance id, the account id, the instance name or any ARN
