@domain:cli @area:exit-codes @from:cmd/aws-killswitch
Feature: Invocation errors and exit codes
  Gives cron jobs, Budgets actions and CI a distinct exit number for each kind of wrong invocation, so they can react without parsing messages.

  @id:cli.exit-codes.001 @rule:cli.exit.wrong-command-is-usage @verified @source:cmd/aws-killswitch/main.go#run @test:cmd/aws-killswitch/exit_test.go#TestUnknownCommandExitsUsage
  Scenario Outline: No command or an unknown command prints usage and exits 64
    When the operator runs aws-killswitch <invocation>
    Then the usage text listing plan, fire, status, restore, spend and verify is printed
    And <error>
    And the process exits 64

    Examples:
      | invocation        | error                                         |
      | with no arguments | no error line is shown                        |
      | detonate          | the error "unknown command detonate" is shown |

  @id:cli.exit-codes.002 @rule:cli.exit.unknown-flag-exits-two @inferred @source:cmd/aws-killswitch/main.go#main @questionable
  Scenario: An unknown flag exits 2, not the documented usage code
    When the operator runs aws-killswitch plan --dry-run
    Then the usage text is printed
    And the process exits 2

  @id:cli.exit-codes.003 @rule:cli.exit.unclassified-failure-is-one @verified @source:cmd/aws-killswitch/exit.go#exitCode @test:cmd/aws-killswitch/exit_test.go#TestUnclassifiedErrorExitsFail
  Scenario: Any failure without its own meaning exits 1
    Given the state store becomes unreachable mid-run
    When the operator runs status
    Then the error is printed prefixed "error: "
    And the process exits 1

  @id:cli.exit-codes.004 @rule:cli.exit.flags-anywhere @inferred @source:cmd/aws-killswitch/main.go#main
  Scenario: Flags may come before or after the plan id
    Given plan 20261107-104500 has stopped resources
    When the operator runs restore --yes 20261107-104500 --json
    Then 20261107-104500 is taken as the plan id
    And --yes applies and the restore runs
