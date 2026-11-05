# AI Rules

## Reading files

The harness elides the middle of a long `Read` ("... (N lines omitted)"), and code you did not see
gets edited blind. Read a whole file only when it is under 150 lines. Otherwise run `rg -n` first,
then `Read` with `offset` and `limit` around the match. Use line numbers or symbols the ticket
already names.

## Agent skills

### Issue tracker

Issues live as local markdown files outside the repo. There is no `.scratch/` directory and no
`gx tickets list` command. Use these instead:

- `gx tickets root` prints the store path.
- `gx tickets epics` lists the epics.
- `gx tickets show <addr>` shows a ticket or an epic.

See `~/.dotfiles/core/ai/skills/setup-elentok-skills/issue-tracker-local.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Adding a Migration step

- A change to the ticket or epic schema (field, enum value, section name) needs a new named step
  in `migrations/steps.go`. Add one; never rename or reuse a released step name.
- Then update `migrations/testdata/schema.snapshot`:
  `go test ./migrations -run TestSchemaSnapshot -update-schema-snapshot`. The guard test fails
  until you do.

## Testing

- Test the packages you touched plus their direct importers (find importers with `findReferences`,
  see [gx-gopls](../skills/gx-gopls/SKILL.md)). Don't run `go test ./...` for a local change.
- ralph-loop's land step runs no tests: it only cherry-picks commits onto the feature branch. The
  full suite runs in CI (`.github/workflows/ci.yml`, on pushes and PRs to `main`): `make test`
  (`scripts/test.sh`: race, other Go, server and web unit tests side by side), `make web-e2e`, and
  `go test ./e2e/...`. `make test` shares one CPU budget between its steps, half the cores by
  default; set `GX_TEST_CPUS=<n>` to change it. Every `gx brief go-test` run caps itself the same
  way (GOMAXPROCS at two fifths of that budget) unless the environment already sets GOMAXPROCS.
- Test-writing rules shared with other projects are in `skills/gx-test-guidelines.md`. The rules
  below are gx-only.
- **Default to no new Playwright flow.** Cover render and component logic with vitest and server
  rules with Go tests. A flow needs a real server round trip, browser layout, or a multi-page
  journey, and extends an existing flow when it can. `playwrightguard` fails above `maxFlows`. Full
  rules: `skills/gx-playwright-e2es`.
- Ticket agents run changed-file `gx check` and never `gx check --full`. The epic's verify ticket,
  the proof and the person run `gx check --full`; the verify ticket runs it and the soak in the
  background and polls.
- Never run `go clean -cache` directly, use `gx brief go-clean-cache`.
- Once a targeted run passes, stop. Don't rerun the full suite to double-check.
- Run every `go test` with `-exec "$PWD/scripts/proc-cap"` (the Makefile does). It caps your user's
  process count at the current count + 200, so a test binary that forks itself (a `TestMain` missing
  `servertest.RunSessionProcesses()` or `herdrfake.RunHelperProcess()`) fails fast instead of
  crashing the laptop. A package using `servertest` needs both calls.
- Verify with `gx check` in the foreground, unpiped. Iterate with `gx brief`. Never pipe, `tail` or
  background either.
- `server/` is the slowest package. Run it only when you touched `server/` or
  `server/servertest/`, and scope it with `-run` while iterating. A plain `go test ./server` runs
  the tests in parallel and takes about 2 minutes.
- Tests must never read the developer's real Claude config (their `editorMode: vim` changes
  outcomes). Pass `HOME`, `CLAUDE_CONFIG_DIR` and the rest through config (`claudeenv.In(home)`
  returns the env entries for `server.Config.Env` or `exec.Cmd.Env`, with a vim-mode `~/.claude.json`
  as a canary: code that reads `HOME` instead of `CLAUDE_CONFIG_DIR` fails loudly). Never use
  `t.Setenv` or `os.Setenv` for these. `envguard/envguard_test.go` bans new process-env
  reads of `HOME`, `PATH`, `XDG_*`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME`, `os.UserHomeDir()` and
  `exec.Command` without a `.Env =`; its allowlist lists what is left.
