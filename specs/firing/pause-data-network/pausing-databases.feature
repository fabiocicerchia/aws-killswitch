@domain:firing @area:pause-data-network @from:awsx/execute-data
Feature: Pausing and resuming databases
  Lets an operator who opted databases into a fire stop their running cost without losing data, and bring them back on restore.

  Background:
    Given the policy sets include_databases
    And the database is in scope and available

  @id:firing.pause-data-network.001 @rule:firing.databases.stop-not-delete @inferred @source:internal/awsx/execute_data.go#stopDBInstance
  Scenario Outline: Firing stops an in-scope database without deleting it
    Given a running <kind> named "orders-db" in us-east-1
    When the operator fires the plan
    Then "orders-db" is stopped
    And "orders-db" still exists with its data
    And the fire records "orders-db" as changed

    Examples:
      | kind         |
      | rds-instance |
      | rds-cluster  |

  @id:firing.pause-data-network.002 @rule:recovery.databases.restart-on-restore @inferred @source:internal/awsx/execute_data.go#startDBInstance
  Scenario Outline: Restoring a fire starts each stopped database again
    Given a fire stopped the <kind> named "orders-db"
    When the operator restores that fire
    Then "orders-db" is started
    And the restore records "orders-db" as restored

    Examples:
      | kind         |
      | rds-instance |
      | rds-cluster  |

  @id:firing.pause-data-network.003 @rule:firing.databases.final-snapshot-taken @inferred @source:internal/awsx/execute.go#Executor @questionable
  Scenario Outline: A plan promising a snapshot before stopping a database does not take one
    Given the policy sets final_snapshot
    And the plan shows "snapshot, then stop the database" for the <kind> named "orders-db"
    When the operator fires the plan
    Then "orders-db" is stopped
    And no killswitch snapshot of "orders-db" exists afterwards

    Examples:
      | kind         |
      | rds-instance |
      | rds-cluster  |
