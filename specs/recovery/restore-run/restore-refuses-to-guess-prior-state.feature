@domain:recovery @area:restore-run @from:model/model-test
Feature: Restoring only to the exact recorded prior state
  Protects the operator from a restore that reports success while leaving the account down, by refusing to put back a value it cannot read exactly.

  Background:
    Given a fire has changed the resource and recorded its prior state

  @id:recovery.restore-run.016 @rule:recovery.restore.distribution-enabled-must-be-recorded-exactly @verified @source:internal/awsx/execute_ingress.go#restoreDistribution @test:internal/model/model_test.go#TestPriorBoolOnlyAcceptsARealBool
  Scenario Outline: A distribution whose recorded enabled state is not a true or false value is not restored by guessing
    Given the recorded prior enabled state of CloudFront distribution E1EXAMPLE is <recorded>
    When the operator restores the fire
    Then the restore of E1EXAMPLE <outcome>

    Examples:
      | recorded        | outcome                                                   |
      | true            | re-enables the distribution                               |
      | the text "true" | fails with "no recorded enabled state; refusing to guess" |
      | the number 1    | fails with "no recorded enabled state; refusing to guess" |
      | missing         | fails with "no recorded enabled state; refusing to guess" |

  @id:recovery.restore-run.017 @rule:recovery.restore.stage-throttle-restored-exactly @verified @source:internal/awsx/execute_ingress.go#restoreStage @test:internal/model/model_test.go#TestPriorFloatKeepsFractionalRatesAndRefusesNonsense
  Scenario Outline: An API stage's fractional throttle rate is restored exactly, and an unusable one is refused
    Given API Gateway stage a1b2c3/prod had a recorded default throttle rate of <rate> requests/second and burst 200
    When the operator restores the fire
    Then the restore of a1b2c3/prod <outcome>

    Examples:
      | rate          | outcome                                                     |
      | 10.5          | sets the stage rate back to 10.5 and burst to 200           |
      | 100           | sets the stage rate back to 100 and burst to 200            |
      | -5            | fails with "no usable recorded throttle; refusing to guess" |
      | the text "10" | fails with "no usable recorded throttle; refusing to guess" |

  @id:recovery.restore-run.019 @rule:recovery.restore.lambda-reservation-unreadable-removes-reservation @inferred @source:internal/awsx/execute_compute.go#restoreLambda @questionable
  Scenario Outline: A Lambda function whose recorded reservation flag cannot be read has its reservation removed
    Given the recorded prior state of Lambda function checkout-worker says whether it had a concurrency reservation as <recorded>
    When the operator restores the fire
    Then the restore <outcome>

    Examples:
      | recorded        | outcome                                        |
      | false           | removes the function's concurrency reservation |
      | the text "true" | removes the function's concurrency reservation |
      | missing         | removes the function's concurrency reservation |
