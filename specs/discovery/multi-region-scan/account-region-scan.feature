@domain:discovery @area:multi-region-scan @from:awsx/discover
Feature: Scanning the account across regions
  Operators facing a cost incident need discovery to return as much of the account as it can reach, labelled with the account it belongs to, even when some services or lookups fail.

  Background:
    Given the operator's policy scopes the run to one or more regions

  @id:discovery.multi-region-scan.001 @rule:discovery.scan.unknown-account-label @inferred @source:internal/awsx/discover.go#AccountID @questionable
  Scenario: A plan is still produced when the account identity cannot be resolved
    Given the caller identity lookup fails
    When the operator runs plan
    Then the plan header names the account as "unknown"
    And no error is reported for the failed identity lookup
