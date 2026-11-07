@domain:planning @area:protected-resources @from:policy/policy
Feature: Protecting resources with the protect tag
  Lets a team mark a resource as untouchable ahead of time, so no scope, flag or operator under pressure can include it in a fire.

  Background:
    Given the operator runs aws-killswitch with a valid policy file

  @id:planning.protected-resources.011 @rule:planning.protected.only-explicit-false-unprotects @verified @source:internal/policy/policy.go#isProtected @test:internal/plan/plan_test.go#TestProtectTagOnlyIgnoredWhenExplicitlyFalse
  Scenario Outline: The protect tag protects unless its value explicitly says no
    Given the policy scope.tags is {"Env": "prod"}
    And the account has a function tagged Env=prod and killswitch:protect="<value>"
    When the operator plans a fire
    Then the function is <outcome>

    Examples:
      | value           | outcome                 |
      | true            | skipped as protected    |
      |                 | skipped as protected    |
      | because reasons | skipped as protected    |
      | 1               | skipped as protected    |
      | false           | scheduled to be stopped |
      | FALSE           | scheduled to be stopped |
      | 0               | scheduled to be stopped |
      | no              | scheduled to be stopped |
      | off             | scheduled to be stopped |
