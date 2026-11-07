@domain:cost @area:current-spend @from:cmd/aws-killswitch
Feature: Checking month-to-date spend against a threshold
  Lets a cron or operator compare month-to-date cost to a budget and trip the killswitch on a distinct exit code.

  Background:
    Given Cost Explorer reports month-to-date spend for the account

  @id:cost.current-spend.005 @rule:cost.spend.threshold-verdict @inferred @source:cmd/aws-killswitch/commands.go#cmdSpend
  Scenario Outline: Spend above the threshold exits 3; at or below it reports what is left
    Given month-to-date spend is $<spend>
    When the operator runs spend --threshold 500
    Then a "month to date: $<spend>" line with the reporting period is printed
    And <verdict line> is printed
    And the process exits <code>

    Examples:
      | spend  | verdict line                                 | code |
      | 612.40 |   OVER the $500.00 threshold                 | 3    |
      | 320.00 |   under the $500.00 threshold ($180.00 left) | 0    |
      | 500.00 |   under the $500.00 threshold ($0.00 left)   | 0    |

  @id:cost.current-spend.006 @rule:cost.spend.json-ignores-threshold @inferred @source:cmd/aws-killswitch/commands.go#cmdSpend @questionable
  Scenario: Machine-readable spend output never signals an exceeded threshold
    Given month-to-date spend is $612.40
    When the operator runs spend --threshold 500 --json
    Then the month-to-date figures are printed as JSON
    And no over-threshold line is printed
    And the process exits 0

  @id:cost.current-spend.007 @rule:cost.spend.stale-figures-warned @inferred @source:cmd/aws-killswitch/commands.go#cmdSpend
  Scenario: Estimated spend figures carry a lag warning
    Given Cost Explorer marks the month-to-date figures as estimated
    When the operator runs spend
    Then "figures are estimated and lag by hours — for a fast trip, drive this from a Budgets action, not from polling" is printed

  @id:cost.current-spend.008 @rule:cost.spend.no-policy-needed @inferred @source:cmd/aws-killswitch/main.go#run
  Scenario: Spend works without a policy file
    Given no killswitch.json exists
    When the operator runs spend
    Then month-to-date spend is printed
    And no missing-policy error is shown
