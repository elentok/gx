# Reducing test failures in gx

Research date: 2026-11-16. This report uses official Go and Playwright documentation and the current repository configuration. It identifies audit targets, not proven causes. No historical failure dataset was analyzed.

## Priorities

1. Wait for observable completion instead of elapsed time. Audit negative assertions that can pass before work starts.
2. Keep resources isolated until shutdown completes. Cancel and join goroutines and child processes before removing their files.
3. Use controlled time for in-process scheduling tests. Keep real sockets and subprocess integration tests outside `synctest`.
4. Preserve zero retries and first-failure artifacts in required browser CI.
5. Collect first-attempt failure records before changing worker counts or timeout defaults.

## Existing protections

- [Playwright config](../../webui/e2e/playwright.config.ts) has three workers, full parallelism, zero retries, and traces retained on failure. Timeouts are 60 seconds per test, 10 seconds per assertion, and 20 seconds per action. Service workers are blocked except where flows opt in.
- [CI](../../.github/workflows/ci.yml) installs Chromium dependencies, runs `make web-e2e`, and uploads `webui/test-results` on failure.
- [The test runner](../../scripts/test.sh) budgets CPU across concurrent race, Go, server, and web steps. The race step uses `GOMAXPROCS=1`. Browser workers are outside that budget when a full local check includes e2e.
- [Repository rules](../../.ai/index.md) require process caps, isolated repositories, injected child environments, and short Unix socket paths. [Test guidelines](../../skills/gx-test-guidelines.md) already cover shared state, timer counts, wall clocks, stale assertions, and missing stderr. Apply those rules to known failures first.

## Go tests

An elapsed sleep does not establish that another goroutine completed. Prefer a channel, a joined goroutine, or an explicit completion callback. For integration tests, poll the exact expected state with a bounded deadline and print the last observed state on failure. Avoid exact event counts in short wall-clock windows. The Go team's [testing time article](https://go.dev/blog/testing-time) explains why real-time asynchronous tests become slow or flaky under load.

Use `testing/synctest` for self-contained timer, cancellation, debounce, and retry logic. It provides an isolated bubble with a fake clock. Time advances when its goroutines are durably blocked, and `synctest.Wait` supplies a synchronization point. gx's Go 1.27.3 declaration supports the package introduced in Go 1.25. Its restrictions matter: avoid external goroutines, real network operations, external processes, and leaked background work. It does not replace real fixture-server tests. See [synctest](https://pkg.go.dev/testing/synctest).

Register cleanup when a resource starts. Stop and join background work before removing directories. Include child exit status and stderr on failure. Go's `TempDir` supplies unique directories, and `Cleanup` runs after subtests in reverse registration order. `Setenv` affects the process and cannot run in parallel tests or under parallel ancestors. For gx, keep environment values in fixture or command config as required by repository rules. Directory cleanup alone cannot stop children. See [testing](https://pkg.go.dev/testing).

