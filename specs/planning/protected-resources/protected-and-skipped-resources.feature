@domain:planning @area:protected-resources @from:plan/plan
Feature: Protecting resources from the killswitch
  Guarantees the operator that unrecoverable data and opt-in-only resources are never touched by default, and that every skipped resource appears in the plan with its reason.

  Background:
    Given an inventory discovered in account 123456789012, region eu-west-1

  @id:planning.protected-resources.004 @rule:plan.protection.stateful-storage-never-touched @verified @source:internal/plan/plan.go#Build @test:internal/plan/plan_test.go#TestStatefulStorageIsNeverTouched @questionable
  Scenario Outline: Stateful storage is never touched, whatever the scope or flags
    Given the policy scope is everything
    And the inventory contains <resource>
    When the operator builds a plan
    Then the plan has no action for <resource>
    And it is listed as skipped with "protected kind: this tool never touches stateful storage"

    Examples:
      | resource                                                          |
      | arn:aws:s3:::example-bucket                                       |
      | arn:aws:dynamodb:eu-west-1:123456789012:table/orders              |
      | arn:aws:elasticfilesystem:eu-west-1:123456789012:file-system/fs-1 |
      | arn:aws:backup:eu-west-1:123456789012:vault/default               |

  @id:planning.protected-resources.005 @rule:plan.protection.out-of-scope-explained @verified @source:internal/plan/plan.go#Build @test:internal/plan/plan_test.go#TestProtectTagWinsOverEveryScope @test:internal/plan/plan_test.go#TestProtectTagOnlyIgnoredWhenExplicitlyFalse @test:internal/plan/plan_test.go#TestOutOfScopeResourcesAreExplained
  Scenario Outline: Protected-tagged and out-of-scope resources are skipped with the reason
    Given the policy is scoped to Env=prod
    And an in-scope-kind resource tagged <tags>
    When the operator builds a plan
    Then the plan has no action for it
    And it is listed as skipped with "<reason>"

    Examples:
      | tags                                 | reason                        |
      | Env=prod, killswitch:protect=true    | tagged killswitch:protect     |
      | Env=prod, killswitch:protect=(empty) | tagged killswitch:protect     |
      | Env=staging                          | Env=staging, scope wants prod |
      | (none)                               | no Env tag                    |

  @id:planning.protected-resources.006 @rule:plan.protection.risky-kinds-are-opt-in @verified @source:internal/plan/plan.go#actionFor @test:internal/plan/plan_test.go#TestDatabasesAreOptInAndCarryTheSevenDayWarning @test:internal/plan/plan_test.go#TestInstanceStoreLossIsRefusedByDefault @test:internal/plan/plan_test.go#TestNATGatewayDeletionIsOptInAndFlagged
  Scenario Outline: Databases, NAT gateway deletion and instance-store loss are skipped unless the policy opts in
    Given the policy is scoped to Env=prod without <setting>
    And an in-scope running <kind>
    When the operator builds a plan
    Then the plan has no action for it
    And it is listed as skipped with a reason naming <setting>

    Examples:
      | kind                                     | setting                   |
      | RDS instance                             | include_databases         |
      | RDS cluster                              | include_databases         |
      | NAT gateway                              | delete_nat_gateways       |
      | EC2 instance with instance-store volumes | allow_instance_store_loss |

  @id:planning.protected-resources.007 @rule:plan.protection.already-at-rest-skipped @verified @source:internal/plan/plan.go#actionFor @test:internal/plan/plan_test.go#TestAlreadyStoppedResourcesAreSkipped @test:internal/plan/plan_test.go#TestNodegroupAlreadyAtZeroIsRefusedNotReapplied @test:internal/plan/plan_test.go#TestAnAlreadyDisabledDistributionIsRefused @test:internal/plan/plan_test.go#TestAStageAlreadyThrottledToZeroIsRefused
  Scenario Outline: Resources already at rest are skipped rather than stopped again
    Given an in-scope <kind> whose recorded state is <state>
    When the operator builds a plan
    Then the plan has no action for it
    And it is listed as skipped with "<reason>"

    Examples:
      | kind                                 | state              | reason                    |
      | auto-scaling group                   | desired capacity 0 | already at zero           |
      | ECS service                          | desired count 0    | already at zero           |
      | EKS nodegroup                        | desired size 0     | already at zero           |
      | EC2 instance                         | stopped            | not running (stopped)     |
      | CloudFront distribution              | disabled           | already disabled          |
      | API Gateway stage                    | rate limit 0       | already throttled to zero |
      | RDS instance (include_databases set) | stopped            | not available (stopped)   |
