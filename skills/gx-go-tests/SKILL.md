---
name: gx-go-tests
description: Rules for writing Go tests that stay fast and stay green. Covers readiness waits, state files, process limits, cleanups, fault injection and wire fields. Use when writing, changing or reviewing a Go test, or when a Go test is flaky.
---

# Go tests

These rules add the Go forms to the common rules in
[gx-test-guidelines](../gx-test-guidelines/SKILL.md). Read those first. A rule here names the Go
tool or the Go trap, and the general point stays in the guidelines.

## Readiness signal

- **Why:** a test sends input when a socket listens or a pid file exists, before the program reads
  it. A pseudo-terminal echoes input before the program reads it, so a screen check that passes on
  the echo proves nothing.
- **Fix:** poll the surface the test is about (an endpoint, a status event), after the program has
  acknowledged the input. A start timeout is never a readiness signal.

## State files

- **Why:** a truncate followed by a write leaves a window in which a reader sees an empty file. A loser
  that reads a pid or lock file in that window gets an empty value and a wrong diagnosis.
- **Fix:** write a state file in one write, or write a temp file and `os.Rename` it. A reader that
  can still see an empty file retries the read, and prints the path.

## Test state

- **Why:** a shared constant path or name makes parallel tests and reruns see each other's state.
- **Fix:** take every path from `t.TempDir()` and every unique name from `t.Name()`.

## Socket paths

- **Why:** a Unix socket path is capped near 100 bytes. A long temp dir breaks the bind with
  `bind: invalid argument`.
- **Fix:** a test that binds a socket uses a short temp dir.

## Clocks

- **Why:** `time.Now()` in the code under test makes the test depend on wall time.
- **Fix:** inject a clock and move it in the test.

## Real machine state

- **Why:** a test that reads the real `HOME`, sibling worktrees, real branches or a live server's
  state passes on one machine and fails on another.
- **Fix:** build the home, the repo and the state inside the test's temp dir, and pass them in.

## Fakes on idle paths

- **Why:** a fake agent idles without a report. New idle-path or finish-path behaviour, such as a
  stall nudge, makes every fake hang until the test times out.
- **Fix:** give the new behaviour an off switch, and keep it off by default in the fake.

## Quiet windows

- **Why:** the test waits for "no change for 500 ms" and treats it as the end of async work. A busy
  machine pauses longer than the window.
- **Fix:** identify the work by its id and poll for that, with a timeout.

## Nested waits

- **Why:** a start timeout, a slot wait and a poll add up to more than the limit set by `-timeout`.
  The test binary is killed first, and the diagnostic never prints.
- **Fix:** add the inner waits up. Keep the sum under the outer limit and leave room to print the
  state.

## Child stderr

- **Why:** a child process writes stderr to the parent's stderr. The text appears under whichever
  test is running, not under the test that started the child.
- **Fix:** capture stderr per child in a buffer. Attach it to the failure message of the test that
  owns the child.

## Fork errors

- **Why:** a failed `fork/exec` reports `resource temporarily unavailable`, which looks like a
  plain exit 1. A mass failure then reads as many causes.
- **Fix:** classify the error where it happens, for example `process limit hit (EAGAIN)`, with
  `errors.Is(err, syscall.EAGAIN)`. One cause then reads as one cause, and an unexplained exit 1
  stands out.

## Counters

- **Why:** the success signal is a counter, but the fake moves it on startup. The test passes by
  accident, then fails when the baseline moves.
- **Fix:** make the fake do the work, and assert the event. Look for an existing seam in the code
  under test before you add a wait.

## Baselines

- **Why:** a baseline taken before the first read and one taken after it differ by the thing the
  wait counts. A launch that moves the baseline at startup shows the same failure.
- **Fix:** take the baseline at a defined point after startup, and wait on a change from it. Assert
  the baseline value in the test.

## Waits on the wrong state

- **Why:** the test polls the runner's view of the state. The code under test reads another store,
  such as a run registry, so the two views disagree for a moment.
