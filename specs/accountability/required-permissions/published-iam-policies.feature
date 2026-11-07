@domain:accountability @area:required-permissions @from:docs/iam-calls-test
Feature: Publishing the IAM policies the tool needs
  Operators attach the published IAM policies to the identity that runs aws-killswitch, and need them to allow every AWS call each command makes, so a fire or a restore never stops halfway on AccessDenied.

  Background:
    Given the operator has the published policies iam-plan-readonly.json and iam-fire.json

  @id:accountability.required-permissions.001 @rule:accountability.required-permissions.published-policies-grant-every-call @verified @source:docs/iam-plan-readonly.json#Discover @test:docs/iam_calls_test.go#TestPoliciesGrantEveryCallTheToolMakes
  Scenario Outline: The published policies allow every AWS call each command makes
    Given an identity with only <attached policies> attached
    And an account holding every supported resource kind
    When the operator runs <commands>
    Then no AWS call made by <commands> is refused for a missing permission
    And the commands get as far as their permissions allow without stopping on AccessDenied

    Examples:
      | attached policies                        | commands               |
      | iam-plan-readonly.json                   | plan, status and spend |
      | iam-plan-readonly.json and iam-fire.json | fire and restore       |
