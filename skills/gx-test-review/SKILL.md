---
name: gx-test-review
description: Read-only review of the tests in a diff against the shared test skills and the project's own test docs. Judges coverage, early-passing assertions, timing, isolation, cleanup and diagnostics, plus guideline rules, duplicate coverage and test level. Runs in the code-review batch next to the quality and spec reviews.
disable-model-invocation: true
---

# Test review

Review the tests a diff adds or changes. You only read. Do not edit files and do not run anything:
judge from the text.

## 1. Skip when no test file changed

List the files in the diff. A test file is one named `*_test.go`, `*.test.ts`, `*.test.tsx`,
`*.spec.ts`, or one under a `tests/` or `e2e/` directory. When the diff holds none, report
"no test files changed" and stop.

## 2. Read

- The diff, limited to the test files and the code they cover.
- [gx-test-guidelines](../gx-test-guidelines/SKILL.md), the framework-free rule list, for every test
  file.
- The framework skill that matches the changed files:
  - `*_test.go`: [gx-go-tests](../gx-go-tests/SKILL.md).
  - `*.test.ts`, `*.test.tsx`, `*.spec.ts`: [gx-vitest-tests](../gx-vitest-tests/SKILL.md).
  - `*.e2e.ts`, or a file under `e2e/`: [gx-playwright-tests](../gx-playwright-tests/SKILL.md).
- The reviewed project's own test docs. `.ai/index.md` is the pointer: read it, then follow every
  test doc or skill it names.

## 3. Judge

- **Meaningful coverage:** the test asserts the behaviour a person or the next run sees, not how the
  fix is written. A seam the ticket names, checked weakly (`!= nil`, "did not error", a count), is a
  gap.
- **Assertions that pass early:** a negative assertion with no positive barrier, a one-time read after
  a wait on another source, a read before the thing it needs is ready, a click on a disabled control.
- **Timing assumptions:** a fixed sleep or `waitForTimeout`, a wall-clock assertion, a raised timeout
  or a retry used as the fix, a count of events from a short timer, nested waits longer than the hang
  limit.
- **Isolation:** shared ids, paths or ports, package-level state, reads of the real HOME or live
  machine state, a cache that is an input of the test.
- **Cleanup:** a cleanup that panics when the thing is already gone, a timer, listener, process or
  file that outlives the test. A product timer needs a disposer: a product module that starts a timer
  at module level or in a long-lived object must export a disposer that the test setup can call.
- **Diagnostics:** a failure that does not print the state it waited for (last value read, child
  stderr, log path).
- **Guideline rules:** a new or changed test that breaks a rule in the lists you read. Name the rule.
- **Duplicate coverage:** the new test asserts what an existing test already asserts. Point at both.
- **Test level:** the behaviour is tested at a higher level (e2e, server) than needed, or at a level
  that cannot observe it. Say which level fits, using the levels in the project's test docs.

Report only what you can point at in the diff. Each finding names the file and line, the rule or
reason, and the fix.

## 4. Hand over

Return the findings to the caller. They join the other reviews' findings and go through the
consultant triage in [gx-code-review](../gx-code-review/SKILL.md) (approved, deferred, rejected). No
separate path.
