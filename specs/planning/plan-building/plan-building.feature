@domain:planning @area:plan-building @from:plan/plan-test
Feature: Building a shutdown plan that is safe to run and to reverse
  Gives operators a reviewed plan that runs in a safe order, is the same every time it is built, and makes them confirm the risky parts.

  Background:
    Given Given a scope policy selecting resources tagged Env=prod

  @id:planning.plan-building.005 @rule:planning.order.ingress-before-compute-before-network-before-data @verified @source:internal/plan/plan.go#sortPlan @test:internal/plan/plan_test.go#TestIngressIsCutBeforeComputeIsDrained @test:internal/plan/plan_test.go#TestASGIsScaledBeforeLooseInstancesAreStopped @test:internal/plan/plan_test.go#TestNodegroupIsScaledBeforeLooseInstancesAndAfterLambda @test:internal/plan/plan_test.go#TestCloudFrontIsCutFirstOfAll
  Scenario: Traffic is cut before compute is drained, and managed groups go before loose instances
    Given the policy opts into databases and NAT gateway deletion
    And the inventory holds one in-scope resource of each supported kind
    When the operator builds a plan
    Then the actions run in this order: CloudFront distribution, API Gateway stage, ALB listener, Lambda function, ECS service, EKS nodegroup, auto-scaling group, EC2 instance, NAT gateway, RDS cluster, RDS instance

  @id:planning.plan-building.006 @rule:planning.order.deterministic @verified @source:internal/plan/plan.go#sortPlan @test:internal/plan/plan_test.go#TestPlanIsDeterministic
  Scenario: The same inventory always produces the same plan, whatever order it was discovered in
    Given an inventory of Lambda functions "fn-a" and "fn-b", auto-scaling group "asg-z" and ALB listener "l-1", all tagged Env=prod
    When the operator builds a plan twice from that inventory, discovered in different orders
    Then both plans list the same actions in the same order

  @id:planning.plan-building.007 @rule:planning.restore.reverse-phase-order @verified @source:internal/plan/plan.go#RestoreOrder @test:internal/plan/plan_test.go#TestRestoreBringsComputeBackBeforeIngress @test:internal/plan/plan_test.go#TestNewKindsRestoreInReverseOfTheOrderTheyWereCut @questionable
  Scenario: Restore brings data and compute back before letting traffic in
    Given a fire stopped ALB listener "listener-1", auto-scaling group "asg-1" and RDS instance "db-1"
    When the operator restores the fire
    Then "db-1" is restored first
    And "asg-1" is restored next
    And "listener-1" is restored last

  @id:planning.plan-building.008 @rule:planning.ack.warnings-require-acknowledgement @verified @source:internal/plan/plan.go#acknowledgements @test:internal/plan/plan_test.go#TestDatabasesAreOptInAndCarryTheSevenDayWarning
  Scenario Outline: Every warning in the plan is something the operator must acknowledge
    Given the plan schedules <resource>
    When the operator builds a plan
    Then the acknowledgements include "<warning>"

    Examples:
      | resource                          | warning                                                                                                  |
      | CloudFront distribution "E123"    | CloudFront takes minutes to propagate a disable to every edge, and returns errors to users while it does |
      | API Gateway stage "abc123/prod"   | callers get 429, not a connection error — a client with retries will keep trying                         |
      | RDS instance "db-1"               | AWS restarts a stopped database by itself after 7 days                                                   |
      | NAT gateway "nat-1"               | deletion is not a stop — restore recreates it with a new address and repoints route tables               |
      | instance-store EC2 instance "i-1" | instance-store data will be lost permanently                                                             |

  @id:planning.plan-building.009 @rule:planning.ack.large-plan-threshold @verified @source:internal/plan/plan.go#acknowledgements @test:internal/plan/plan_test.go#TestLargePlansRequireAcknowledgement
  Scenario Outline: A plan changing more resources than the confirm threshold must be acknowledged
    Given the policy sets confirm_above to <threshold>
    And the plan schedules <count> Lambda functions
    When the operator builds a plan
    Then the acknowledgements include "this plan changes <count> resources, over the confirm_above threshold of <limit>"

    Examples:
      | threshold | count | limit |
      | 25        | 30    | 25    |
      | unset     | 26    | 25    |
