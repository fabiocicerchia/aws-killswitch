@domain:cli @area:output-rendering @from:cmd/aws-killswitch
Feature: Reading a shutdown plan
  Shows the operator what a fire would stop, what it would leave alone and why, and what it would save, before anything changes.

  Background:
    Given a valid policy file scopes the account

  @id:cli.output-rendering.002 @rule:cli.render.plan-shows-changes-and-refusals @inferred @source:cmd/aws-killswitch/render.go#printPlan
  Scenario: The plan lists changes by phase and every untouched resource with its reason
    Given the scope contains a load balancer listener and a Lambda function
    And an RDS database is excluded because databases are not included
    When the operator runs plan
    Then a header names the plan id, account and regions
    And each non-empty phase is shown in capitals with its reason and one row per action
    And "NOT TOUCHED (1)" lists the database with its reason
    And "2 resources would change" is printed
    And "Nothing has changed. Run `fire --yes` to apply." closes the output

  @id:cli.output-rendering.003 @rule:cli.render.unpriced-not-shown-as-free @inferred @source:cmd/aws-killswitch/render.go#printSavings
  Scenario: Resources with no price estimate are reported as not estimated, never as free
    Given 3 EC2 instances are priced at $0.42/hour in total
    And 1 EC2 instance and 1 Lambda function have no price estimate
    When the operator runs plan
    Then "Estimated hourly saving (us-east-1 list price, compute only):" is printed
    And the ec2-instance line shows "$0.42/hour  (1 not estimated)"
    And the lambda line shows "not estimated  (1 not estimated)"

  @id:cli.output-rendering.004 @rule:cli.render.empty-plan @inferred @source:cmd/aws-killswitch/render.go#printPlan
  Scenario: An empty plan says nothing would change and offers no fire hint
    Given no resource matches the policy scope
    When the operator runs plan
    Then "Nothing in scope would be changed." is printed
    And "0 resources would change" is printed
    And no "Run `fire --yes` to apply." line is printed

  @id:cli.output-rendering.005 @rule:cli.render.json-output @inferred @source:cmd/aws-killswitch/render.go#emit
  Scenario Outline: --json prints the result as indented JSON instead of text
    When the operator runs <command> --json
    Then <result> is printed to stdout as two-space-indented JSON
    And no human-readable text is printed to stdout

    Examples:
      | command            | result                      |
      | plan               | the plan                    |
      | status             | the list of restore records |
      | spend              | the month-to-date summary   |
      | verify --plan-only | the verification report     |

  @id:cli.output-rendering.006 @rule:cli.render.long-names-truncated @inferred @source:cmd/aws-killswitch/render.go#truncate @questionable
  Scenario: Long resource names are cut to the column width with an ellipsis
    Given a resource's reference is 60 characters long
    When the operator runs plan
    Then its reference shows its first 39 characters followed by "…"
    And the notes column stays aligned
