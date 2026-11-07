@domain:discovery @area:ingress-discovery @from:awsx/discover-ingress
Feature: Finding the public entry points that let traffic in
  Lets the operator see every load balancer listener, CloudFront distribution and REST API stage that is currently letting traffic into the account, each with enough of its prior state recorded to put it back exactly.

  Background:
    Given the operator has AWS credentials for account 123456789012
    And the policy scopes the run to region eu-west-1

  @id:discovery.ingress-discovery.001 @rule:discovery.ingress.only-forwarding-listeners-are-candidates @inferred @source:internal/awsx/discover_ingress.go#forwards
  Scenario Outline: Only load balancer listeners that forward traffic are offered for cutting
    Given load balancer "web-alb" has a listener on port <port> whose default action is <default action>
    When the operator runs a plan
    Then the listener "web-alb:<port>" is <outcome> in the plan

    Examples:
      | port | default action            | outcome                     |
      | 443  | forward to a target group | listed as an ingress action |
      | 80   | redirect to HTTPS         | absent                      |
      | 8080 | fixed 503 response        | absent                      |

  @id:discovery.ingress-discovery.002 @rule:discovery.ingress.api-stage-throttle-restored-as-found @inferred @source:internal/awsx/discover_ingress.go#apiStage
  Scenario Outline: A throttled API stage is restored to the stage limit it had, or to the account limit if it had none
    Given REST API "orders-api" stage "prod" has <stage default throttle>
    And the operator has fired a plan that throttled "orders-api prod" to 0 requests per second
    When the operator restores the fire
    Then "orders-api prod" <restored throttle>

    Examples:
      | stage default throttle                                     | restored throttle                                                      |
      | no stage-wide default throttle                             | has no stage-wide default throttle again, so the account limit applies |
      | a stage-wide default of 100 requests per second, burst 200 | has a stage-wide default of 100 requests per second, burst 200 again   |

  @id:discovery.ingress-discovery.003 @rule:discovery.ingress.entry-points-named-as-known @inferred @source:internal/awsx/discover_ingress.go#apiStage
  Scenario Outline: Entry points are named in the plan the way the operator knows them
    Given the account has <entry point>
    When the operator runs a plan
    Then the plan names it "<shown as>"

    Examples:
      | entry point                                                           | shown as                      |
      | a forwarding listener on port 443 of load balancer "web-alb"          | web-alb:443                   |
      | distribution "E2QWRUHEXAMPLE" served at d111111abcdef8.cloudfront.net | d111111abcdef8.cloudfront.net |
      | stage "prod" of REST API "orders-api"                                 | orders-api prod               |
      | stage "prod" of an unnamed REST API with id "a1b2c3d4e5"              | a1b2c3d4e5 prod               |
