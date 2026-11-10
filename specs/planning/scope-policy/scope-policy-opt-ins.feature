@domain:planning @area:scope-policy @from:plan/plan-test
Feature: Writing down what an account allows before an incident
  Lets operators state in the policy which destructive or lossy stops are allowed, and rejects a policy that would hit more than they meant.

  @id:planning.scope-policy.001 @rule:planning.scope.databases-opt-in @verified @source:internal/plan/plan.go#databaseAction @test:internal/plan/plan_test.go#TestDatabasesAreOptInAndCarryTheSevenDayWarning
  Scenario Outline: Databases are only stopped when the policy includes them
    Given a scope policy selecting resources tagged Env=prod with include_databases <setting>
    And an available RDS instance "db-1" tagged Env=prod
    When the operator builds a plan
    Then "db-1" is <outcome>

    Examples:
      | setting | outcome                                                                                             |
      | unset   | refused with "database: excluded unless include_databases is set"                                   |
      | set     | scheduled last, in the data phase, warning "AWS restarts a stopped database by itself after 7 days" |

  @id:planning.scope-policy.002 @rule:planning.scope.instance-store-loss-opt-in @verified @source:internal/plan/plan.go#instanceAction @test:internal/plan/plan_test.go#TestInstanceStoreLossIsRefusedByDefault
  Scenario Outline: An instance whose local disks a stop would erase is refused unless the policy accepts the loss
    Given a scope policy selecting resources tagged Env=prod with allow_instance_store_loss <setting>
    And a running EC2 instance "i-1" tagged Env=prod with instance-store volumes
    When the operator builds a plan
    Then "i-1" is <outcome>

    Examples:
      | setting | outcome                                                                                                      |
      | unset   | refused with "has instance-store volumes, which a stop erases; set allow_instance_store_loss to accept that" |
      | set     | scheduled with the warning "instance-store data will be lost permanently"                                    |

  @id:planning.scope-policy.003 @rule:planning.scope.nat-deletion-opt-in @verified @source:internal/plan/plan.go#actionFor @test:internal/plan/plan_test.go#TestNATGatewayDeletionIsOptInAndFlagged
  Scenario Outline: NAT gateways are only deleted when the policy opts in, and the plan says it is a deletion
    Given a scope policy selecting resources tagged Env=prod with delete_nat_gateways <setting>
    And a NAT gateway "nat-1" tagged Env=prod
    When the operator builds a plan
    Then "nat-1" is <outcome>

    Examples:
      | setting | outcome                                                                                                                              |
      | unset   | refused with "NAT gateways can only be deleted, not stopped; set delete_nat_gateways to include them"                                |
      | set     | scheduled in the network phase, warning "deletion is not a stop — restore recreates it with a new address and repoints route tables" |
