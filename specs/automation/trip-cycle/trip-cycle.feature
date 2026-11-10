@domain:automation @area:trip-cycle @from:trip/trip
Feature: Running one killswitch trip
  Gives the operator and the automated budget trigger one shared discover, plan and fire cycle, so a repeated or unattended trip cannot stop the account twice or fire something nobody acknowledged.

  Background:
    Given an account with running resources in the policy's scope
    And a restore record store the trip can write to

  @id:automation.trip-cycle.001 @rule:automation.trip-cycle.cooldown-blocks-repeat-fire @verified @source:internal/trip/trip.go#Fire @test:internal/trip/trip_test.go#TestARepeatedNotificationInsideTheCooldownDoesNotDoubleFire @test:internal/trip/trip_test.go#TestAFireOlderThanTheCooldownDoesNotBlockANewOne @test:internal/trip/trip_test.go#TestARestoredSnapshotDoesNotBlockANewFire @test:internal/trip/trip_test.go#TestAPlannedButNeverFiredSnapshotDoesNotBlock @test:internal/trip/trip_test.go#TestAFireStampedInTheFutureIsIgnored
  Scenario Outline: A second trip within an hour of an unrestored fire is refused
    Given a previous fire recorded <fired> that is <restored>
    When the trip is triggered again without an override
    Then the trip outcome is <outcome>
    And when refused, the reason reads "already fired <ago> ago; inside the 1h0m0s cooldown"
    And when refused, no resource is discovered or changed and no new restore record is written

    Examples:
      | fired                                 | restored               | outcome | ago   |
      | 15 minutes ago                        | not restored           | refused | 15m0s |
      | 2 hours ago                           | not restored           | fired   |       |
      | 10 minutes ago                        | restored 5 minutes ago | fired   |       |
      | never (planned only)                  | not restored           | fired   |       |
      | 30 minutes in the future (clock skew) | not restored           | fired   |       |

  @id:automation.trip-cycle.003 @rule:automation.trip-cycle.empty-plan-fires-nothing @inferred @source:internal/trip/trip.go#Fire
  Scenario: A trip with nothing in scope stops nothing
    Given no resource in the policy's scope is stoppable
    When the trip is triggered
    Then nothing is fired
    And the reason reads "nothing in scope to stop"

  @id:automation.trip-cycle.004 @rule:automation.trip-cycle.acknowledgement-required-blocks-unattended @inferred @source:internal/trip/trip.go#Fire
  Scenario Outline: A plan with actions needing acknowledgement is not fired without force
    Given the plan contains <n> action(s) that require acknowledgement
    When the trip is triggered with force <force>
    Then the trip outcome is <outcome>
    And when refused, the reason reads "<n> action(s) need acknowledgement; not firing unattended"

    Examples:
      | n | force | outcome |
      | 2 | off   | refused |
      | 2 | on    | fired   |

  @id:automation.trip-cycle.005 @rule:automation.trip-cycle.dry-run-changes-nothing @inferred @source:internal/trip/trip.go#Fire
  Scenario: A dry-run trip changes nothing and is never held by the cooldown
    Given a previous fire 15 minutes ago that is not restored
    When the trip is triggered as a dry run
    Then the plan is built and reported
    And no resource is changed
    And the outcome reports not fired

  @id:automation.trip-cycle.006 @rule:automation.trip-cycle.discovery-failures-degrade-not-abort @inferred @source:internal/trip/trip.go#Discover
  Scenario: A service that cannot be read degrades the trip instead of stopping it
    Given one service in one region refuses to be listed for lack of permission
    When the trip is triggered
    Then resources from every other service and region are still planned and fired
    And the listing failure is reported as a warning alongside the outcome

  @id:automation.trip-cycle.008 @rule:automation.trip-cycle.plan-id-is-utc-timestamp @verified @source:internal/trip/trip.go#PlanID @test:internal/trip/trip_test.go#TestPlanIDIsSortableAndSecondPrecise
  Scenario: Each trip's plan is identified by its UTC start time to the second
    Given a trip starts at 2026-09-02 12:00:00 UTC
    When the trip builds its plan
    Then the plan id is "20260902-120000"
    And a trip one second later gets a plan id that sorts after it
