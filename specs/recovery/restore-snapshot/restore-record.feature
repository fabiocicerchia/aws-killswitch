@domain:recovery @area:restore-snapshot @from:state/state
Feature: Keeping a trustworthy restore record
  Operators firing the killswitch need a restore record that is known to be complete and findable before anything in the account is changed, so a stopped account can always be brought back.

  Background:
    Given the operator has reviewed a shutdown plan for account 123456789012

  @id:recovery.restore-snapshot.001 @rule:recovery.snapshot.fire-refused-without-verified-record @verified @source:internal/state/state.go#PutVerified @test:internal/state/state_test.go#TestPutVerifiedCatchesALossyStore @questionable
  Scenario Outline: The fire is refused unless the restore record is written and read back intact
    Given the restore record store <store_behaviour>
    When the operator fires the plan
    Then the fire is refused with an error naming the store
    And no resource in the account is changed

    Examples:
      | store_behaviour                                         |
      | rejects the write                                       |
      | accepts the write but cannot return it                  |
      | returns the record with fewer entries than were written |

  @id:recovery.restore-snapshot.002 @rule:recovery.snapshot.records-prior-state-of-every-action @verified @source:internal/state/state.go#From @test:internal/state/state_test.go#TestFromPlanCarriesEveryActionsPriorState
  Scenario: The restore record carries the prior state of every planned action
    Given the plan scales auto-scaling group asg-1 in eu-west-1 from a desired capacity of 4 and replaces the default actions of listener l-1
    When the operator fires the plan
    Then the restore record holds one entry per planned action
    And each entry names its kind, id, region and phase, plus the prior state to put back

  @id:recovery.restore-snapshot.003 @rule:recovery.snapshot.lambda-refuses-without-durable-store @inferred @source:internal/state/state.go#Build
  Scenario Outline: The serverless trigger refuses to fire without a usable S3 state location
    Given the policy's state_uri is <state_uri>
    When the serverless trigger runs a trip
    Then the trip is refused with "refusing to fire without a restore path"
    And no resource in the account is changed

    Examples:
      | state_uri           |
      | empty               |
      | /var/lib/killswitch |
