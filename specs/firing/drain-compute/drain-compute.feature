@domain:firing @area:drain-compute @from:awsx/execute-compute
Feature: Draining and restoring compute workloads
  Lets an operator stop compute that burns money while running, then bring each workload back to exactly the capacity it had before.

  Background:
    Given the operator has a reviewed plan that includes compute resources in the account

  @id:firing.drain-compute.002 @rule:recovery.restore-run.compute-returns-to-recorded-capacity @inferred @source:internal/awsx/execute_compute.go#restoreLambda
  Scenario Outline: Restoring puts each compute workload back to its recorded capacity
    Given "<name>" was drained by a fire that recorded <recorded>
    When the operator restores the fire
    Then "<name>" ends up <restored_state>

    Examples:
      | name                             | recorded                     | restored_state                                                           |
      | Lambda function orders-worker    | a reserved concurrency of 25 | with reserved concurrency 25                                             |
      | Lambda function thumbnailer      | no reserved concurrency      | with no reserved concurrency at all, using the account's unreserved pool |
      | ECS service api                  | a desired count of 3         | with desired count 3                                                     |
      | auto-scaling group web-asg       | min 1, desired 3, max 6      | with min 1, desired 3 and max 6                                          |
      | EC2 instance i-0abc1234def567890 | that it was running          | started again                                                            |
      | EKS node group ng-default        | min 3 and desired 6          | with min 3 and desired 6, max size as it was                             |

  @id:firing.drain-compute.003 @rule:recovery.restore-run.compute-refuses-incomplete-record @inferred @source:internal/awsx/execute_compute.go#restoreASG
  Scenario Outline: Restore refuses to guess a compute capacity the record does not hold
    Given "<name>" was drained by a fire whose record <defect>
    When the operator restores the fire
    Then "<name>" is left drained
    And the restore reports "<message>" for "<name>"

    Examples:
      | name                          | defect                                              | message                                                          |
      | auto-scaling group web-asg    | has min and desired but no max size                 | incomplete or unusable recorded capacity; refusing to guess      |
      | ECS service api               | has a negative desired count                        | no usable recorded desired count; refusing to guess              |
      | ECS service api               | names no cluster                                    | no recorded cluster for this service                             |
      | EKS node group ng-default     | has no desired size                                 | incomplete or unusable recorded node counts; refusing to guess   |
      | EKS node group ng-default     | names no cluster                                    | recorded state names no cluster or nodegroup                     |
      | Lambda function orders-worker | says it had a reservation but holds no usable value | recorded a reservation but not a usable value; refusing to guess |
