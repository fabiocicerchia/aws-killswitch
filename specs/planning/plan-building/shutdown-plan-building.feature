@domain:planning @area:plan-building @from:plan/plan
Feature: Building a shutdown plan
  Gives the operator a reviewed, stable, ordered list of changes so the plan they approve is the plan that fires and restore brings the account back without traffic hitting nothing.

  Background:
    Given an inventory discovered in account 123456789012, region eu-west-1
    And a policy scoped to resources tagged Env=prod

  @id:planning.plan-building.002 @rule:plan.ordering.same-inventory-same-plan @verified @source:internal/plan/plan.go#sortPlan @test:internal/plan/plan_test.go#TestPlanIsDeterministic
  Scenario: The same inventory always yields the same plan
    Given the same in-scope resources are discovered twice in a different order
    When the operator builds a plan from each discovery
    Then both plans list the same actions in the same positions
    And refusals are listed in resource name order
