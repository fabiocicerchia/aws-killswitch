@domain:recovery @area:state-storage @from:state/s3
Feature: Keeping restore records in S3
  Operators keep each fire's restore record in an S3 bucket they own, so a restore still works from another machine, later, under pressure.

  Background:
    Given the operator has an otherwise valid policy file

  @id:recovery.state-storage.001 @rule:recovery.state-storage.state-uri-must-be-s3 @verified @source:internal/state/s3.go#ParseURI @test:internal/state/state_test.go#TestParseURI
  Scenario Outline: A state location that is not an S3 bucket address is refused
    Given the operator's policy names a state_uri of <state_uri>
    When the operator runs any command that needs the state store
    Then the run is <outcome>
    And with bucket <bucket> and prefix <prefix> when accepted

    Examples:
      | state_uri                           | outcome                                                              | bucket        | prefix          |
      | s3://my-ops-bucket/killswitch       | accepted                                                             | my-ops-bucket | killswitch      |
      | s3://my-ops-bucket/killswitch/prod/ | accepted                                                             | my-ops-bucket | killswitch/prod |
      | s3://my-ops-bucket                  | accepted                                                             | my-ops-bucket | (none)          |
      | /var/lib/killswitch                 | refused with "state_uri must be s3://bucket/prefix" and exit code 78 | -             | -               |
      | https://example.com                 | refused with "state_uri must be s3://bucket/prefix" and exit code 78 | -             | -               |
