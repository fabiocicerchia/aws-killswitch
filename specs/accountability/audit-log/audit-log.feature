@domain:accountability @area:audit-log @from:audit/audit
Feature: Keeping an answerable audit log of what the killswitch did
  Gives operators and incident reviewers a trustworthy, append-only record of who ran the killswitch, when, and what each event was, without the log ever becoming the reason a cost incident goes unhandled.

  Background:
    Given the operator runs aws-killswitch with an audit log path, by default ~/.aws-killswitch/audit.jsonl

  @id:accountability.audit-log.001 @rule:accountability.audit-log.append-only @inferred @source:internal/audit/audit.go#New
  Scenario: Earlier audit entries are kept when the killswitch runs again
    Given the audit log already holds the entries from yesterday's fire
    When the operator runs aws-killswitch restore
    Then yesterday's entries are still in the audit log, unchanged
    And the restore's entries follow them at the end of the file

  @id:accountability.audit-log.002 @rule:accountability.audit-log.private-to-owner @inferred @source:internal/audit/audit.go#New
  Scenario: A first run creates the audit log readable only by its owner
    Given the directory ~/.aws-killswitch does not exist yet
    When the operator runs aws-killswitch fire for the first time
    Then the directory ~/.aws-killswitch is created, accessible only to the operator
    And audit.jsonl is created inside it, readable and writable only by the operator
    And no "no such file or directory" warning is shown

  @id:accountability.audit-log.003 @rule:accountability.audit-log.author-identity-fallback @inferred @source:internal/audit/audit.go#whoami
  Scenario Outline: Every audit entry names an author, even when the account has no user name
    Given the killswitch runs where the system user name is <system user> and $USER is <USER variable>
    When the killswitch records an event
    Then the audit line's "by" is <recorded author>

    Examples:
      | system user    | USER variable | recorded author |
      | "alice"        | "bob"         | "alice"         |
      | not resolvable | "bob"         | "bob"           |
      | not resolvable | unset         | "unknown"       |

  @id:accountability.audit-log.004 @rule:accountability.audit-log.unavailable-log-does-not-block @inferred @source:internal/audit/audit.go#New
  Scenario: An audit log that cannot be opened is reported but does not stop the run
    Given the audit log path points to a location the operator cannot write
    When the operator runs aws-killswitch fire
    Then a warning "warning: audit log unavailable (<reason>); continuing" is printed on standard error
    And the fire goes ahead
    And no audit entries are recorded for this run
