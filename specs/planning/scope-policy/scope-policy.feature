@domain:planning @area:scope-policy @from:policy/policy
Feature: Scoping what the killswitch may hit
  Lets the operator decide, in a policy file written before any incident, exactly which resources a fire may touch, so a typo or a missing setting can never widen the blast to the whole account.

  Background:
    Given the operator runs aws-killswitch with a policy file

  @id:planning.scope-policy.004 @rule:planning.scope.unknown-policy-keys-rejected @inferred @source:internal/policy/policy.go#Parse
  Scenario: A policy file with an unrecognised key is refused
    Given the policy file contains the misspelled key "include_database"
    When the operator plans a fire
    Then the run is refused and the error names the policy file and the unknown field "include_database"
    And nothing is planned or changed
