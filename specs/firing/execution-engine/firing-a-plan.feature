@domain:firing @area:execution-engine @from:engine/engine-test
Feature: Firing a shutdown plan without losing the way back
  Operators firing the killswitch need every change recorded before and after it happens, so a stopped account can always be put back.

  Background:
    Given a reviewed shutdown plan for account 123456789012

  @id:firing.execution-engine.009 @rule:firing.engine.stop-when-record-cannot-be-updated @verified @source:internal/engine/engine.go#Fire @test:internal/engine/engine_test.go#TestFireStopsWhenTheRecordCanNoLongerBeUpdated
  Scenario: A fire stops as soon as the restore record can no longer be updated
    Given the plan stops fn-1, fn-2 and fn-3
    And the state store records the change to fn-1, then fails every later write with "s3: connection reset"
    When the operator fires the plan
    Then the fire stops with an error naming fn-2, the resource it changed but could not record, and telling the operator to run `restore` against the plan
    And no further resources are changed
