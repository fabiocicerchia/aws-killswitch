@domain:recovery @area:restore-run @from:engine/engine-test
Feature: Restoring an account after a fire
  Operators restoring a fired account need traffic to return only after its backends do, and need a restore to never replay twice.

  Background:
    Given plan plan-1 was fired and its restore record is stored

  @id:recovery.restore-run.012 @rule:recovery.restore.never-replayed @verified @source:internal/engine/engine.go#Restore @test:internal/engine/engine_test.go#TestRestoreIsIdempotent
  Scenario: A second restore of the same fire is refused and changes nothing
    Given plan-1 has already been restored
    When the operator restores plan-1 again
    Then the restore is refused with "this snapshot records no changes to undo"
    And no resource is restored a second time
