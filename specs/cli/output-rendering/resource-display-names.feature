@domain:cli @area:output-rendering @from:awsx/helpers
Feature: Naming discovered resources in the plan
  Operators reviewing a shutdown plan need each row to carry the name they know the resource by, and never a blank.

  Background:
    Given the operator has an AWS account with running EC2 instances and NAT gateways

  @id:cli.output-rendering.001 @rule:discovery.naming.name-tag-or-id @inferred @source:internal/awsx/helpers.go#nameOf
  Scenario Outline: A resource is shown by its Name tag, or by its id when it has none
    Given a <kind> with id <id> whose Name tag is <name_tag>
    When the operator builds a plan for the account
    Then the plan lists the <kind> under the name <shown_as>

    Examples:
      | kind         | id                    | name_tag     | shown_as              |
      | EC2 instance | i-0abc123def4567890   | web-frontend | web-frontend          |
      | EC2 instance | i-0abc123def4567890   | (absent)     | i-0abc123def4567890   |
      | EC2 instance | i-0abc123def4567890   | (empty)      | i-0abc123def4567890   |
      | NAT gateway  | nat-0123456789abcdef0 | egress-a     | egress-a              |
      | NAT gateway  | nat-0123456789abcdef0 | (absent)     | nat-0123456789abcdef0 |
