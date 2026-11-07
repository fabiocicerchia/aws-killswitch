@domain:planning @area:plan-building @from:plan/plan
Feature: Building a shutdown plan
  Gives the operator a reviewed, stable, ordered list of changes so the plan they approve is the plan that fires and restore brings the account back without traffic hitting nothing.

  Background:
    Given an inventory discovered in account 123456789012, region eu-west-1
    And a policy scoped to resources tagged Env=prod

  @id:planning.plan-building.001 @rule:plan.ordering.ingress-cut-before-compute-drained @verified @source:internal/plan/plan.go#sortPlan @test:internal/plan/plan_test.go#TestIngressIsCutBeforeComputeIsDrained @test:internal/plan/plan_test.go#TestASGIsScaledBeforeLooseInstancesAreStopped @test:internal/plan/plan_test.go#TestNodegroupIsScaledBeforeLooseInstancesAndAfterLambda @test:internal/plan/plan_test.go#TestCloudFrontIsCutFirstOfAll
  Scenario Outline: Ingress is cut before compute is drained, and scaling groups before loose instances
    Given in-scope resources of kinds <kinds> are running
    When the operator builds a plan
    Then the plan's actions run in the order <order>
    And every ingress action comes before every compute action

    Examples:
      | kinds                                      | order                                      |
      | asg, alb-listener, lambda                  | alb-listener, lambda, asg                  |
      | ec2-instance, asg                          | asg, ec2-instance                          |
      | ec2-instance, eks-nodegroup, lambda        | lambda, eks-nodegroup, ec2-instance        |
      | alb-listener, apigateway-stage, cloudfront | cloudfront, apigateway-stage, alb-listener |

  @id:planning.plan-building.002 @rule:plan.ordering.same-inventory-same-plan @verified @source:internal/plan/plan.go#sortPlan @test:internal/plan/plan_test.go#TestPlanIsDeterministic
  Scenario: The same inventory always yields the same plan
    Given the same in-scope resources are discovered twice in a different order
    When the operator builds a plan from each discovery
    Then both plans list the same actions in the same positions
    And refusals are listed in resource name order

  @id:planning.plan-building.003 @rule:plan.acknowledgement.warnings-and-size-must-be-confirmed @verified @source:internal/plan/plan.go#acknowledgements @test:internal/plan/plan_test.go#TestDatabasesAreOptInAndCarryTheSevenDayWarning @test:internal/plan/plan_test.go#TestLargePlansRequireAcknowledgement
  Scenario Outline: Risky actions and oversized plans require the operator's acknowledgement
    Given the plan contains <situation>
    When the operator builds a plan
    Then the plan's acknowledgement list contains "<acknowledgement>"

    Examples:
      | situation                                                  | acknowledgement                                                                            |
      | a database stop under include_databases                    | AWS restarts a stopped database by itself after 7 days                                     |
      | a NAT gateway deletion under delete_nat_gateways           | deletion is not a stop — restore recreates it with a new address and repoints route tables |
      | an instance-store instance under allow_instance_store_loss | instance-store data will be lost permanently                                               |
      | a CloudFront distribution disable                          | CloudFront takes minutes to propagate a disable to every edge                              |
      | an API Gateway stage throttle                              | callers get 429, not a connection error                                                    |
      | 30 actions with confirm_above 25                           | this plan changes 30 resources, over the confirm_above threshold of 25                     |

  @id:planning.plan-building.004 @rule:plan.restore.compute-before-ingress @verified @source:internal/plan/plan.go#RestoreOrder @test:internal/plan/plan_test.go#TestRestoreBringsComputeBackBeforeIngress @test:internal/plan/plan_test.go#TestNewKindsRestoreInReverseOfTheOrderTheyWereCut @questionable
  Scenario: Restore brings data and compute back before letting traffic in
    Given a fire stopped a database, an auto-scaling group and an ALB listener
    When the operator restores the account
    Then the database is restored first
    And the auto-scaling group is restored before the ALB listener
    And the ALB listener is restored last
