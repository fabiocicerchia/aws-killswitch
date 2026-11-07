@domain:recovery @area:fire-status @from:cmd/aws-killswitch
Feature: Listing past fires
  Shows the operator every recorded fire, whether it has been restored, and any deadlines, so they know what can be put back.

  Background:
    Given a valid policy file exists

  @id:recovery.fire-status.001 @rule:recovery.status.lists-records @inferred @source:cmd/aws-killswitch/commands.go#cmdStatus
  Scenario: Status lists each recorded plan with its state and deadlines
    Given plan 20261107-104500 fired 3 of 4 resources in account 123456789012 and was not restored
    When the operator runs status
    Then "20261107-104500  fired 2026-11-07T10:45:00Z  3 changed of 4  (123456789012)" is printed
    And each pending deadline is listed beneath on a "    ! " line

  @id:recovery.fire-status.002 @rule:recovery.status.empty-store-message @inferred @source:cmd/aws-killswitch/commands.go#cmdStatus
  Scenario: Status with no records says nothing has been fired from here
    Given the state store holds no records
    When the operator runs status
    Then "no snapshots in <store> — nothing has been fired from here" is printed
    And the process exits 0
