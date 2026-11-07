@domain:assurance @area:scratch-account @from:examples/scratch-account
Feature: Provisioning a scratch account for verification
  Gives operators a disposable account with one resource of every kind the killswitch can stop, so a verify run checks every fire and restore path against real AWS and only touches resources the module created.

  Background:
    Given the operator has applied the scratch-account module to a throwaway AWS account

  @id:assurance.scratch-account.001 @rule:assurance.scratch-account.every-supported-kind-present @inferred @source:examples/scratch-account/main.tf#aws_lb_listener
  Scenario Outline: The scratch account holds a resource of every kind the killswitch can stop
    Given verify treats <kind> as a supported kind
    When the operator applies the scratch-account module
    Then the account holds at least one <resource> of kind <kind>
    And a verify run against the account does not list <kind> as NOT EXERCISED

    Examples:
      | kind                    | resource                                               |
      | alb-listener            | HTTP listener on an internal application load balancer |
      | lambda                  | function with reserved concurrency                     |
      | ecs-service             | Fargate service                                        |
      | asg                     | auto-scaling group                                     |
      | ec2-instance            | standalone instance outside any auto-scaling group     |
      | nat-gateway             | NAT gateway with its own elastic IP                    |
      | rds-instance            | PostgreSQL database instance                           |
      | rds-cluster             | Aurora PostgreSQL cluster                              |
      | eks-nodegroup           | EKS managed nodegroup                                  |
      | cloudfront-distribution | CloudFront distribution                                |
      | apigateway-stage        | REST API stage                                         |

  @id:assurance.scratch-account.002 @rule:assurance.scratch-account.everything-carries-scope-tag @inferred @source:examples/scratch-account/main.tf#locals @questionable
  Scenario: Every resource the scratch account creates carries the killswitch scope tag and the name prefix
    Given the scope tag is "killswitch = scratch" and the name prefix is "ksverify"
    When the operator applies the scratch-account module
    Then every tagged resource it creates carries the tag "killswitch = scratch"
    And every tagged resource carries Name "ksverify" and ManagedBy "aws-killswitch/examples/scratch-account"
    And instances launched by the auto-scaling group inherit the same tags

  @id:assurance.scratch-account.003 @rule:assurance.scratch-account.non-default-prior-settings @inferred @source:examples/scratch-account/main.tf#aws_lambda_function
  Scenario Outline: Scratch resources start from non-default settings, so a restore has to put back a real value
    Given the scratch account has been seeded
    When the operator reads the <resource> before any fire
    Then its <setting> is <value>, not zero and not unset

    Examples:
      | resource           | setting                          | value      |
      | Lambda function    | reserved concurrency             | 5          |
      | API Gateway stage  | throttling rate and burst limits | 100 and 50 |
      | ECS service        | desired task count               | 2          |
      | auto-scaling group | min, desired and max capacity    | 1, 2 and 3 |
      | EKS nodegroup      | min, desired and max size        | 1, 2 and 3 |

  @id:assurance.scratch-account.004 @rule:assurance.scratch-account.seed-output-next-steps @inferred @source:examples/scratch-account/main.tf#next
  Scenario: Seeding the scratch account tells the operator to plan, verify and then destroy
    When the operator applies the scratch-account module
    Then the output lists "aws-killswitch plan --config killswitch.json"
    And the output lists "aws-killswitch verify --config killswitch.json --yes"
    And the output tells the operator to run "terraform destroy" before the bill notices
