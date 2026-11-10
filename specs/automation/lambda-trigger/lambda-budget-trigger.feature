@domain:automation @area:lambda-trigger @from:cmd/killswitch-lambda
Feature: Firing the killswitch from a budget alert
  Lets an operator have AWS Budgets trip the killswitch unattended the moment a spend threshold is crossed, without ever giving up the restore path.

  Background:
    Given the killswitch is deployed as a Lambda function subscribed to a Budgets notification

  @id:automation.lambda-trigger.001 @rule:automation.lambda.refuses-without-durable-restore-record @inferred @source:cmd/killswitch-lambda/main.go#handle
  Scenario Outline: The serverless trigger refuses to fire when no S3 restore record location is configured
    Given the policy's state_uri is <state_uri>
    When a budget alert invokes the killswitch
    Then nothing in the account is stopped
    And the invocation fails with an error saying <message>

    Examples:
      | state_uri             | message                                                  |
      | (unset)               | no state store: set state_uri, or pass a local directory |
      | /var/killswitch/state | state_uri must be s3://bucket/prefix                     |

  @id:automation.lambda-trigger.002 @rule:automation.lambda.policy-from-environment @inferred @source:cmd/killswitch-lambda/main.go#loadPolicy
  Scenario Outline: The policy is taken inline from the environment first, then from a packaged file, and its absence refuses the run
    Given KILLSWITCH_POLICY is <inline>
    And KILLSWITCH_POLICY_PATH is <path>
    When a budget alert invokes the killswitch
    Then the policy used is <used>

    Examples:
      | inline                  | path                              | used                                                                                                                                          |
      | a valid policy document | killswitch.json in the package    | the inline policy                                                                                                                             |
      | blank                   | killswitch.json in the package    | killswitch.json from the package                                                                                                              |
      | blank                   | unset                             | none: the invocation fails with "no policy: set KILLSWITCH_POLICY to the file's contents, or KILLSWITCH_POLICY_PATH to a path in the package" |
      | blank                   | missing.json (not in the package) | none: the invocation fails with "reading missing.json: ..."                                                                                   |

  @id:automation.lambda-trigger.003 @rule:automation.lambda.invalid-policy-refused @inferred @source:cmd/killswitch-lambda/main.go#handle
  Scenario: An invalid policy stops the trigger before anything is discovered or changed
    Given the configured policy fails validation
    When a budget alert invokes the killswitch
    Then nothing in the account is stopped
    And the invocation fails with an error starting "policy:"

  @id:automation.lambda-trigger.006 @rule:automation.lambda.dry-run-test-invocation @inferred @source:cmd/killswitch-lambda/main.go#handle
  Scenario: A dry-run invocation exercises discovery and planning without stopping anything
    Given the account has running resources in scope
    When the operator invokes the killswitch directly with dry_run true
    Then nothing in the account is changed
    And the response has fired false, dry_run true and a plan_id
    And the response trigger is "direct invocation"

  @id:automation.lambda-trigger.007 @rule:automation.lambda.response-reports-outcome @inferred @source:cmd/killswitch-lambda/main.go#Response
  Scenario Outline: Each invocation reports what it did, what triggered it and which services could not be inspected
    Given a budget notification with subject <subject> and message id msg-0001
    And discovery could not read rds in eu-west-1 for lack of permission
    When the budget alert invokes the killswitch
    Then the response and the CloudWatch log line "killswitch: {...}" carry fired, changed, failed, plan_id and account_id
    And the response trigger is <trigger>
    And the response warnings include one beginning "rds in eu-west-1: "

    Examples:
      | subject                                                    | trigger                                                               |
      | AWS Budgets: monthly-cap has exceeded your alert threshold | AWS Budgets: monthly-cap has exceeded your alert threshold (msg-0001) |
      | (empty)                                                    | budgets notification (msg-0001)                                       |
