@domain:accountability @area:required-permissions @from:docs/iam-test
Feature: Least-privilege IAM policies for planning, firing and the Lambda trigger
  Gives operators ready-made IAM policies whose limits they can rely on, so read-only users cannot change anything and the identity that fires the killswitch can never destroy data.

  Background:
    Given the operator attaches the IAM policies shipped with aws-killswitch

  @id:accountability.required-permissions.002 @rule:permissions.iam.readonly-policy-cannot-change-resources @verified @source:docs/iam-plan-readonly.json#Discover @test:docs/iam_test.go#TestReadonlyPolicyCannotChangeAnything
  Scenario: The read-only policy grants only read access
    Given an identity has only the read-only plan policy attached
    When the operator reviews what the read-only policy allows
    Then every allowed action is a Describe, List, Get or GET call
    And the policy covers discovery, spend lookup and reading saved restore records
    And no allowed action can stop, modify or delete a resource

  @id:accountability.required-permissions.003 @rule:permissions.iam.fire-policy-denies-data-destruction @verified @source:docs/iam-fire.json#NeverTouchTheData @test:docs/iam_test.go#TestFirePolicyDeletesOnlyNatGateways
  Scenario Outline: The firing identity can never delete stored data
    Given an identity has the fire policy attached
    When the identity attempts <action>
    Then the request is refused by an explicit deny in the fire policy

    Examples:
      | action                             |
      | s3:DeleteObject                    |
      | s3:DeleteBucket                    |
      | ec2:DeleteVolume                   |
      | ec2:DeleteSnapshot                 |
      | ec2:DeregisterImage                |
      | rds:DeleteDBInstance               |
      | rds:DeleteDBCluster                |
      | rds:DeleteDBSnapshot               |
      | dynamodb:DeleteTable               |
      | elasticfilesystem:DeleteFileSystem |
      | backup:DeleteBackupVault           |
      | backup:DeleteRecoveryPoint         |

  @id:accountability.required-permissions.004 @rule:permissions.iam.fire-policy-only-reversible-changes @verified @source:docs/iam-fire.json#NatGatewayOptIn @test:docs/iam_test.go#TestFirePolicyDeletesOnlyNatGateways @questionable
  Scenario: The fire policy allows deleting only NAT gateways
    Given an identity has the fire policy attached
    When the operator reviews what the fire policy allows
    Then the only allowed delete action that removes a resource is ec2:DeleteNatGateway
    And lambda:DeleteFunctionConcurrency is also allowed, so a Lambda function's concurrency setting can be removed
    And every other allowed action reads, stops, starts, scales, throttles or modifies a resource, or recreates a NAT gateway and its routes on restore
    And the only S3 write allowed is putting restore records under the killswitch/ prefix of the ops bucket

  @id:accountability.required-permissions.005 @rule:permissions.iam.fire-compute-scoped-by-env-tag @inferred @source:docs/iam-fire.json#StopComputeReversibly @questionable
  Scenario: Compute can be stopped only on resources carrying the configured Env tag
    Given an identity has the fire policy attached with the Env tag set to "prod"
    And an EC2 instance is tagged Env=staging
    When the identity attempts to stop that instance
    Then the request is refused because the instance lacks the Env=prod tag
    And load balancer listeners, CloudFront, API Gateway, NAT gateways and databases are not limited by this tag

  @id:accountability.required-permissions.006 @rule:permissions.iam.lambda-role-assumable-only-by-lambda @verified @source:docs/iam-lambda-trust.json#Statement @test:docs/iam_test.go#TestOnlyLambdaAssumesTheExecutionRole
  Scenario: Only the Lambda service can assume the killswitch execution role
    Given the killswitch execution role uses the shipped trust policy
    When a principal other than the Lambda service attempts to assume the role
    Then the request is refused
    And the trust policy's single statement allows sts:AssumeRole only to the lambda.amazonaws.com service

  @id:accountability.required-permissions.007 @rule:permissions.iam.lambda-extra-cannot-change-resources @verified @source:docs/iam-lambda-extra.json#WriteItsOwnLogs @test:docs/iam_test.go#TestLambdaExtraCannotChangeAResource
  Scenario: The Lambda's extra permissions cannot change any resource
    Given the killswitch execution role has the Lambda extra policy attached
    When the operator reviews what the Lambda extra policy allows
    Then it allows writing only to the aws-killswitch Lambda log groups
    And it allows listing only the ops bucket, to read past fire history
    And it allows no action that stops, modifies or deletes a resource
