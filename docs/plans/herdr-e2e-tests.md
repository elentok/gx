# herdr-backed e2e tests

Recommended e2e scenarios for `testutil/herdrctl` + `e2e/`, derived from a survey of every
documented gx incident that traced back to a bug/quirk in `herdr` itself (not gx's own scheduling
logic). Full survey with ticket/commit citations is in the session that produced this doc — ask to
regenerate it if the citations are needed again; only the actionable list is kept here.

Ordered by production impact (highest first). Each entry names the herdr primitive under test and
the gx-side workaround it would guard.

**Verified against herdr 0.8.0** (see the `herdr-e2e-testing` wayfinder map, ticket 01): two
candidates below turned out to already be fixed upstream or not a live risk and are marked
accordingly — skip building tests for those. The rest are confirmed-current behavior or still
need the fake-agent harness to test at all; each is annotated below.

## Harness prerequisite

`testutil/herdrctl` currently wraps `pane run/send-keys/send-text/wait-output/read` only. Several
scenarios below need `herdr agent *` (start/prompt/wait/explain) too — extend the harness with
those before tackling anything below marked (agent API).

- [x] Extend `herdrctl.Workspace` with `AgentStart`, `AgentPrompt`, `AgentWait`, `AgentExplain`
      wrapping `herdr agent ...`, following the same JSON-envelope-vs-raw-text handling already
      worked out for `pane read`. Landed as thin wrappers delegating to the production
      `herdr.AgentStart`/`AgentPrompt`/`AgentWait`/`AgentExplain` (error-returning) functions,
      `t.Fatalf`-ing on error — no argv-building duplicated between the two layers.

## Fake agent script

Every scenario needs a controllable stand-in for Claude/Codex running inside the herdr pane —
not a real agent call. Build one small script/binary (shell or Go) that can be told, via
env var or arg, to: print a working spinner glyph + OSC title for N seconds, then go idle; or
stall silently; or simulate a long `/compact`-style pause. This is the `testutil/herdrfake` idea
applied to the *other* side of the pane — a fake `claude`, not a fake `herdr`.

- [x] Write `testutil/agentfake` (or similar): a small controllable program that emits the
      terminal-title/spinner-glyph patterns herdr's detection manifest looks for, with knobs for
      timing (immediate idle, slow-working, stall, fake-compact). Landed as
      `testutil/agentfake` (`--mode=idle|slow-working|compact|stall`, `--duration=`), built as the
      literal binary name `claude` via `testutil/agentfake/cmd/agentfake`, so `herdr agent start
      --kind claude` execs it. To win PATH resolution over a real `claude` inside the pane's
      **fish** shell (whose config re-derives PATH from its own `fish_add_path` config on
      startup, reordering an inherited `--env PATH=` ahead of a real one), the harness runs
      `fish_add_path --prepend --move <dir>` inside the pane instead of relying on
      `workspace create --env` alone — see `herdrctl.Workspace.PrependPath`. This is
      fish-specific; a non-fish CI shell will need an equivalent, flagged as a follow-up for
      ticket 03 (macOS CI job).

## Scenarios

- [x] **Compaction false-idle** — landed as
      `e2e/compaction_false_idle_test.go`
      (`TestCompactionFalseIdle_AgentWaitDoesNotSettleBeforeFakeCompactionEnds`). Drives
      `testutil/agentfake --mode=compact` through a simulated 6s `/compact` pause and asserts
      `herdr agent prompt --wait` doesn't return before the pause elapses, and reports the turn as
      `done` (a completed turn), not `idle` (never started) — `--until idle` alone never matches a
      real completed turn, only `--until done` (or the default idle/done/blocked set) does; this
      surprised the first draft of the test, which hung using `--until idle` explicitly. Verified
      hands-on that the assertion is load-bearing (temporarily forcing it to require an impossibly
      long elapsed time made it fail as expected) and that the test passes against the real
      installed herdr 0.8.0.
- [ ] **Idle-while-working** — needs fake-agent harness (unverified in 0.8.0 beyond one narrow
      v0.6.7 fix). Fake agent prints continuous visible tool-use output with a rotating spinner
      glyph; assert `AgentGet`/`agent wait` never reports `idle` while the fake agent is still
      emitting output.
- [x] ~~**`agent wait` on already-finished pane doesn't hang**~~ — **confirmed fixed since herdr
      v0.6.3**, well before the 0.8.0 baseline. Verified hands-on: `agent wait --until <state>`
      against a pane already in that state returns in ~10ms. Skip — not worth a regression test.
- [ ] **`agent_prompt_stalled` error shape** — confirmed current behavior (since v0.7.5), testable
      today with no fake-agent harness needed. Drive a prompt against a real/fake agent configured
      to produce zero state-change for >5s; assert herdr returns the structured
      `{"error":{"code":"agent_prompt_stalled"}}` envelope (not a generic timeout string) — locks
      in the exact shape `ralphloop/waitforfinish.go`'s `isPollTimeout` depends on.
- [x] ~~**ctrl-c interrupt actually interrupts**~~ — **not a live risk.** `ctrl-c` (hyphen) is
      genuinely rejected as `invalid_key`, but `ctrl+c` (plus) is herdr's documented canonical form
      and gx's codebase only ever sends the plus form. Skip.
- [ ] **32-char agent-name cap** — confirmed current behavior, testable today. Call `agent start`
      with a >32-char name; assert the specific `invalid_agent_name` error code (gx currently only
      special-cases `agent_name_taken` — this test would justify closing that gap).
- [ ] **`agent_name_taken` same-cwd reattach** — confirmed current behavior, testable today. Start
      an agent under a name, then start again under the same name from the same cwd while the
      first is still live; assert the collision behavior/error shape gx's `AgentNameTakenError`
      handling expects.
- [ ] **Concurrent `agent start` pane-allocator race** — inconclusive: 5 concurrent launches on 5
      distinct panes showed no race hands-on, but same-pane/higher-concurrency contention wasn't
      tried. Lower priority than the others; revisit whether this is worth a dedicated scenario.
- [ ] **Tab close reliability** — not reproduced as broken in 0.8.0 (5/5 clean create-close-list
      loops; v0.8.0 also added auto-closing the workspace on last-tab-close, verified hands-on).
      Reframe as a "pin the contract" test rather than a bug repro — testable today.
- [ ] **Worktree/workspace/tab topology** — confirmed current behavior, testable today.
      `herdr worktree create --workspace` + `--cwd` together is still rejected by the CLI parser
      as mutually exclusive; pin this down so a future herdr upgrade that changes the contract
      fails a test instead of silently reintroducing the old topology bug.

## Explicitly out of scope for herdr e2es

Bugs already confirmed as gx-only (ticket dependency-graph deadlocks, `tickets add --parent`
frontmatter, Telegram markdown escaping, `featureMu` serialization, `pendingEpics` restart loss,
etc.) — these belong in gx's regular unit/integration tests, not this harness.

## CI

Separate job from the main `go test ./...` step:
- `brew install herdr` (macOS runner only, unless/until a Linux binary is confirmed available),
  start `herdr server &`, set `HERDR_ENV=1`, run `go test ./e2e/...`.
- Keep it isolated so herdr-install hiccups or real-timing flakes (esp. the pane-allocator race
  scenario) don't fail the main suite.