- Run web tests through `make web-test`. gx installs dependencies when it creates an iteration
  worktree, so web tests work in a fresh worktree. `make web-test` still installs first, at the cost
  of a slower run.
- `gx brief` tools: `go-test`, `go-build`, `go-vet`, `vitest`, `tsc`, `playwright`, `conflicts`.
  Any other name exits 2 with `unknown tool`. Use `gx brief go-build ./...` and
  `gx brief go-vet ./cmd`, never `go build ./... | tail`.
- Quiet web runs, from anywhere in the repo (always pass `-C webui`; the shell cwd persists between
  calls, so never `cd webui`):
  - `gx brief -C webui vitest src/epic`
  - `gx brief -C webui tsc`
  - e2e flows: `gx brief -C webui playwright disk.e2e.ts 76 77`. Bare flow numbers become one `-g`
    pattern. Never write `-g "^76\."`: Playwright matches `-g` against `[project] › file › title`,
    so a `^` anchor finds no tests. It adds the config and the line reporter, and runs
    `make web-build` only when `webui/dist/index.html` is missing.
- Unix socket paths are capped near 100 bytes, and `t.TempDir()` can exceed that. Holder-socket
  tests (anything that binds `holder.sock`) need a short `TMPDIR` or a short state root; use
  `shortTempDir` in `nativerunner` and `nativerunner/holder` tests. The failure is `bind: invalid argument`.
- Tests never read live machine state, such as sibling worktrees, real branches or the live
  server's state. Use `t.TempDir()` and `testutil.TempRepo`.

## Adding a server route

A new route needs all of these. A missing one fails a test, not the build:

- A subsystem moved out of root lists its routes in `Routes() []wire.Route` (`server/wire/route.go`:
  pattern, socket handler, web mode, web variant) and `subsystemRoutes` in `server/routes.go` gathers
  it. This replaces the three steps below for that subsystem. Root still has `routeTable` and
  `webWrites` until the root cleanup ticket removes them.
- `routeTable` in `server/server.go`.
- A CLI verb: `routeVerbs` in `cmd/server_parity_test.go` plus the command itself
  (`TestRouteTableHasCLIVerbs`).
- The web mux in `server/web.go` when the browser calls it (`webWrites` for writes).
- The allowlist in `TestWeb_EverythingElseIs404` (`server/web_test.go`).

Server tests: build fixtures with `servertest.WriteTicketWith` (`server/servertest/store.go`)
instead of hand-written helpers. A web test that needs a project repo also needs
`SetProjectRepoNoAutoMerge`; `newFFFixture` does not provide one.

`servertest` fake agents idle without sending a report. New idle-path or finish-path behaviour (a
stall nudge, say) makes every fake agent hang until the test times out. Give it an off switch that
is off by default in `servertest`.

## Pauses

Pause only when gx cannot recover on its own. Every pause needs a web button that ends it, never
a gx CLI step. A new pause kind needs a row in the pause-to-verb table. See
[ADR 0042](../docs/adr/0042-few-pauses-all-recoverable-from-the-web.md).

## Package map

There is no `internal/` directory. Top-level packages live at the repo root:

