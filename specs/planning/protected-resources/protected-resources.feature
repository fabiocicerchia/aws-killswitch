@domain:planning @area:protected-resources @from:plan/plan-test
Feature: Deciding what the killswitch leaves alone
  Gives operators a guarantee that unrecoverable storage and deliberately protected resources are never stopped, and that the plan explains every resource it leaves alone.

  Background:
    Given a scope policy selecting resources tagged Env=prod

  @id:planning.protected-resources.008 @rule:planning.protected.stateful-storage-never-touched @verified @source:internal/plan/plan.go#Build @test:internal/plan/plan_test.go#TestStatefulStorageIsNeverTouched
  Scenario Outline: Stateful storage is never scheduled, whatever the scope
    Given the scope policy is changed to cover everything in the account
    And the inventory contains the resource "<arn>"
    When the operator builds a plan
    Then the plan contains no action for "<arn>"
    And the plan lists "<arn>" as refused with "protected kind: this tool never touches stateful storage"

    Examples:
      | arn                                                               |
      | arn:aws:s3:::example-bucket                                       |
      | arn:aws:dynamodb:eu-west-1:123456789012:table/orders              |
      | arn:aws:fsx:eu-west-1:123456789012:file-system/fs-1               |
      | arn:aws:backup:eu-west-1:123456789012:vault/default               |
      | arn:aws:kms:eu-west-1:123456789012:key/example                    |

  @id:planning.protected-resources.009 @rule:planning.protected.protect-tag-unprotects-only-when-explicitly-false @verified @source:internal/policy/policy.go#isProtected @test:internal/plan/plan_test.go#TestProtectTagOnlyIgnoredWhenExplicitlyFalse
  Scenario Outline: Only an explicitly false protect tag lets a resource be stopped
    Given a Lambda function "fn-1" tagged Env=prod and killswitch:protect="<value>"
    When the operator builds a plan
    Then "fn-1" is <outcome>

    Examples:
      | value           | outcome                 |
      | true            | refused as protected    |
      | yes             | refused as protected    |
      |                 | refused as protected    |
      | 1               | refused as protected    |
      | because reasons | refused as protected    |
      | false           | scheduled to be stopped |
      | FALSE           | scheduled to be stopped |
      | 0               | scheduled to be stopped |
      | no              | scheduled to be stopped |
      | off             | scheduled to be stopped |
