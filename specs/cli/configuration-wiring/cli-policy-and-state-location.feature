@domain:cli @area:configuration-wiring @from:cmd/aws-killswitch
Feature: Loading the policy, credentials and restore-record location
  Makes sure the tool never acts on a missing or malformed policy and tells the operator where the restore record will live, because losing it means losing the way back.

  @id:cli.configuration-wiring.001 @rule:cli.config.policy-required-and-valid @verified @source:cmd/aws-killswitch/wiring.go#policyFor @test:cmd/aws-killswitch/exit_test.go#TestMissingPolicyFileExitsNoInput @test:cmd/aws-killswitch/exit_test.go#TestUnparsablePolicyExitsDataErr @test:cmd/aws-killswitch/exit_test.go#TestUnscopedPolicyExitsConfig
  Scenario Outline: A missing, malformed or unscoped policy stops the run before discovery
    Given <policy>
    When the operator runs plan
    Then <message> is shown
    And no account discovery happens
    And the process exits <code>

    Examples:
      | policy                                                  | message                                                                                            | code |
      | no file exists at killswitch.json                       | "no policy at killswitch.json — this tool will not act without one..." followed by a sample policy | 66   |
      | killswitch.json holds truncated JSON                    | the file path and the parse error                                                                  | 65   |
      | killswitch.json holds an unknown field such as "scopes" | the file path and the unknown-field error                                                          | 65   |
      | killswitch.json has an empty scope                      | the file path and the validation error                                                             | 78   |

  @id:cli.configuration-wiring.002 @rule:cli.config.state-location @verified @source:cmd/aws-killswitch/wiring.go#buildStore @test:cmd/aws-killswitch/exit_test.go#TestUnusableStateURIExitsConfig
  Scenario Outline: The restore record goes to S3 and a local copy, or locally with a warning
    Given <state setting>
    When the operator runs plan
    Then <outcome>

    Examples:
      | state setting                                                 | outcome                                                                                                          |
      | the policy has no state_uri                                   | a warning says snapshots are only in ~/.aws-killswitch and losing it loses the restore record; the run continues |
      | state_uri is s3://ops-bucket/killswitch                       | records are kept in that S3 prefix first and in the local directory too                                          |
      | --state s3://other-bucket/ks overrides a configured state_uri | records are kept in s3://other-bucket/ks                                                                         |
      | state_uri is gs://not-s3/x                                    | the error "state_uri must be s3://bucket/prefix, got \"gs://not-s3/x\"" is shown and the process exits 78        |

  @id:cli.configuration-wiring.003 @rule:cli.config.unusable-credentials-refused @inferred @source:cmd/aws-killswitch/main.go#run
  Scenario: Unusable AWS credentials stop every command
    Given the AWS configuration cannot be loaded, for example the named profile does not exist
    When the operator runs plan
    Then the error begins "loading AWS credentials:"
    And the process exits 78

  @id:cli.configuration-wiring.004 @rule:cli.config.degrade-not-abort @inferred @source:cmd/aws-killswitch/wiring.go#warnAll
  Scenario Outline: An unavailable audit log or a refused discovery call warns but does not stop the run
    Given <problem>
    When the operator runs plan
    Then the line "<warning>" is printed to stderr
    And the plan is still printed
    And the process exits 0

    Examples:
      | problem                                                    | warning                                          |
      | the audit log path is not writable                         | warning: audit log unavailable (...); continuing |
      | one service refuses a Describe call for lack of permission | warning: <the discovery error>                   |