- `cmd/` — CLI commands (`gx tickets`, `gx agent`, ...)
- `server/` — the gx server; `server/servertest/` holds its test helpers; `server/wire/` holds the
  wire types, consts and pure methods (no behaviour, no import of `server`); `server/broker/` holds
  the SSE event broker and throttle (imports `wire` only); `server/webtls/` holds web TLS: the
  root and leaf certificates, SNI and source-IP listeners, host allowlist and the root downloads;
  `server/webauth/` holds web login: the login store (password, TOTP, devices, setup code, rate
  limits), the cookie gate (`Gate`) and the login routes (imports `webtls` and `wire`; the root
  implements `Host`; `cmd` and `testutil/gxfixture` import it too);
  `server/disk/` holds disk watching: free-space status, the watch loop, the hold, the clean-now
  flow and the temp sweep (the root sets its `HealthSink`; `cmd` imports it for `FileFreeSpace`);
  `server/limits/` holds usage limits: the Claude limit store, limit history, per-harness usage
  samples with their gate and override, the usage summary, the burn stats and the token poll (the
  root passes `SpendDays`, from the budget ledger);
  `server/budget/` holds the daily spend budget: the ledger, the soft and hard latches, override and
  increase, the poll, the budget kill, the hard-limit grace countdown, the notices and the
  budget-changed events (imports `limits`; the root implements `Host.PauseTicket`; the extra-usage
  check and the long-ticket check stay in the root);
  `server/health/` holds the system health report: the CPU, memory and disk meters, the process
  and owner probes, the CPU spike watch and the start incident note (the root implements `Sessions`
  for the session meta read; `cmd` imports it for `CPUSpikeDir`, `CPUWatchConfig`,
  `StartIncidentPath`, `FileSystemHealth` and `FileProcesses`);
  `server/stats/` holds the stats read model behind GET /v1/stats: throughput, friction, cost and
  grilling (imports `limits`; the root implements `MapEpics`; `WebShare` lives here for the epic
  detail);
  `server/sessions/chat/` holds the session chat: the `Chat` (reads, failure banner, command lists,
  permission dialogs, screens), its `Poll`, `Routes()`, the transcript path and file caches
  (`TranscriptPaths`, `FileCache`, `AuthFailures`), the parsed-chat `Cache`, the Grilling round and
  run-end parsers, `DesignContinueAction` and `AcceptsKeys`. It reaches the sessions through the
  `Source` interface (the root implements it as `chatSource`); its tests use a fake `Source`;
  `server/sessions/` holds the interactive sessions: the `Manager` (create, stop, remove, focus,
  banner, watches, state) and the `Resumer` (resume, restore, handoff, link, step, fix agent), with
  `Routes()`. It imports `chat` (never the reverse); the root implements its `Host`, `Prompter`
  and back-call interfaces and calls `Watches.Close()` at the `session-watches-close` shutdown step;
  `server/ticketwrite/` holds single-ticket writes (so far the leaf types: `RecoveryHold` and
  `RecoveryState`, `PauseFold`, `NudgeLimiter`, `PauseChatDetail`, `RetroSubmit`; the `Server`
  methods follow in later tickets);
  `server/recovery/` holds failure recovery: planning, the rule/investigate/propose/escalate
  actions, the recovery verbs, the investigate flow, follow-ups, the bad round block filing and
  `LaunchSkill`/`LaunchPrompt`, with `Routes()` for `tickets/approve`. It imports `ticketwrite`; the
  root implements its `Host` (`LiveIteration`) and passes it the pause hold. Import it as
  `srvrecovery` (the name clashes with the top-level `recovery`);
  `server/settings/` holds the settings holder, config reload, config status and config file watch
  (imports `wire` only; the root registers its appliers); `server/runs/` holds the run registry
  (`Run`, `TrackedRun`, `Registry`); `server/index/` holds
  the ticket index, the store watch and the poll/watch loop, and the project, lock and history
  reads (imports `wire` and `broker`; the root registers the `OnRescan` steps and the migration gate);
  `server/projects/` holds the project `Registry`, the start-up `Migrator` and `Checks`, with their
  routes, the scratch and outbox projects and `AgentEnv` (imports `index`, `runs`, `settings` and
  `wire`; `cmd` imports it too); `server/queue/` holds the queue and its pause gate, the Queue agent
  rows, AFK and vi mode, and the one-offs (submit, sweep, `IsOneOffEpic`), with their routes
  (imports `index`, `projects`, `runs`, `broker` and `wire`; the root implements `Host`; `cmd`
  imports it too); `server/harness/` holds the harness and runner choice (resolution order, Default
  harness, labels, missing-program checks, runner selection) and the `/v1/settings/agents` routes
  (imports `index`, `projects`, `queue`, `runs`, `settings` and `wire`; the root implements `Host`
  and imports it as `harnesschoice`, since the top-level `harness/` clashes);
  `server/merge/` holds epic merging and landing: the `Merger` (epic merge, merge recovery, the
  late-finish and hand-merged scans), the `Landing` (dirty-trunk merge, stash-merge, trunk conflicts) and
  the `Reconciler` (parent-defect and late-finish scans the root registers as `OnRescan` steps),
  with `Routes()` for `epics/merge` and `epics/stash-merge`. The root implements its `Deps`; the
  merge guard's `closeAndWait` stays with the root's shutdown;
  `server/epics/` holds the epic read models (ticket detail, epic list and detail, epic and design
  cards, map progress, recently finished) and the epic-state predicates, with `Routes()` for
  `tickets/detail`, `epics` and `epics/detail`. The root implements its `Deps`;
  `server/designs/` holds the Design sessions (open, worktree, delete, and the cleanup the Manager
  and the rescan call back), with `Routes()` for `designs/open`, `designs/delete` and
  `designs/worktree`;
  `server/sessions/prompt/` holds the session prompts: the auto-advance countdown, turn settle,
  draft clearing, the start clock, prompt send and keys, the first prompt, and the actions and
  actionables, with `Routes()` for all six prompt routes (`sessions/auto-advance` and `/cancel`
  among them). It has no `Host`; its tests share `servertest/promptsession.go`;
  `server/ticketops/` holds the ticket operations: ask and answer, the agent verbs, cancel, unmute,
  retry, recommend and the repair verbs (resume, start over, merge, verify), with `Routes()`. It
  reaches the run loop in the root through the `Launch` interface (the root implements it; the
  tests use `launchFake`)
  `server/sched/` holds the scheduling top of the lifecycle; so far the `HerdrWatch` (herdr
  availability, hold reason, banner, retry loop), the `Reclaimer` (startup reclaim of saved
  runs, `ResolveRun`, `Resume`) and the `Launcher` (`ClaimAndLaunch`, launch, `finishRun`/`settleRun`,
  `NotifyDone`) and the `Scheduler` (the claim loop and its caps, the base and merge-target rules, gate watchdog, deadlock check, finished-claim watchdog, live iterations) and the `Explainer` (verdicts, pending rows, claim holds, check-lock label), with `Routes()` for `iterations`, `queue` and `tickets/explain`. The root implements `HerdrHost` and `ReclaimHost`; the `Scheduler` implements `LaunchHost` and the root's `Launch` hook (`LiveIteration`); the `Explainer` reads the `Scheduler` directly and the claim pass asks it for holds
