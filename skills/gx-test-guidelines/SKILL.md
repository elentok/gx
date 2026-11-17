---
name: gx-test-guidelines
description: Rules for tests that stay fast and stay green, in any language or framework. Use when writing, changing or reviewing a test, or when a test is flaky or slow.
---

# Test guidelines

Common rules for tests that stay fast and stay green. Each rule has a name, why the test flakes or
slows, and the fix. A flake counts as a failure. Language rules live in the language's own skill,
for example [gx-go-tests](../gx-go-tests/SKILL.md). Project-specific rules live in that project's
own docs.

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

- **Why:** the test reads state (a screen value, a style, a file) right after the action that
  changes it. The read wins the race on a fast machine and loses it under load.
- **Fix:** wait on the condition with a timeout, then read. Never a fixed sleep.

## Waiting on the wrong signal

- **Why:** the test waits for the first thing that exists, not for the signal that means "ready for
  this action". A listening socket is not a program that reads input. A file that exists is not a
  finished write. Input sent too early is lost, and the test then waits for an effect that never
  comes. An echo of the input on a terminal also proves nothing, because the echo comes before the
  program reads it.
- **Fix:** name the readiness point (an acknowledgement, a status event) and wait for it. Check the
  surface the test is about, after the program has acknowledged the input.

## Counters that do not count the event

- **Why:** the success signal is a counter ("a turn started"), but the fake moves it on startup, not
  on the event. The test passes by accident. It fails later, when the baseline moves.
- **Fix:** make the fake do the work and assert that the event happened. Check what really moves the
  counter.

## Baseline taken at the wrong time

- **Why:** a wait compares against a baseline taken earlier. A baseline taken before the first read
  and one taken after it differ by exactly the thing the test waits for. The wait is then satisfied
  at once, or never.
- **Fix:** ask when the baseline is taken, relative to the first observation. Take it after the
  state is stable, and assert the baseline value.

## Quiet window as completion

- **Why:** the test treats "no change for 500 ms" as proof that async work has finished. Under load
  the work pauses longer than the window, and the test moves on too early.
- **Fix:** identify the work by an id or a final state, and wait for that.

## Nested waits that outgrow the limit

- **Why:** a start timeout, a slot wait and a poll add up to more than the limit that kills a hung
  test. The kill comes first, so the diagnostic never prints.
- **Fix:** make the inner waits fit inside the outer limit, with room to print the state.

## Cleanups that fail on "already gone"

- **Why:** a cleanup that raises an error for a missing file or process ends the whole test run
  when something else already removed it. Every test still running fails with it.
- **Fix:** a cleanup tolerates "already gone" and never aborts for a missing fixture.

## States shorter than a scheduling delay

- **Why:** the state lasts about as long as a scheduling delay (a 1 s countdown). Polling cannot see
  it reliably.
- **Fix:** hold the timer in the test, or observe an event instead of polling.

## Races proved by luck

- **Why:** a rare failure (1 in 100) cannot be fixed with confidence by repeating the run. A rate
  that low shows nothing: 0 of 80 against 0 of 80 proves nothing.
- **Fix:** inject the suspected delay at the seam (a pause before the baseline, a stall before the
  read) so the failure is deterministic. Then the fix goes from failing to passing in a few runs.
  Use repeats under load to find the shape of a failure, not to prove a fix.

## A long timeout for a missed event

- **Why:** a wait that fails after a long pause means the condition never became true, usually
  because an event was lost or can no longer happen. A longer timeout only hides the lost event.
- **Fix:** find the event that did not arrive. Look at what the code under test reads, and wait on
  that state.

## Classifying failures by signature

- **Why:** reading code first sends the search in the wrong direction. A family of failures can look
  like one cause and be another.
- **Fix:** group the failure logs by their message first. Count each signature. Then read code for
  the largest group, and rule out a cause when no log carries its message.

## Host binary assumed

- **Why:** the test needs a tool on the search path (an agent CLI, a version-control config) that CI
  or a clean machine does not have, or that behaves differently on the developer's machine.
- **Fix:** inject a fake binary or pin the code path that does not need it. Fail loudly on an
  unknown call. Run one CI job with a stripped search path to find the rest.

## Shared ids and state

- **Why:** parallel tests, or test processes, share one constant (a conversation id, a temp
  template, a port). They overwrite each other's state.
- **Fix:** derive ids and paths from the test's own name or its own private directory. Keep no
  mutable state shared between tests.

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

## Presentation text as a comparison key

- **Why:** a check compares display text (a card title, a label). A wording change breaks the check
  in a package nobody edited, and the failure can sit unseen for days.
- **Fix:** keep a raw, machine-meaningful field next to the display text and compare that. Build the
  expected value from the product's own builder and the input from an independent fake. Never fix
  such a failure by editing the fake to match the new wording.

## Wall clock

- **Why:** the test uses day math, the time zone or the current hour. It passes at 14:00 and fails
  near midnight or in another zone.
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
