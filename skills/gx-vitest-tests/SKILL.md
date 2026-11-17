---
name: gx-vitest-tests
description: Rules for writing Vitest tests that stay green. Covers mocks, waits, shared caches, product timers, fake timers and test environments. Use when writing or reviewing Vitest tests, or when a Vitest test is flaky.
---

# Vitest tests

These rules add the Vitest forms to the common rules in
[gx-test-guidelines](../gx-test-guidelines/SKILL.md). Read those first. Where the project's own test
docs set a rule, the project's rule wins.

## Prefer spyOn over mock

- **Why:** `vi.mock` is not type safe. If the mocked module is removed or changed, nothing fails
  at compile time.
- **Fix:** use `vi.spyOn`, even when the existing code uses `vi.mock`. Use `vi.mock` only when
  `vi.spyOn` cannot work.

## Blank line between blocks

- **Why:** a blank line between `describe` and `it` blocks makes each case easy to find.
- **Fix:** put one blank line between every `describe` and `it` block.

## Wait on what you read

- **Why:** a test can wait on text from fetch A and then assert on output from fetch B. The wait
  passes, B has not arrived, and the assertion fails. One page showed a card from the list fetch,
  while the test waited on the detail fetch.
- **Fix:** wait on the output the assertion reads. If the assertion reads fetch B, wait on text
  that only fetch B produces.

## Renders share one cache

- **Why:** two renders in one test share `sessionStorage` and any page cache. The second render
  can paint the first render's content, and a wait on that content passes too early.
- **Fix:** clear `sessionStorage` and the page cache between the renders. Or wait on text that
  proves the new content painted.

## Never click a disabled control

- **Why:** `fireEvent.click` on a disabled button does nothing and reports no error. The test then
  waits for an effect that never starts.
- **Fix:** wait until the control is enabled, then click it.

## Never fix a timeout with a longer wait

- **Why:** a wait that ran the whole timeout means the condition never became true. A longer wait
  only makes the failure slower. In one case focus never reached a dialog, so the key press went
  to the wrong element.
- **Fix:** find the lost event or the stale state and fix that. Do not add a retry and do not raise
  a timeout.

## A product timer needs a disposer

- **Why:** a timer in a product module that nobody cancels fires after the test environment is
  gone. It throws (for example `ReferenceError: document is not defined`) and fails the run while
  every test is green.
- **Fix:** a function-local timer is cleared on unmount. A module-level timer exports a disposer,
  and the test setup runs it in `afterEach`.

## Prove a race with an injected delay

- **Why:** a race you cannot make fail is a guess, and a fix for it proves nothing.
- **Fix:** add a delay to the fake that the code waits on and run only that test file. See the
  test fail for the race's reason. Then remove the delay and see it pass.

## Check the whole run

- **Why:** Vitest can print an `Errors` line and exit with code 1 while no test failed, for
  example for an error thrown after a test ended. A summary that lists only failed tests hides it.
- **Fix:** read the exit code and the full output, not only the list of failed tests.

## Use the global async wait

- **Why:** the project sets one async wait for `waitFor` and `findBy*`. A shorter timeout on one
  call makes it flaky on a slow machine. A longer one hides a condition that never comes true.
- **Fix:** call `waitFor` and `findBy*` with no timeout option and use the global wait.

## Name a non-default environment

- **Why:** a test file runs in the project's default environment unless it says otherwise. A reader
  cannot tell which one applies, and a DOM assertion such as `document` or `screen` cannot be met
  in a node-environment file.
- **Fix:** when the file needs another environment, put `// @vitest-environment node` (or the
  other name) at the top. Keep DOM assertions out of node-environment files.

## Restore real timers

- **Why:** fake timers that stay installed leak into the next test. A timer still pending when the
  test ends can fire later and fail another test.
- **Fix:** call `vi.useRealTimers()` in `afterEach`. Run or clear every pending timer before the
  test ends.