- `ralphloop/` — ralph-loop scheduler, landing and verification
- `tickets/`, `tickets/schema/` — local markdown ticket tracker; `tickets/tickettest` — its shared test helpers; `tickets/ticketlog` — pause-round readers (question, rounds, answered, start-over, pause detail); `tickets/ticketmove` — ticket move, id allocation and ref rewriting; `tickets/ticketmigrate` — migrations and backfills (Migrate*, Backfill*, pause migration, store walk); `tickets/ticketgit` — trunk resolution, repo opening, epic spec path and iteration-branch parsing (imports `tickets`, never the reverse); `tickets/ticketspec` — publish-side code (lint-claims, manifest, verify plan, publish checks, estimates)
- `agentrunner/` — the agent-runner interface; `nativerunner/` — herdr-free runners; `nativerunner/holder` — the pty holder process, its socket protocol and client (`nativerunner` imports it, never the reverse)
- `harness/` — what runs inside an interactive agent session
- `herdr/` — herdr CLI/socket client
- `runner/` — runs a child command with inherited stdio
- `git/` — git operations
  - `git/internal/run` — runs git children (`Run`, `RunWith`, `RunRaw`, `RunError`) and the `Exec`/`AllowFail` helpers; `git` re-exports the public names
  - `git/stage` — staging, unstaging, patch apply, delta/commit diffs, `StagedFiles`; imports `git`, never the reverse
  - `git/history` — fast-forward merge (`MergeBranchInto`, `MergeFastForward`), `TreeID`, cherry-pick, trailers, reword and amend; imports `git`, never the reverse
  - `git/pr` — GitHub pull requests via `gh` (list open/closed, comments, facets, PR URLs); imports `git`, never the reverse
