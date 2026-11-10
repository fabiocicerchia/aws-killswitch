@domain:automation @area:trip-cycle @from:trip/trip-test
Feature: Refusing a duplicate trip after a recent fire
  Keeps a budget alarm that is delivered over and over from firing the killswitch twice, which would record the already-stopped account as its prior state and make restoring it useless.

  Background:
    Given the account's restore records are kept in the configured state store

  @id:automation.trip-cycle.011 @rule:automation.trip-cycle.unreadable-state-does-not-block @verified @source:internal/trip/trip.go#FiredRecently @test:internal/trip/trip_test.go#TestUnreadableStateDoesNotRefuseToFire
  Scenario: A trip is not refused because past restore records cannot be read
    Given the state store refuses to list past restore records, for example with "s3: access denied"
    When the budget alarm trips the killswitch
    Then the cooldown check does not block the trip
    And the trip goes on to discover, plan and fire
    And no warning about the skipped cooldown check is reported

  @id:automation.trip-cycle.012 @rule:automation.trip-cycle.operator-force-overrides-cooldown @inferred @source:internal/trip/trip.go#Fire
  Scenario Outline: Only an operator forcing a fire can override the cooldown
    Given the killswitch fired a plan 15 minutes ago that has not been restored
    When the operator fires with <flags>
    Then <outcome>

    Examples:
      | flags         | outcome                                                                                                                           |
      | --yes         | the fire is refused with "already fired 15m0s ago; inside the 1h0m0s cooldown; pass --force to fire anyway" and a usage exit code |
      | --yes --force | the cooldown is ignored and the account is discovered, planned and fired                                                          |
