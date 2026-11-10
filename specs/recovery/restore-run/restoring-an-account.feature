@domain:recovery @area:restore-run @from:engine/engine
Feature: Restoring an account from its restore record
  Operators restoring after a fire need resources put back in a safe order, exactly once, with failures left restorable.

  Background:
    Given a fire of plan plan-1 recorded which resources it changed

  @id:recovery.restore-run.007 @rule:recovery.restore.reverse-phase-order @verified @source:internal/engine/engine.go#Restore @test:internal/engine/engine_test.go#TestRestoreRunsInReversePhaseOrder
  Scenario: Restore brings data and compute back before traffic is let in
    Given the fire closed listener listener-1, scaled down auto-scaling group asg-1 and stopped database db-1
    When the operator restores plan-1
    Then db-1 is restored first, then asg-1, then listener-1

  @id:recovery.restore-run.009 @rule:recovery.restore.stop-on-first-failure @inferred @source:internal/engine/engine.go#Restore @questionable
  Scenario: A restore that hits a failure stops there and leaves the rest restorable
    Given the fire stopped database db-1 and scaled down auto-scaling group asg-1
    And restoring db-1 fails with "access denied"
    When the operator restores plan-1
    Then the restore fails with "restoring db-1 failed: access denied"
    And asg-1 is not restored
    And the restore record still lists db-1 and asg-1 as changed, with the reason for db-1
    And the fire is not marked as restored

  @id:recovery.restore-run.010 @rule:recovery.restore.dry-run-touches-nothing @inferred @source:internal/engine/engine.go#Restore
  Scenario: A dry-run restore changes nothing
    Given the fire changed 3 resources
    When the operator restores plan-1 as a dry run
    Then no resource is restored
    And the result reports 3 skipped
    And the restore record is unchanged
