@domain:recovery @area:fire-status @from:engine/engine
Feature: Warning when AWS will restart a stopped database
  Operators need to know when AWS's seven-day automatic restart will bring a stopped database back to billing.

  Background:
    Given a fire on 2026-08-01 12:00 UTC stopped database db-1 and function fn-1

  @id:recovery.fire-status.003 @rule:recovery.status.database-auto-restart-deadline @verified @source:internal/engine/engine.go#Deadlines @test:internal/engine/engine_test.go#TestDatabaseDeadlinesAreReported
  Scenario Outline: Stopped databases show when AWS restarts them, or that it already has
    Given it is <elapsed> after the fire
    When the operator views the fire's deadlines
    Then only db-1 is listed, not fn-1
    And db-1 shows "<message>"

    Examples:
      | elapsed | message                                              |
      | 2 days  | AWS restarts this in 5.0 days (2026-08-08T12:00:00Z) |
      | 8 days  | AWS has already restarted this — it is billing again |
