@domain:cost @area:current-spend @from:awsx/spend
Feature: Reporting month-to-date spend
  Gives the operator the account's month-to-date AWS bill, and says when it is estimated, so they can decide whether spending has run away before pressing the killswitch.

  Background:
    Given the account has Cost Explorer data for the current month

  @id:cost.current-spend.001 @rule:cost.spend.month-to-date-window @inferred @source:internal/awsx/spend.go#MonthToDate
  Scenario Outline: Spend covers the first of the month through today, in UTC
    Given today is <today> in UTC
    When the operator asks for the month-to-date spend
    Then the reported period starts on <start>
    And the reported period ends on <end>, exclusive
    And the reported amount is the account's unblended cost for that period in USD

    Examples:
      | today      | start      | end        |
      | 2026-11-07 | 2026-11-01 | 2026-11-08 |
      | 2026-11-01 | 2026-11-01 | 2026-11-02 |
      | 2026-11-30 | 2026-11-01 | 2026-12-01 |

  @id:cost.current-spend.002 @rule:cost.spend.estimated-figures-flagged @inferred @source:internal/awsx/spend.go#MonthToDate
  Scenario: Estimated figures are flagged as stale, not refused
    Given Cost Explorer still marks the current month's figures as estimated
    When the operator asks for the month-to-date spend
    Then the month-to-date amount is still reported
    And the result is marked stale
    And the operator sees a warning that the figures are estimated and lag by hours, and that a fast trip should be driven from a Budgets action

  @id:cost.current-spend.003 @rule:cost.spend.unreadable-amount-refused @inferred @source:internal/awsx/spend.go#MonthToDate
  Scenario: An unreadable cost amount fails the report instead of being counted as zero
    Given Cost Explorer returns a cost amount of "n/a" for the month
    When the operator asks for the month-to-date spend
    Then no spend figure is reported
    And the operator sees the error 'unreadable cost amount "n/a"'

  @id:cost.current-spend.004 @rule:cost.spend.forecast-reported @inferred @source:internal/awsx/spend.go#Spend @questionable
  Scenario: The spend forecast is always reported as zero
    Given the account is on course to spend more this month than it has so far
    When the operator asks for the month-to-date spend as JSON
    Then the result contains a Forecast field
    And the Forecast value is 0
