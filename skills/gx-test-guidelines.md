# Test guidelines

Generic rules for tests that stay fast and stay green. Each rule has a name, why it flakes or
slows, and the fix. A flake counts as a failure (see [gx-failing-tests](gx-failing-tests.md)).
Project-specific rules live in that project's own docs. Playwright-only rules live in the
`gx-playwright-e2es` skill; the state-read rule below is the generic form.

## Timer-count assertions

- **Why:** the test asserts how many events a short timer produced (a 50 ms coalescing window, a
  9 ms gap). Under load the work spreads over more windows and the count changes.
- **Fix:** assert the final state, never the number of events. If a window must be asserted, make
  it a test-controlled clock or a window far wider than the work.

## Product waits with no test override

- **Why:** the product has a long default wait (30 s ready signal, 45 s server start). A test whose
  fake never meets the condition sits out the whole wait, and the suite takes minutes.
- **Fix:** every test that reaches such a wait sets a short override. A wait that can only be met
  by a real dependency gets a fake that meets it.

## State reads without a wait

- **Why:** the test reads state (a DOM value, a computed style, a file) right after the action that
  changes it. The read wins the race on a fast machine and loses it under load.
- **Fix:** wait on the condition with a timeout (poll, or a web-first assertion), then read. Never
  a fixed sleep.

## Host binary assumed

- **Why:** the test needs a tool on `PATH` (`claude`, `codex`, `git` config) that CI or a clean
  machine does not have, or that behaves differently on the developer's machine.
- **Fix:** inject a fake binary or pin the code path that does not need it. Fail loudly on an
  unknown call. Run one CI job with a stripped `PATH` to find the rest.

## Shared ids and state

- **Why:** parallel tests, or test processes, share one constant (a conversation id, a temp
  template, a port). They overwrite each other's state.
- **Fix:** derive ids and paths from `t.Name()` or `t.TempDir()`. No package-level mutable state.

## Exact-count ratchets

- **Why:** a guard that allows exactly N occurrences (sleeps, env reads, old words) passes on each
  branch and fails after two branches merge, late and far from the cause.
- **Fix:** run the guards in the default check, not only on full runs. Keep the allowlist sorted so
  merges conflict early instead of failing later.

## Generated-file drift

- **Why:** a checked-in generated file goes stale. The freshness check runs only in CI, so the
  failure arrives after the push.
- **Fix:** run the generator check in the local check. Name the regenerate command in the failure
  message.

## Hidden failure causes

- **Why:** a helper drops a child's stderr or its reason (`no output line: EOF`, `not cancelled:`
  with no state). Nobody can tell a flake from a bug, so the fix is a guess.
- **Fix:** a helper that ends a test must print why: the child's exit status and stderr, or the
  state it saw. Never truncate names or reasons in failure output.

## Wall clock

- **Why:** the test uses `time.Now()` day math, the time zone or the hour. It passes at 14:00 and
  fails near midnight or in another zone.
- **Fix:** inject a clock and seed fixed instants. Pin the time zone in the test.

## Stale assertions

- **Why:** the product change was decided and merged, but a test still asserts the old value. It
  fails on every run, and a follow-up marked done hides it.
- **Fix:** change the assertion in the same commit as the behavior. When closing a fix, check that
  the fix is on the main branch.

## Fail-fast hiding a platform

- **Why:** a CI matrix with fail-fast cancels the other platforms when one fails. A platform that
  is cancelled most of the time is unknown, not green.
- **Fix:** turn fail-fast off for test matrices, so every platform reports on every run.
