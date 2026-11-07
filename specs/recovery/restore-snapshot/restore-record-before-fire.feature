@domain:recovery @area:restore-snapshot @from:state/state-test
Feature: Recording how to put the account back before firing
  Operators need to know a fire will never start until a complete, readable record of every resource's prior settings exists, so pressing the killswitch can always be undone.

  Background:
    Given the operator has a reviewed plan for account 123456789012

  @id:recovery.restore-snapshot.004 @rule:recovery.restore-record.verified-before-any-change @verified @source:internal/state/state.go#PutVerified @test:internal/state/state_test.go#TestPutVerifiedCatchesALossyStore
  Scenario Outline: The killswitch refuses to fire when the restore record cannot be read back in full
    Given the configured state store <store_behaviour>
    When the operator fires the plan
    Then the fire is refused with "refusing to fire" and the reason "<reason>"
    And no resource in the account is changed

    Examples:
      | store_behaviour                                        | reason                                                                                             |
      | accepts the record but returns it with entries missing | snapshot in <store> has 0 entries, wrote 2 — refusing to proceed without a reliable restore record |
      | accepts the record but cannot read it back             | snapshot written to <store> but could not be read back                                             |
      | rejects the write                                      | writing snapshot to <store>                                                                        |

  @id:recovery.restore-snapshot.005 @rule:recovery.restore-record.covers-every-planned-action @verified @source:internal/state/state.go#From @test:internal/state/state_test.go#TestFromPlanCarriesEveryActionsPriorState
  Scenario: Every resource the plan will stop is in the restore record with its prior settings
    Given the plan stops auto-scaling group asg-1 with desired capacity 4
    And the plan blocks load balancer listener l-1 that forwarded traffic
    When the operator fires the plan
    Then the restore record holds 2 entries
    And each entry carries the settings the resource had before the fire

  @id:recovery.restore-snapshot.006 @rule:recovery.restore-record.prior-numbers-preserved @verified @source:internal/model/model.go#PriorInt @test:internal/state/state_test.go#TestLocalStoreRoundTripsPriorStateAsNumbers
  Scenario: Recorded capacity numbers come back as the same whole numbers
    Given auto-scaling group asg-a had desired capacity 3, minimum 1 and maximum 6 before the fire
    And its restore record has been saved
    When the operator reads the restore record back to restore
    Then the record gives desired capacity 3, minimum 1 and maximum 6
