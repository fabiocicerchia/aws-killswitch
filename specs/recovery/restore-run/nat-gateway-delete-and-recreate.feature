@domain:recovery @area:restore-run @from:awsx/execute-network
Feature: Deleting and recreating NAT gateways
  Operators who fire the killswitch on a NAT gateway need it gone while the account is stopped, and need private subnets to get their egress back when they restore.

  Background:
    Given a NAT gateway was found in an account region, with its subnet, connectivity type, Elastic IP allocation and the routes pointing at it recorded in the restore record

  @id:recovery.restore-run.001 @rule:recovery.nat-gateway.fire-deletes-gateway @inferred @source:internal/awsx/execute_network.go#deleteNATGateway
  Scenario: Firing deletes a NAT gateway instead of stopping it
    Given the approved plan includes deleting the NAT gateway nat-0a1b2c3d4e5f60789
    When the operator fires the plan
    Then NAT gateway nat-0a1b2c3d4e5f60789 is deleted
    And its Elastic IP allocation eipalloc-0123456789abcdef0 is kept in the account

  @id:recovery.restore-run.002 @rule:recovery.nat-gateway.recreated-with-same-public-address @inferred @source:internal/awsx/execute_network.go#restoreNATGateway
  Scenario: Restoring recreates the NAT gateway in its subnet with the same public address
    Given NAT gateway nat-0a1b2c3d4e5f60789 in subnet subnet-0aa11bb22cc33dd44 with allocation eipalloc-0123456789abcdef0 and connectivity type public was deleted by a fire
    When the operator restores the fire
    Then a new NAT gateway exists in subnet subnet-0aa11bb22cc33dd44
    And the new NAT gateway uses allocation eipalloc-0123456789abcdef0 and so the same public IP address
    And the new NAT gateway has connectivity type public

  @id:recovery.restore-run.003 @rule:recovery.nat-gateway.routes-repointed @inferred @source:internal/awsx/execute_network.go#repointRoutes
  Scenario Outline: Restoring sends every recorded route back through the recreated NAT gateway
    Given route table rtb-0f1e2d3c4b5a69788 had route <destination> through the deleted NAT gateway
    And after the fire that route <route_state>
    When the operator restores the fire
    Then route <destination> in rtb-0f1e2d3c4b5a69788 targets the recreated NAT gateway

    Examples:
      | destination  | route_state                     |
      | 0.0.0.0/0    | is still present but blackholed |
      | 10.20.0.0/16 | was removed                     |

  @id:recovery.restore-run.004 @rule:recovery.nat-gateway.non-ipv4-routes-not-restored @inferred @source:internal/awsx/execute_network.go#repointRoutes @questionable
  Scenario: Routes without an IPv4 destination are not put back through the recreated NAT gateway
    Given route table rtb-0f1e2d3c4b5a69788 had an IPv6 route 64:ff9b::/96 through the deleted NAT gateway
    When the operator restores the fire
    Then the NAT gateway is recreated
    And route 64:ff9b::/96 in rtb-0f1e2d3c4b5a69788 is not pointed at the recreated NAT gateway
    And the restore reports no error for that route

  @id:recovery.restore-run.005 @rule:recovery.nat-gateway.refused-without-subnet @inferred @source:internal/awsx/execute_network.go#restoreNATGateway
  Scenario: A NAT gateway with no recorded subnet cannot be restored
    Given the restore record for NAT gateway nat-0a1b2c3d4e5f60789 has no subnet
    When the operator restores the fire
    Then no NAT gateway is created
    And the restore of nat-0a1b2c3d4e5f60789 fails with "no recorded subnet; cannot recreate"

  @id:recovery.restore-run.006 @rule:recovery.nat-gateway.partial-restore-reported @inferred @source:internal/awsx/execute_network.go#restoreNATGateway
  Scenario Outline: A NAT gateway restore that only partly succeeds names what was left undone
    Given NAT gateway nat-0a1b2c3d4e5f60789 was deleted by a fire
    When the operator restores the fire and <failure>
    Then the restore of the NAT gateway fails with "<message>"
    And the recreated NAT gateway is left in place

    Examples:
      | failure                                                                      | message                                                                                  |
      | the new gateway is not available within 10 minutes                           | created nat-0ffee0ffee0ffee00 but it did not become available                            |
      | route 0.0.0.0/0 in rtb-0f1e2d3c4b5a69788 can neither be replaced nor created | recreated nat-0ffee0ffee0ffee00 but could not repoint 0.0.0.0/0 in rtb-0f1e2d3c4b5a69788 |
