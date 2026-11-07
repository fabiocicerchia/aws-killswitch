@domain:cli @area:commands @from:cmd/aws-killswitch
Feature: Firing and restoring from the command line
  Gives the on-call operator a fire that never acts without explicit consent and a restore they can preview first, so pressing the killswitch is safe.

  Background:
    Given a valid policy file scopes the account

  @id:cli.commands.001 @rule:cli.fire.dry-run-without-yes @inferred @source:cmd/aws-killswitch/commands.go#cmdFire
  Scenario: Firing without --yes is a dry run that changes nothing
    Given the policy scope contains resources that can be stopped
    And none of them needs an acknowledgement
    When the operator runs fire without --yes
    Then the plan is printed
    And the output ends with "This was a dry run — --yes was not passed, so nothing changed."
    And no resource is stopped and no restore record is written
    And the process exits 0

  @id:cli.commands.002 @rule:cli.fire.ack-required-needs-force @inferred @source:cmd/aws-killswitch/commands.go#refuseToFire @questionable
  Scenario Outline: A plan carrying acknowledgement warnings is refused without --force
    Given the plan would cause an acknowledgement-required effect such as instance-store data loss
    When the operator runs fire <mode> without --force
    Then "this plan needs --force:" is printed with each warning on a "  ! " line
    And the error "refusing to fire without acknowledgement" is shown
    And nothing is stopped
    And the process exits 64

    Examples:
      | mode          |
      | without --yes |
      | with --yes    |

  @id:cli.commands.003 @rule:cli.fire.cooldown-refuses-second-fire @inferred @source:cmd/aws-killswitch/commands.go#cmdFire
  Scenario: A second fire within the cooldown is refused unless forced
    Given the account was fired 15 minutes ago and not restored
    When the operator runs fire --yes without --force
    Then the error names the cooldown reason followed by "; pass --force to fire anyway"
    And no new restore record is written
    And the process exits 64

  @id:cli.commands.004 @rule:cli.fire.nothing-in-scope-fails @inferred @source:cmd/aws-killswitch/commands.go#refuseToFire
  Scenario: Firing when nothing is in scope fails
    Given no resource in the account matches the policy scope
    When the operator runs fire
    Then the error "nothing in scope to stop" is shown
    And the process exits 1

  @id:cli.commands.005 @rule:cli.fire.partial-failure-reports-restore-path @inferred @source:cmd/aws-killswitch/commands.go#cmdFire
  Scenario: A fire that could not stop everything still gives the restore command and fails
    Given the plan has 5 resources and 2 cannot be stopped
    When the operator runs fire --yes
    Then "plan 20261107-104500: 3 changed, 2 failed" is printed
    And any restore deadlines are listed on "  ! " lines
    And "Restore with:  aws-killswitch restore 20261107-104500 --yes" is printed
    And the error "2 resources could not be stopped; see the audit log" is shown
    And the process exits 1

  @id:cli.commands.006 @rule:cli.restore.dry-run-preview @inferred @source:cmd/aws-killswitch/commands.go#cmdRestore
  Scenario: Restoring without --yes previews what would come back, compute first
    Given plan 20261107-104500 recorded 2 stopped resources
    When the operator runs restore 20261107-104500 without --yes
    Then "plan 20261107-104500 would restore 2 resources, compute first:" is printed
    And one row per resource shows its kind, name and id, and phase, in restore order
    And "Dry run — pass --yes to apply." is printed
    And nothing is restored and the process exits 0

  @id:cli.commands.007 @rule:cli.restore.stops-at-first-failure @inferred @source:cmd/aws-killswitch/commands.go#cmdRestore
  Scenario: A restore stops at the first resource it cannot bring back
    Given plan 20261107-104500 recorded 3 stopped resources
    And the second one in restore order cannot be restored
    When the operator runs restore 20261107-104500 --yes
    Then the first resource is restored
    And the third resource is not attempted
    And the error "restoring <id> failed: ..." is shown with no restored/failed summary
    And the process exits 1

  @id:cli.commands.008 @rule:cli.restore.refuses-missing-or-empty-record @inferred @source:cmd/aws-killswitch/commands.go#cmdRestore
  Scenario Outline: A restore with no usable record is refused with its own exit code
    Given <situation>
    When the operator runs <command>
    Then the error "<message>" is shown
    And the process exits <code>

    Examples:
      | situation                                            | command                       | message                                               | code |
      | no plan id is given                                  | restore --yes                 | restore needs a plan id — see `aws-killswitch status` | 64   |
      | no record exists for plan 20200101-000000            | restore 20200101-000000 --yes | no snapshot with that plan id: 20200101-000000        | 66   |
      | plan 20261107-104500 has already been fully restored | restore 20261107-104500 --yes | this snapshot records no changes to undo              | 66   |
