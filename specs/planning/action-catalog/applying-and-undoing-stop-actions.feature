@domain:planning @area:action-catalog @from:awsx/execute
Feature: Applying and undoing each stop action
  Operators need to know exactly what firing does to each kind of resource and that restore puts back what was recorded, so they dare press the killswitch.

  Background:
    Given the operator has reviewed a plan for the account

  @id:planning.action-catalog.001 @rule:planning.action-catalog.stop-meaning-per-kind @inferred @source:internal/awsx/execute.go#appliers
  Scenario Outline: Firing stops each kind of resource in its own way, deleting only NAT gateways
    Given the plan includes a <kind> named <resource>
    When the operator fires the plan
    Then the <kind> is <stopped_state>
    And no resource other than a NAT gateway is deleted

    Examples:
      | kind                    | resource              | stopped_state                                                                     |
      | ALB listener            | app-lb/443            | answering every request with 503 "Service temporarily unavailable (cost control)" |
      | Lambda function         | orders-worker         | throttled to a reserved concurrency of 0                                          |
      | ECS service             | web/api               | scaled to a desired count of 0                                                    |
      | auto-scaling group      | batch-asg             | set to minimum 0, maximum 0, desired 0                                            |
      | EC2 instance            | i-0abc1234def567890   | stopped                                                                           |
      | EKS nodegroup           | prod/general          | scaled to a minimum and desired size of 0, maximum left unchanged                 |
      | CloudFront distribution | E2EXAMPLE123          | disabled                                                                          |
      | API Gateway stage       | shop-api/prod         | throttled to 0 requests per second with 0 burst                                   |
      | NAT gateway             | nat-0abc1234def567890 | deleted                                                                           |
      | RDS instance            | orders-db             | stopped                                                                           |
      | Aurora cluster          | reports-cluster       | stopped                                                                           |

  @id:planning.action-catalog.002 @rule:firing.pause-data-network.final-snapshot-when-opted-in @inferred @source:internal/awsx/execute.go#Executor @questionable
  Scenario: A database stopped under a final-snapshot policy is stopped without a snapshot
    Given the policy sets include_databases and final_snapshot
    And the plan lists RDS instance orders-db as "snapshot, then stop the database"
    When the operator fires the plan
    Then RDS instance orders-db is stopped
    And no snapshot named killswitch-orders-db-<timestamp> is taken
