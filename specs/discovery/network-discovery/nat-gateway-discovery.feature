@domain:discovery @area:network-discovery @from:awsx/discover-network
Feature: Discovering NAT gateways and the routes that depend on them
  Serves the operator who needs the biggest idle network cost found and deleted, with enough recorded to recreate each gateway and its egress exactly as it was.

  Background:
    Given the operator's credentials can read the account's VPC networking in region eu-west-1
    And the policy has delete_nat_gateways set

  @id:discovery.network-discovery.001 @rule:discovery.network.only-available-nat-gateways @inferred @source:internal/awsx/discover_network.go#natGateways
  Scenario Outline: Only NAT gateways that are currently available are discovered
    Given a NAT gateway "nat-0a1b2c3d4e5f60001" in state <state>
    When the operator plans a shutdown
    Then the NAT gateway "nat-0a1b2c3d4e5f60001" <outcome> in the plan

    Examples:
      | state     | outcome                         |
      | available | appears as a nat-gateway to delete |
      | pending   | does not appear                    |
      | deleting  | does not appear                    |
      | deleted   | does not appear                    |
      | failed    | does not appear                    |

  @id:discovery.network-discovery.002 @rule:discovery.network.nat-gateway-routes-recorded @inferred @source:internal/awsx/discover_network.go#natGateways
  Scenario: Every IPv4 route through a NAT gateway is recorded so restore can repoint it
    Given a NAT gateway "nat-0a1b2c3d4e5f60001" in subnet "subnet-0aa11bb22cc33dd44" of VPC "vpc-0123456789abcdef0"
    And route table "rtb-0f1e2d3c4b5a69788" sends "0.0.0.0/0" through that NAT gateway
    And route table "rtb-0f1e2d3c4b5a69799" sends "10.20.0.0/16" through that NAT gateway
    When the operator plans a shutdown
    Then the NAT gateway's recorded prior state names subnet "subnet-0aa11bb22cc33dd44", VPC "vpc-0123456789abcdef0" and its connectivity type
    And its recorded routes are "rtb-0f1e2d3c4b5a69788" → "0.0.0.0/0" and "rtb-0f1e2d3c4b5a69799" → "10.20.0.0/16"

  @id:discovery.network-discovery.003 @rule:discovery.network.nat-gateway-non-ipv4-routes-unrestorable @inferred @source:internal/awsx/discover_network.go#routesVia @questionable
  Scenario Outline: A route through a NAT gateway with an IPv6 or prefix-list destination is recorded without a destination and is not put back
    Given a NAT gateway "nat-0a1b2c3d4e5f60001"
    And route table "rtb-0f1e2d3c4b5a69788" sends <destination> through that NAT gateway
    When the operator plans a shutdown
    Then the NAT gateway's recorded routes list "rtb-0f1e2d3c4b5a69788" with an empty destination
    And restoring the NAT gateway later does not recreate that route

    Examples:
      | destination                        |
      | IPv6 "64:ff9b::/96"                |
      | prefix list "pl-0123456789abcdef0" |

  @id:discovery.network-discovery.004 @rule:discovery.network.nat-gateway-public-address-kept @inferred @source:internal/awsx/discover_network.go#natGateways @questionable
  Scenario Outline: A NAT gateway's Elastic IP is recorded so restore brings back the same public address
    Given a public NAT gateway "nat-0a1b2c3d4e5f60001" using Elastic IP allocation <allocations>
    When the operator plans a shutdown
    Then the NAT gateway's recorded prior state holds allocation <recorded>

    Examples:
      | allocations                                                   | recorded                          | note                           |
      | "eipalloc-0abc1234def567890"                                  | "eipalloc-0abc1234def567890"      | single address                 |
      | "eipalloc-0abc1234def567890" and "eipalloc-0abc1234def567891" | "eipalloc-0abc1234def567890" only | secondary address not recorded |
      | none (private NAT gateway)                                    | no allocation                     | nothing to keep                |

  @id:discovery.network-discovery.005 @rule:discovery.network.nat-gateway-incomplete-record-not-planned @inferred @source:internal/awsx/discover_network.go#natGateways
  Scenario: When a NAT gateway's routes cannot be read, no NAT gateway in that region is planned and the failure is reported
    Given NAT gateways "nat-0a1b2c3d4e5f60001" and "nat-0a1b2c3d4e5f60002" are available
    And the operator's credentials cannot read route tables
    When the operator plans a shutdown
    Then neither NAT gateway appears in the plan
    And the operator sees a discovery warning starting "warning: natgateway in eu-west-1:"
    And resources found by other services in eu-west-1 still appear in the plan

  @id:discovery.network-discovery.006 @rule:discovery.network.nat-gateway-flat-hourly-estimate @inferred @source:internal/awsx/discover_network.go#natGateways @questionable
  Scenario Outline: Each NAT gateway is estimated to save a flat $0.045 per hour in every region
    Given an available NAT gateway "nat-0a1b2c3d4e5f60001" in region <region>
    When the operator plans a shutdown
    Then the NAT gateway's estimated hourly saving is $0.045

    Examples:
      | region         |
      | us-east-1      |
      | eu-west-1      |
      | ap-southeast-1 |
