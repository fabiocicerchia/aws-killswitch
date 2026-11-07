@domain:cost @area:savings-estimates @from:awsx/pricing-test
Feature: Estimating the hourly saving of a shutdown
  Gives the operator reviewing a plan an honest floor for how much per hour stopping each resource saves, and says plainly when no defensible estimate exists.

  Background:
    Given Given savings are estimated from us-east-1 on-demand Linux list prices, compute only

  @id:cost.savings-estimates.006 @rule:cost.savings.instance-list-price @verified @source:internal/awsx/pricing.go#EC2HourlyUSD @test:internal/awsx/pricing_test.go#TestEC2PricesMatchThePublishedTable @test:internal/awsx/pricing_test.go#TestCaseAndWhitespaceDoNotChangeTheAnswer @test:internal/awsx/pricing_test.go#TestSizeFactorsAreMonotonic
  Scenario Outline: A stopped instance is priced from its family's large size, scaled by size
    Given Given the account runs an instance of type <instance_type>
    When When the operator builds a plan
    Then Then the plan estimates stopping that instance saves $<usd_per_hour> per hour

    Examples:
      | instance_type | usd_per_hour |
      | t3.large      | 0.0832       |
      | t3.medium     | 0.0416       |
      | m5.xlarge     | 0.192        |
      | m5.4xlarge    | 0.768        |
      | c6i.2xlarge   | 0.34         |
      | r6i.large     | 0.126        |
      | M5.LARGE      | 0.096        |
      |   m5.large    | 0.096        |

  @id:cost.savings-estimates.007 @rule:cost.savings.burstable-small-sizes @verified @source:internal/awsx/pricing.go#sizeFactor @test:internal/awsx/pricing_test.go#TestEC2PricesMatchThePublishedTable @questionable
  Scenario Outline: Small burstable instances are priced at a fixed fraction of the large size
    Given Given the account runs an instance of type <instance_type>
    When When the operator builds a plan
    Then Then the plan estimates stopping that instance saves $<usd_per_hour> per hour

    Examples:
      | instance_type | usd_per_hour |
      | t3.small      | 0.0416       |
      | t3.micro      | 0.0208       |
      | t3.nano       | 0.0104       |

  @id:cost.savings-estimates.008 @rule:cost.savings.database-list-price @verified @source:internal/awsx/pricing.go#RDSHourlyUSD @test:internal/awsx/pricing_test.go#TestRDSPrices @questionable
  Scenario Outline: A stopped database is priced at its single-AZ open-source engine list price
    Given Given the account runs a database of class <instance_class>
    When When the operator builds a plan
    Then Then the plan estimates stopping that database saves $<usd_per_hour> per hour

    Examples:
      | instance_class | usd_per_hour |
      | db.m5.large    | 0.342        |
      | db.r6g.2xlarge | 1.728        |

  @id:cost.savings-estimates.009 @rule:cost.savings.nat-flat-rate @verified @source:internal/awsx/pricing.go#NATGatewayHourlyUSD @test:internal/awsx/pricing_test.go#TestNATIsFlat
  Scenario: Every NAT gateway is priced at one flat hourly rate
    Given Given the account has a NAT gateway
    When When the operator builds a plan
    Then Then the plan estimates removing that NAT gateway saves $0.045 per hour

  @id:cost.savings-estimates.010 @rule:cost.savings.unpriced-is-not-free @verified @source:internal/awsx/pricing.go#familyPrice @test:internal/awsx/pricing_test.go#TestUnknownIsNotEstimatedRatherThanFree @test:internal/awsx/pricing_test.go#TestOracleAndSQLServerClassesAreNotGuessed
  Scenario Outline: A resource with no known price is reported as not estimated, never as free
    Given Given the account runs a <resource> of type <type>
    When When the operator builds a plan
    Then Then the plan reports that resource's saving as "not estimated"
    And And it adds nothing to the estimated hourly saving

    Examples:
      | resource | type         |
      | instance | x9z.large    |
      | instance | m5.enormous  |
      | instance | m5           |
      | instance | m5.          |
      | instance | .large       |
      | instance |              |
      | database | db.x2g.large |
