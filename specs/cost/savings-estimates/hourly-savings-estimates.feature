@domain:cost @area:savings-estimates @from:awsx/pricing
Feature: Estimating the hourly saving of a plan
  Gives the operator a defensible per-hour figure for what firing a plan stops spending, and admits plainly when there is no estimate, so the decision to fire is not made on a number that claims an unknown is free.

  Background:
    Given the operator has built a shutdown plan for the account

  @id:cost.savings-estimates.003 @rule:cost.savings.nat-gateway-flat-rate @verified @source:internal/awsx/pricing.go#NATGatewayHourlyUSD @test:internal/awsx/pricing_test.go#TestNATIsFlat
  Scenario: Every NAT gateway is estimated at one flat hourly rate
    Given the plan removes a NAT gateway
    When the operator reviews the plan
    Then the plan estimates that NAT gateway's hourly saving at $0.045, whatever its traffic or region

  @id:cost.savings-estimates.004 @rule:cost.savings.list-price-floor @inferred @source:internal/awsx/pricing.go#RDSHourlyUSD @questionable
  Scenario Outline: The estimate is the us-east-1 on-demand single-AZ list price regardless of where or how the resource runs
    Given the plan stops a <kind> of type "<type>" that runs <circumstance>
    When the operator reviews the plan
    Then the plan estimates that resource's hourly saving at $<usd>
    And the saving section is headed "Estimated hourly saving (us-east-1 list price, compute only)"

    Examples:
      | kind         | type        | circumstance             | usd   |
      | EC2 instance | m5.large    | in eu-west-1             | 0.096 |
      | RDS instance | db.m5.large | as a Multi-AZ deployment | 0.342 |
      | RDS instance | db.m5.large | the Oracle engine        | 0.342 |

  @id:cost.savings-estimates.005 @rule:cost.savings.small-and-metal-size-scaling @verified @source:internal/awsx/pricing.go#sizeFactor @test:internal/awsx/pricing_test.go#TestEC2PricesMatchThePublishedTable @questionable
  Scenario Outline: Sizes below medium and bare-metal hosts are priced off fixed multiples that diverge from the published list
    Given the plan stops an EC2 instance of type "<type>"
    When the operator reviews the plan
    Then the plan estimates that instance's hourly saving at $<usd>

    Examples:
      | type     | usd    |
      | t3.nano  | 0.0104 |
      | t3.micro | 0.0208 |
      | t3.small | 0.0416 |
      | m5.metal | 3.072  |