- **Fix:** wait on the state the code under test reads. Check the package's own test seams first.

## Cleanups

- **Why:** a cleanup that calls `panic` for a missing fixture ends the whole test binary. Every test
  still running fails with it.
- **Fix:** register cleanups with `t.Cleanup`. A cleanup tolerates "already gone" (`os.IsNotExist`,
  a process that has exited) and reports other errors with `t.Errorf`.

## Short timers

- **Why:** a state lasts about as long as a scheduling delay, for example a 1 s countdown. Polling
  cannot see it.
- **Fix:** hold the timer in the test with an injected clock or a channel, or observe an event.

## Fault injection

- **Why:** a rare failure (1 in 100) cannot be shown by repeats. A fix that passes 80 runs against a
  base that also passes 80 runs proves nothing.
- **Fix:** inject the suspected delay at the seam (a `time.Sleep` in a test hook before the
  baseline, a stall before the read, a spin poll in place of a coarse one). The flake becomes 2 of
  2, and the fix 0 of 2. Use `-count` under load to find the shape of the failure. Run the base and
  the fix at the same `-count` and under the same load. Say so in the report when the failure rate
  was too low to compare.

## Failure signatures

- **Why:** reading code before the logs sends the search in the wrong direction. In one case 17 of
  18 logs said `started no turn`, and none said `resource temporarily unavailable`. That ruled out
  the process limit in one script.
- **Fix:** run `rg -c` for each message across the log files, and count each signature. Read code for
  the largest group.

## Timeouts for missed events

- **Why:** a test that fails after a pause waits for a state that cannot happen any more, for
  example an event handled by a `sync.Once` that already ran.
- **Fix:** find the lost event. Never raise the timeout, and never rerun the test in the hope that
  it passes.

## Process limits

- **Why:** `ulimit -u` on macOS counts every process of the user, across all sessions, and the limit
  is fixed when each binary starts. A cap of "count plus N" holds only for that moment. Size N from
  the measured peak of the whole run, not from one package. A package with no process-heavy work
  that gets a fork error is a victim of another spike.
- **Fix:** sample the process count once a second during any full run you tune a cap for. Read the
  count at the failing second before you read the failing test. Run a process-limit wrapper around
  the test binaries. Never retry, and never lengthen a timeout, for `resource temporarily
  unavailable`, because the error repeats once the count crosses the limit.

## Burst tests

- **Why:** a test that starts many processes charges them to every concurrent test binary and every
  other session, even when it raises its own limit.
- **Fix:** run a burst test in its own step, or behind a lock shared by the whole user. A helper
  that raises `RLIMIT_NPROC` for itself carries a comment that sibling test binaries do not get the
  raise.

## Wire fields

- **Why:** a change to a field that other code reads breaks a reader in another package. One commit
  changed a card title and updated three packages' tests, and a fourth package that compared the
  title to the screen text failed unseen on the main branch.
- **Fix:** find every reader before you change a wire or card field, with gopls references or
  `rg` across all packages. Run the tests of the dependent packages before the commit, not only the
  package you edited. Compare a raw field, never the display text. Build the expected value from the
  product's builder and the input from an independent fake.

## Wake-ups that replace a poll

- **Why:** an event-driven loop replaced a poll. A setting change that took effect at the next tick
  now has no wake-up, and a lost file-watcher event is never recovered.
- **Fix:** list every setting and state change that used to take effect at the next tick, and give
  each one its own wake-up. Keep a fallback poll short enough to cover a lost event within the
  test's wait, or make its interval configurable for tests. Test the lost-event path.

## Debounce keys

- **Why:** a loop that restarts its quiet timer on every raw event starves under a steady stream
  of events.
- **Fix:** debounce on the change that matters, such as the set of dirty files. In the test, state
  the wake-up the loop depends on, and trigger it with a test hook. Never depend on a platform
  file watcher for a ten-second wait in a unit test.
