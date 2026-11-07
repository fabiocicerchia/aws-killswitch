@domain:firing @area:execution-engine @from:engine/engine-test
Feature: Firing a shutdown plan without losing the way back
  Operators firing the killswitch need every change recorded before and after it happens, so a stopped account can always be put back.

  Background:
    Given a reviewed shutdown plan for account 123456789012

  @id:firing.execution-engine.007 @rule:firing.engine.record-before-any-change @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestNothingIsTouchedIfTheSnapshotCannotBeWritten @test:internal/engine/engine_test.go#TestSilentlyLossyStoreIsCaughtBeforeAnythingChanges
  Scenario Outline: Nothing is stopped unless the restore record is safely stored first
    Given the state store <store_problem>
    When the operator fires the plan
    Then the fire is refused with an error starting "refusing to fire"
    And no resource in the account is changed

    Examples:
      | store_problem                                    |
      | refuses the write (s3: access denied)            |
      | accepts the write but drops the recorded entries |

  @id:firing.execution-engine.009 @rule:firing.engine.stop-when-record-cannot-be-updated @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestFireStopsWhenTheRecordCanNoLongerBeUpdated
  Scenario: A fire stops as soon as the restore record can no longer be updated
    Given the plan stops fn-1, fn-2 and fn-3
    And the state store starts failing (s3: connection reset) after fn-1 is changed
    When the operator fires the plan
    Then the fire stops with an error naming the changed resource and telling the operator to run `restore` against the plan
    And no further resources are changed

  @id:firing.execution-engine.010 @rule:firing.partial-failure.only-successes-are-restorable @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestPartialFireRecordsOnlyWhatActuallyChanged
  Scenario: A resource that fails to stop does not halt the fire and is never restored
    Given the plan stops fn-1, fn-2 and fn-3
    And stopping fn-2 fails with "throttled"
    When the operator fires the plan
    Then the fire reports 2 changed and 1 failed
    And fn-2 is recorded as failed
    And a later restore would act on fn-1 and fn-3 only
