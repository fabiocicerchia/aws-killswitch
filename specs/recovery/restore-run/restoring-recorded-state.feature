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
