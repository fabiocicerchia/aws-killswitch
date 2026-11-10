@domain:recovery @area:restore-snapshot @from:state/state
Feature: Keeping a trustworthy restore record
  Operators firing the killswitch need a restore record that is known to be complete and findable before anything in the account is changed, so a stopped account can always be brought back.

  Background:
    Given the operator has reviewed a shutdown plan for account 123456789012

  @id:recovery.restore-snapshot.002 @rule:recovery.snapshot.records-prior-state-of-every-action @verified @source:internal/state/state.go#From @test:internal/state/state_test.go#TestFromPlanCarriesEveryActionsPriorState
  Scenario: The restore record carries the prior state of every planned action
    Given the plan scales auto-scaling group asg-1 in eu-west-1 from a desired capacity of 4 and replaces the default actions of listener l-1
    When the operator fires the plan
    Then the restore record holds one entry per planned action
    And each entry names its kind, id, region and phase, plus the prior state to put back
