@domain:recovery @area:fire-status @from:engine/engine-test
Feature: Warning before AWS restarts a stopped database
  Operators need to know when AWS will restart a stopped database on its own, because from then it bills again.

  Background:
    Given a fire at 2026-08-01T12:00:00Z stopped database db-1 and Lambda function fn-1

  @id:recovery.fire-status.004 @rule:recovery.status.database-restart-deadline @verified @source:internal/engine/engine.go#Deadlines @test:internal/engine/engine_test.go#TestDatabaseDeadlinesAreReported
  Scenario Outline: Stopped databases show how long until AWS restarts them, and say when it already has
    Given it is <elapsed> after the fire
    When the operator views the fire's status
    Then exactly one warning is shown, for db-1 only
    And the warning reads "<message>"

    Examples:
      | elapsed  | message                                                                 |
      | 48 hours | rds-instance db-1: AWS restarts this in 5.0 days (2026-08-08T12:00:00Z) |
      | 8 days   | rds-instance db-1: AWS has already restarted this — it is billing again |
