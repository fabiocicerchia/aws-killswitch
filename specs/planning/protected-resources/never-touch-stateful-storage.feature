@domain:planning @area:protected-resources @from:model/model
Feature: Never touching stateful storage
  Guarantees operators that whatever scope or flags they pass, the plan never schedules a change against storage whose loss is unrecoverable, and says so for each such resource.

  Background:
    Given the operator's policy puts every resource in the account in scope

  @id:planning.protected-resources.001 @rule:planning.protected-resources.stateful-kinds-never-touched @verified @source:internal/model/model.go#IsNeverTouch @test:internal/plan/plan_test.go#TestStatefulStorageIsNeverTouched
  Scenario Outline: A protected storage kind is never scheduled, whatever the scope
    Given the account holds a resource of kind "<kind>"
    When the operator builds a plan
    Then the plan contains no action for that resource
    And the resource is listed under NOT TOUCHED with the reason "protected kind: this tool never touches stateful storage"

    Examples:
      | kind           |
      | s3             |
      | dynamodb       |
      | ebs-volume     |
      | snapshot       |
      | kms            |
      | iam            |
      | secretsmanager |
      | route53-zone   |

  @id:planning.protected-resources.003 @rule:planning.protected-resources.compute-is-not-protected @verified @source:internal/model/model.go#IsNeverTouch @test:internal/plan/plan_test.go#TestStatefulStorageIsNeverTouched
  Scenario Outline: Compute kinds are not part of the protected set
    Given the account holds a running resource of kind "<kind>"
    When the operator builds a plan
    Then the resource is not refused as a protected kind

    Examples:
      | kind        |
      | ecs-service |
      | asg         |
