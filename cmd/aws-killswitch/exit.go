package main

import "errors"

// Exit codes, following sysexits(3).
//
// This tool is meant to be driven by a Budgets action, a CloudWatch alarm or a
// CI job as much as by a person, and those callers get one number. "The policy
// file is missing" and "the policy file says nothing may be stopped" are
// different problems with different fixes, and a single 1 for both makes the
// caller parse an error message to tell them apart.
//
// 3 is deliberately not in this table: `spend --threshold` documents it as the
// over-threshold verdict, so it stays exactly where and what it is.
const (
	exitFail    = 1  // the run failed, or the check did — a result, not a bug
	exitUsage   = 64 // EX_USAGE: wrong invocation, or a refusal to run as asked
	exitDataErr = 65 // EX_DATAERR: the policy file could not be parsed
	exitNoInput = 66 // EX_NOINPUT: a file or snapshot that must exist does not
	exitConfig  = 78 // EX_CONFIG: the policy or the credentials are unusable
)

// codedError carries the exit code a failure should produce.
//
// Anything not wrapped in one exits exitFail, which is the safe default: a new
// error path is never silently given a meaning nobody chose for it.
type codedError struct {
	code int
	err  error
}

func (e codedError) Error() string { return e.err.Error() }
func (e codedError) Unwrap() error { return e.err }

func failWith(code int, err error) error { return codedError{code: code, err: err} }

// exitCode reports the process exit code for an error returned by run.
func exitCode(err error) int {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return exitFail
}
