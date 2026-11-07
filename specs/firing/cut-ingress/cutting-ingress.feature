@domain:firing @area:cut-ingress @from:awsx/execute-ingress
Feature: Cutting and restoring public entry points
  Lets an operator stop traffic reaching load balancers, CloudFront distributions and API Gateway stages during a fire, then put each one back exactly as it was found.

  Background:
    Given a fire plan includes the account's public entry points

  @id:firing.cut-ingress.001 @rule:firing.ingress.listener-answers-503 @inferred @source:internal/awsx/execute_ingress.go#blockListener
  Scenario: A fired load balancer listener answers every request with a 503 cost-control message
    Given a load balancer listener routes traffic to a target group
    When the operator fires the plan
    Then the listener's default action answers requests with HTTP 503
    And the response body is "Service temporarily unavailable (cost control)" as text/plain
    And the target group and the targets behind it are left unchanged

  @id:firing.cut-ingress.002 @rule:recovery.ingress.listener-routing-restored-verbatim @inferred @source:internal/awsx/execute_ingress.go#restoreListener
  Scenario: Restoring a listener puts back the routing it had before the fire
    Given a listener was fired and its routing before the fire was recorded
    And that recorded routing includes settings this tool does not interpret
    When the operator restores the fire
    Then the listener routes traffic using its recorded default actions again
    And settings the tool does not interpret come back unchanged

  @id:firing.cut-ingress.003 @rule:recovery.ingress.restore-refuses-to-guess @inferred @source:internal/awsx/execute_ingress.go#restoreListener
  Scenario Outline: Restore refuses to guess an entry point's prior state when the record does not hold it
    Given a fired <entry point> whose restore record <record problem>
    When the operator restores the fire
    Then that entry point is not changed
    And the restore of that entry point fails with "<error>"

    Examples:
      | entry point             | record problem                          | error                                                                            |
      | load balancer listener  | has no default actions                  | no recorded default actions; restoring would guess at the routing                |
      | load balancer listener  | has an empty list of default actions    | recorded default actions are empty; refusing to leave the listener with no route |
      | load balancer listener  | has default actions that cannot be read | recorded listener actions are unreadable                                         |
      | CloudFront distribution | has no enabled state                    | no recorded enabled state; refusing to guess                                     |
      | API Gateway stage       | has no rate or burst limit              | no usable recorded throttle; refusing to guess                                   |
      | API Gateway stage       | names no API or stage                   | recorded state names no API or stage                                             |

  @id:firing.cut-ingress.004 @rule:firing.ingress.distribution-disabled-config-preserved @inferred @source:internal/awsx/execute_ingress.go#setDistributionEnabled
  Scenario: A CloudFront distribution is disabled on fire and re-enabled on restore with its other settings kept
    Given an enabled CloudFront distribution
    And its configuration is changed between the fire and the restore
    When the operator restores the fire
    Then the distribution is enabled again
    And configuration changes made since the fire are kept

  @id:firing.cut-ingress.005 @rule:recovery.ingress.already-disabled-distribution-stays-disabled @inferred @source:internal/awsx/execute_ingress.go#restoreDistribution
  Scenario: A distribution that was already disabled before the fire is not switched on by restore
    Given a CloudFront distribution whose restore record says it was disabled before the fire
    When the operator restores the fire
    Then the distribution stays disabled
    And the restore of that distribution reports no error

  @id:firing.cut-ingress.006 @rule:firing.ingress.stage-throttled-to-zero @inferred @source:internal/awsx/execute_ingress.go#throttleStage
  Scenario: A fired API Gateway stage is throttled to zero requests and zero burst
    Given an API Gateway stage that accepts requests
    When the operator fires the plan
    Then the stage's default method rate limit is 0
    And the stage's default method burst limit is 0

  @id:firing.cut-ingress.007 @rule:recovery.ingress.stage-throttle-restored @inferred @source:internal/awsx/execute_ingress.go#restoreStage @questionable
  Scenario Outline: Restoring a stage puts back its prior throttle, or removes the override if it had none
    Given a fired API Gateway stage whose recorded throttle was rate <rate> and burst <burst>
    When the operator restores the fire
    Then the stage <outcome>

    Examples:
      | rate | burst | outcome                                                   |
      | 100  | 200   | has a default rate limit of 100 and burst limit of 200    |
      | -1   | -1    | stays throttled to zero, and its restore fails with "no usable recorded throttle; refusing to guess" |
      | 50   | -1    | has no stage-level throttle, so the account limit applies |