- `ui/...` — TUI (`ui/app` is the root model; one package per tab or component)
- `e2e/` — end-to-end tests (slow, CI only)
- `testutil/` — fakes and helpers: `testutil/agentfake` (fake Claude, shared core),
  `testutil/agentfake/codex` (fake Codex, imports `agentfake`), `testutil/herdrfake`,
  `testutil/runnerfake`, `testutil/herdrctl`, `testutil/teatestv2`, `testutil/claudeenv`
- `testutil/gxfixture` — e2e fixture program: `gxfixture <ready|mixed|finishing|edge>` starts an
  isolated real `gx server` (pty runner, `fakeclaude` as `claude`) and prints JSON with `web_url`,
  socket, state dir, hold file and `log_file` (the server's stdout+stderr, which holds the cleanup
  command's output tail); SIGTERM stops it. Per-ticket worker behaviour
  (finish/ask/fail) lives in `.agentfake-worker.json`, see `scenarios.go`. The `cache-gate` and
  `cache-gate-custom` scenarios (`cachegate.go`) output `gate_dir`: a fake `go` and `ps` in
  `bin/` (put it first on PATH), `go-hold` to hold a fake build, and `go-clean.log`.

Run `go list ./...` for the full list.

## File size

Keep a Go source file under about 1000 lines. When one grows past that, move a cohesive group of
declarations into a new file in the same package (for example `server/sessionstate.go` out of
`server/sessions.go`). Moves only: don't change behaviour in the same commit.

## Core principle: gx must be visually beautiful

Beauty is a requirement, not polish. A feature that works but looks cluttered is not done.

- Before adding UI, check how it sits next to what is already on screen (headers, footer, panels).
- Prefer fewer visual elements: color and weight over boxes, pills and extra glyphs.
- Show only what the person needs right now. Move details (pids, versions, debug info) to the help
  dialog or a modal.
- When unsure, render the screen (e.g. in tmux) and look at it before calling the work done.

## Web UI

Before adding or changing a page or component in `webui/`, read `webui/README.md`. Two rules apply
to every page:

- A page that shows server data repaints from a `sessionStorage` copy after a reload
  (`webui/src/pageCache.ts`).
- A page without data yet shows the centered `Spinner`, never skeleton placeholders.

Testing a click on a Chakra menu item needs `waitFor`. See "Testing a Chakra menu" in
`webui/README.md`.

Web checks:

- Type-check with `gx brief tsc`, not a hand-built `pnpm exec tsc ... | tail`.
- The web UI does not use prettier. Never run it: it reformats unrelated lines.
- Regenerate `webui/src/generated/server.ts` and `wire.ts` with `make web-types`. Wire types live in
  `server/wire` and generate `wire.ts`; `server.ts` re-exports it, so web code keeps importing
  `generated/server`. tygo cannot resolve a constant or alias from another package (it emits
  `any`), so give wire types literal values and map cross-package types in `tygo.yaml`
  `type_mappings` (a wire type used by a `server` type needs an `import type` in the `server`
  entry's `frontmatter` too). Check `git diff` for `any`. During the wire split,
  `scripts/webtypes-diff.sh <base-sha>` compares the output with the base.
- Run e2e with `make web-e2e`. It rebuilds the web bundle first. `pnpm e2e` does not rebuild.

## UI

- See ./design-system.md for UI work
- Don't show any keymaps on the statusbar, only "? help"
- Whenever possible prefer the LSP tool over grepping, see [gx-gopls](../skills/gx-gopls/SKILL.md)
- Run Go tests per package (`go test ./<pkg>/...`), never `go test ./...`: the whole repo is slow
  and loud, and has caused context blowups. Never add `-v`.
- Transient feedback uses `ui/notify` (not `m.statusMsg`): emit `notify.Info/Success/Warning/Error/Progress()` as `tea.Cmd`
- Avoid creating `New()` functions and prefer more descriptive functions, e.g.
  `log.NewModel()`, `keys.NewManager()`, ...
