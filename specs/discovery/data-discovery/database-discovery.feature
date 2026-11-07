@domain:discovery @area:data-discovery @from:awsx/discover-data
Feature: Finding databases that can be paused
  Gives the operator a list of RDS databases and Aurora clusters, each stoppable as one unit, with a price where one can be known, so the plan only proposes database stops that will work.

  Background:
    Given the account has RDS databases in region "eu-west-1"

  @id:discovery.data-discovery.001 @rule:data.discovery.cluster-is-the-stop-unit @inferred @source:internal/awsx/discover_data.go#databases
  Scenario: An Aurora cluster is listed once, not once per member instance
    Given an Aurora cluster "orders-aurora" with member instances "orders-aurora-1" and "orders-aurora-2"
    And a standalone database instance "billing-db"
    When the operator runs a plan
    Then the plan lists "orders-aurora" as one rds-cluster resource
    And the plan lists "billing-db" as one rds-instance resource
    And neither "orders-aurora-1" nor "orders-aurora-2" appears in the plan as an action or a refusal

  @id:discovery.data-discovery.002 @rule:data.discovery.database-price-or-unpriced @inferred @source:internal/awsx/discover_data.go#databases @questionable
  Scenario Outline: A standalone database gets an hourly estimate from its instance class; anything else is reported as unpriced
    Given a database <resource> of kind <kind> with instance class "<class>" that is available
    And the policy has include_databases set
    When the operator runs a plan
    Then the database is counted under <savings_outcome> in the plan's blast radius

    Examples:
      | resource      | kind         | class        | savings_outcome                     |
      | billing-db    | rds-instance | db.m5.large  | the hourly savings for rds-instance |
      | legacy-db     | rds-instance | db.zz9.large | the unpriced count for rds-instance |
      | orders-aurora | rds-cluster  | db.r6g.large | the unpriced count for rds-cluster  |

  @id:discovery.data-discovery.003 @rule:data.discovery.status-decides-stoppability @inferred @source:internal/awsx/discover_data.go#databases
  Scenario Outline: A database that is not available when discovered is refused rather than stopped
    Given a database <resource> of kind <kind> whose status is "<status>"
    And the policy has include_databases set
    When the operator runs a plan
    Then the plan refuses <resource> with reason "not available (<status>)"
    And the plan proposes no stop for <resource>

    Examples:
      | resource      | kind         | status     |
      | billing-db    | rds-instance | stopped    |
      | orders-aurora | rds-cluster  | stopping   |
      | reports-db    | rds-instance | backing-up |

  @id:discovery.data-discovery.004 @rule:data.discovery.failure-reported-per-region @inferred @source:internal/awsx/discover_data.go#databases @questionable
  Scenario: When the database listing fails in a region, the plan reports it and contains no databases from that region
    Given the database listing in "eu-west-1" fails partway, for example because the cluster listing is denied
    And database instance "billing-db" was already listed before the failure
    When the operator runs a plan
    Then the operator sees a discovery error starting "rds in eu-west-1:"
    And the plan contains no database from "eu-west-1", including "billing-db"
    And resources from other services in "eu-west-1" still appear in the plan