The race detector finds races only in executed code. A clean run cannot prove unexecuted paths safe. Its runtime and memory overhead can also expose resource timeouts. A timeout with `-race` alone is not proof of a data race. Inspect the report and resource evidence. See the [race detector guide](https://go.dev/doc/articles/race_detector).

Use repetition to answer a specific question. `-count=1` disables test-result caching. `-count=N` repeats tests. `-shuffle` changes order and reports a seed for replay. `-parallel` limits tests using `t.Parallel`; `-p` limits package-level concurrency. A shuffle seed reproduces order, not every goroutine schedule. See [Go testing flags](https://pkg.go.dev/cmd/go#hdr-Testing_flags).

From the repository root, replace the package and test placeholders:

```sh
# Reproduce without a cached success.
gx brief go-test -exec "$PWD/scripts/proc-cap" ./PACKAGE -run 'TestName$' -count=1

# Only for an assigned flake investigation, use a fixed repeat budget.
gx brief go-test -exec "$PWD/scripts/proc-cap" ./PACKAGE -run 'TestName$' -count=10 -shuffle=on

# Replay the recorded seed. Use -race for shared-memory investigations.
gx brief go-test -exec "$PWD/scripts/proc-cap" ./PACKAGE -run 'TestName$' -count=1 -shuffle=12345 -race
```

Never run `go test ./...` locally. Scope server iterations with `-run`. Stop after required targeted checks pass. A dedicated reliability task may define a separate bounded validation sample.

## Playwright flows

Playwright checks whether it can click an element, including visibility, stability, enabled state, and whether the element receives events. These checks do not establish that gx processed the resulting request or SSE event. Follow the action with an assertion on its result. Avoid bypassing checks with `force` unless the interaction requires it. See [auto-waiting](https://playwright.dev/docs/actionability).

Prefer role and accessible-name locators, independent tests, and controlled external dependencies. These reduce dependence on CSS structure and shared state. See [best practices](https://playwright.dev/docs/best-practices).

Use `toHaveText`, `toHaveCount`, and `toHaveCSS` instead of reading once with `evaluate` and comparing a value. If a one-time read remains necessary, first assert readiness on the same locator. Playwright distinguishes retrying web assertions from generic assertions and provides `expect.poll` for asynchronous values. See [assertions](https://playwright.dev/docs/test-assertions).

A negative polling assertion can pass before work begins. For example, checking that a queue lacks a ticket can succeed before the server enqueues it. This follows from assertions stopping when their condition passes; it is an audit target, not a confirmed gx defect. Wait for a positive completion barrier before asserting absence. If the contract promises absence throughout an interval, test a bounded interval with controlled time or explicit event history. One successful absence read proves only that instant.

Keep `retries: 0` in required CI. Playwright calls a test flaky when it fails and then passes on retry. It discards the failed worker and starts another, so a retry also changes process state and reruns setup. A passing retry does not explain or repair the first failure. See [retries](https://playwright.dev/docs/test-retries).

If a separate diagnostic job later uses retries, preserve first-attempt artifacts and consider `failOnFlakyTests` after checking installed-version support. It fails a run when a test passes only after retry. Retry success must not become the acceptance criterion. See [TestConfig](https://playwright.dev/docs/api/class-testconfig#test-config-fail-on-flaky-tests).

Follow the repository's failure workflow:

1. Read the failing flow's `webui/test-results/.../error-context.md` and retained trace.
2. Check whether an intentional UI change made the expectation stale.
3. Reproduce the selected flow and confirm the reported test count.
4. For a failure outside the changed steps, rerun once. If it fails the same way, use the follow-up workflow and stop iterating. Do not escalate repeat counts or create a clean-worktree baseline.

```sh
# Selector examples, not identified failing flows.
gx brief -C webui playwright disk.e2e.ts 76 77 --list
gx brief -C webui playwright disk.e2e.ts 76 77
```

The wrapper supplies config and reporter. It builds only if the bundle is missing, so run `make web-build` first after UI source changes. Use `make web-e2e` when the full browser suite is required. Do not anchor `-g` to the numeric title's start; the full matched title includes file and project. These rules come from `.ai/index.md` and the gx Playwright skill.

## Resource experiments

Playwright recommends one CI worker for stability and allows more on capable machines. gx uses three deliberately. In a dedicated reliability task, compare one and three workers using the same commit, flow set, and runner class. Keep retries zero and record failures, durations, and resource saturation. Do not run this experiment for an unrelated flow failure during feature work. See [CI guidance](https://playwright.dev/docs/ci) and [CLI worker options](https://playwright.dev/docs/test-cli).

Fewer timeouts at one worker may indicate contention or shared state; that result alone cannot distinguish them. Do not raise every timeout or serialize everything without evidence. Preserve `retain-on-failure`; collecting traces only on first retry would not help with retries disabled.

## Failure records and temporary quarantine

These are proposed gx policies, not Go or Playwright requirements:

- Record commit, runner OS, tool versions, worker count, first failing test, failure signature, phase, and artifact links. Distinguish assertion mismatches, readiness timeouts, process crashes, and infrastructure interruptions.
- Measure first-attempt failed runs divided by completed runs for each test and the suite. Report pass-on-rerun separately. Exclude canceled runs and show sample counts. Track p50/p95 duration and time to repair.
- Rank repairs by recurrence and blocked developer time. Fix deterministic stale assertions as such.
- Quarantine only a named test with an issue, owner, expiry, and recorded lost coverage. Keep running it in a visible diagnostic lane. A skipped test must not count as a stability improvement.
- Restore it after a predeclared sample passes under the original runner and concurrency conditions. A clean sample reduces uncertainty; it does not prove a zero failure rate.

Start with failure records and the most frequent signature. This report cannot rank particular gx tests or estimate the current failure rate without run data.
