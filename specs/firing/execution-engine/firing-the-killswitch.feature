@domain:firing @area:execution-engine @from:engine/engine
Feature: Firing the killswitch with a restore record kept current
  Operators firing the killswitch need every change to be recorded before and as it happens, so an interrupted or partly failed fire can always be put back.

  Background:
    Given the operator has a reviewed shutdown plan for account 123456789012

  @id:firing.execution-engine.001 @rule:firing.engine.no-change-without-verified-record @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestNothingIsTouchedIfTheSnapshotCannotBeWritten @test:internal/engine/engine_test.go#TestSilentlyLossyStoreIsCaughtBeforeAnythingChanges
  Scenario Outline: Nothing is stopped unless the restore record is written and reads back intact
    Given the state store <store_condition>
    When the operator fires the plan
    Then the fire is refused with a message beginning "refusing to fire"
    And no resource in the account is changed

    Examples:
      | store_condition                                                    |
      | refuses the write with "access denied"                             |
      | accepts the write but reads back fewer resources than were written |

  @id:firing.execution-engine.002 @rule:firing.engine.record-updated-after-every-change @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestSnapshotIsUpdatedAfterEveryChange
  Scenario: The restore record lists every resource changed so far, after each change
    Given the plan stops 3 resources: functions fn-1 and fn-2 and auto-scaling group asg-1
    When the operator fires the plan
    Then the fire reports 3 changed
    And the saved restore record lists all 3 resources as changed
    And the record was saved again after each individual change, not only at the end

  @id:firing.execution-engine.003 @rule:firing.partial-failure.continue-and-record-failures @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestPartialFireRecordsOnlyWhatActuallyChanged
  Scenario: One resource that cannot be stopped does not leave the rest running
    Given the plan stops functions fn-1, fn-2 and fn-3
    And AWS throttles the request to stop fn-2
    When the operator fires the plan
    Then fn-1 and fn-3 are stopped
    And the fire reports 2 changed and 1 failed
    And the restore record marks fn-2 as failed with the reason "throttled"
    And a later restore would act on fn-1 and fn-3 only

  @id:firing.execution-engine.004 @rule:firing.engine.halt-when-record-cannot-be-updated @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestFireStopsWhenTheRecordCanNoLongerBeUpdated @questionable
  Scenario: A fire stops changing resources once its restore record can no longer be updated
    Given the plan stops functions fn-1, fn-2 and fn-3
    And the state store starts failing after fn-1 has been stopped
    When the operator fires the plan
    Then the fire stops before stopping fn-3
    And the fire fails with a message telling the operator to run restore against the plan

  @id:firing.execution-engine.005 @rule:firing.engine.dry-run-touches-nothing @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestDryRunWritesNothingAndChangesNothing
  Scenario: A dry run changes nothing and leaves no restore record
    Given the plan stops auto-scaling group asg-1
    When the operator fires the plan as a dry run
    Then no resource in the account is changed
    And no restore record is saved
    And the result reports asg-1 as skipped

  @id:firing.execution-engine.006 @rule:firing.engine.empty-plan-refused @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestEmptyPlanIsRejected
  Scenario: An empty plan is refused rather than fired quietly
    Given the plan contains no actions
    When the operator fires the plan
    Then the fire fails with "plan is empty; nothing to do"
    And no restore record is saved
