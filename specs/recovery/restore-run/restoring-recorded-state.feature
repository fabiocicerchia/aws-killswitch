@domain:recovery @area:restore-run @from:model/model
Feature: Restoring exactly what was recorded
  Gives the operator a restore that puts back the recorded prior state exactly, or refuses, so a restore never reports success while leaving the account down.

  Background:
    Given a fire has been recorded for account 111122223333

  @id:recovery.restore-run.013 @rule:recovery.restore-run.unusable-recorded-count-refused @verified @source:internal/model/model.go#PriorInt32 @test:internal/plan/plan_test.go#TestPriorInt32RefusesRatherThanWrapping
  Scenario Outline: A recorded capacity that is missing, non-numeric or out of range is refused, not guessed
    Given the fire record holds an ECS service whose recorded desired count is <recorded>
    When the operator restores the fire
    Then the service is not rescaled
    And the restore reports "no usable recorded desired count; refusing to guess"

    Examples:
      | recorded     |
      | missing      |
      | the text "3" |
      | -1           |
      | 4294967296   |
      | 5000000000   |

  @id:recovery.restore-run.014 @rule:recovery.restore-run.enabled-flag-must-be-a-real-boolean @verified @source:internal/model/model.go#PriorBool @test:internal/model/model_test.go#TestPriorBoolOnlyAcceptsARealBool
  Scenario Outline: A recorded on/off setting is restored only from a real true or false
    Given the fire record holds a CloudFront distribution whose recorded enabled setting is <recorded>
    When the operator restores the fire
    Then the setting is <outcome>

    Examples:
      | recorded        | outcome                          |
      | true            | read as enabled                  |
      | false           | read as disabled                 |
      | the text "true" | not accepted as a recorded value |
      | the number 1    | not accepted as a recorded value |

  @id:recovery.restore-run.015 @rule:recovery.restore-run.fractional-rates-kept-exactly @verified @source:internal/model/model.go#PriorFloat @test:internal/model/model_test.go#TestPriorFloatKeepsFractionalRatesAndRefusesNonsense
  Scenario Outline: A recorded throttle rate is restored to its exact value, fractions included
    Given the fire record holds an API Gateway stage whose recorded rate is <recorded>
    When the operator restores the fire
    Then the rate is <outcome>

    Examples:
      | recorded      | outcome                      |
      | 10.5          | read as 10.5 requests/second |
      | 100           | read as 100 requests/second  |
      | -1            | not accepted as a rate       |
      | the text "10" | not accepted as a rate       |
