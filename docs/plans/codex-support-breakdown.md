# Draft breakdown: gx:codex-support-impl

Base commit: b97febaa (worktree codex-support-design). pty-as-default-impl is already built in this tree (ADR 0043, nativerunner/pty*.go, server/runner.go launchRunner), so the Where-to-look lists below were checked against real code. Lists built with gopls are stamped `verified at b97febaa`; the rest are `not verified` (rg only).

## Draft epic Summary

gx can now run Codex wherever it runs Claude. A Default harness setting, in Settings and the TUI, decides what every new agent starts with, and rows show which harness, tier and model each agent uses. Where Codex cannot do something, the person sees a clear notice instead.

## Implement tickets (54)

Format: N slug | blocked-by | est | stories | mock states
(Est already includes +10K per mock state.)

01 default-harness-config | - | 50K | 8,31 | start-error
- Delivers: `agents.default-harness` in config.json and project.json (whitelisted, on reload list). `auto`/`pty` need the default harness on PATH, not `claude`; `headless` still needs claude. Server start refuses with an error naming the program. One shared PATH check function for later tickets.
- Seams: config tests (config/project_overrides_test.go, config/parse_test.go; node-free Go); agentrunner/select_test.go; cmd server start test through `preflightAgentRunner` (cmd/server_preflight.go). Mock `start-error` is a cmd test, no scenario.

02 resolver-and-queue-defaults | 01 | 45K | 15,17,18,82,84
- Delivers: one resolver (frontmatter `agent:` > explicit queue item agent > earlier run > default) as a pure function with a table test; web/TUI/server enqueues store empty agent ("follows default"); stored `claude` items stay; `--agent` stays an override.
- Seams: server table test of resolver (new server/harnessresolve_test.go, Go); servertest queue add tests (server/queue_test.go); ui/tickets queue add test (TUI add stops sending claude).

03 resolver-at-claim-sites | 02 | 60K | 1,15,16,19,84
- Delivers: every site calls the resolver: claim, relaunch, herdr hold, usage gate, `iterationRunner`, `resolveAgent`, queue loop, verdicts, recovery prompt. "Ran" = a run recorded a session with transcript or rollout. A different harness than the earlier run starts from the Handoff. Running sessions keep theirs.
- Seams: servertest + runnerfake: retry after switch keeps Claude (gate + runner), never-run follows default, waiting ticket claim reads default at claim time (server/ticketwrites_test.go, server/queue_test.go, server/claimhold tests). Large file warning: server/runner.go is 987 lines.

04 one-offs-follow-default | 03 | 35K | 14,85
- Delivers: one-off submit, evolve retro, diagnose one-off write `agent:` only for an explicit choice; root auto-queue, code-review, verify and evolve tickets claim on the default.
- Seams: servertest for oneoff.go submit, retrostamp, epicmerge_root; cmd/server_oneoff test for `--agent`.

05 codex-launch-shape | 01 | 45K | 33,34,35
- Delivers: the Codex harness in `harness.Lookup` (Launch/Resume args): one shape (workspace-write, on-request, approve-for-me, network_access, writable roots, inline hooks, bypass flag, update off, paste burst off). Replaces `--add-dir`.
- Seams: table test per agent kind in harness/codex_test.go (new) and ralphloop/loop tests for `AgentArgs`; server/sessions launchArgs test.

06 codex-guard-and-prompt | 05 | 40K | 37
- Delivers: `gx agent guard-bash` reads the Codex payload (exit 2); `request_user_input` PreToolUse block; Codex system prompt via `developer_instructions` on fresh launch only, never on `codex resume`.
- Seams: cmd/agent_guard test with Codex JSON; harness/systemprompt_test.go; launch test (fresh vs resume).

07 fakecodex-core | - | 50K | 79
- Delivers: `fakecodex` on agentfake core: OSC 7501 + title, rollout under `CODEX_HOME` (session_meta, response_item, task_complete), version 0.166.0, fixed by a test. Registered via `Harness`.
- Seams: testutil/agentfake tests (Go); nativerunner pty test like nativerunner/fakeclaude_test.go.

08 fakecodex-hooks-rollout | 07,06 | 55K | 79
- Delivers: fake runs the installed hooks (SessionStart, PreToolUse, PermissionRequest, Stop) with Codex JSON on stdin; rollout extras (`token_count.rate_limits`, `compacted`, `task_complete.error`); startup dialogs (trust, hooks review, update, model nudge); worker script switches.
- Seams: agentfake tests; production `codexsession` reads the fake's rollouts.

09 pty-runner-per-harness | 03,05,07 | 60K | 27,28,29,30
- Delivers: PTY runner and holder pick the harness from the launch kind (not hardcoded claude); `launchRunner` returns PTY for Codex under headless/auto/pty and herdr only under `agent-runner: herdr`; run records runner; herdr banner/claim check only under herdr.
- Seams: servertest with runnerfake and herdr fake (server/runner_test.go, server/herdrwatch tests); nativerunner pty test with fakecodex.

10 codex-watch-and-startup | 09,08 | 55K | 38,44,64
- Delivers: Codex Watcher: OSC 7501 state, title fallback, session id, Ready; startup dialogs answered/disabled; Background flag -> shared `Background` status.
- Seams: harness tests (harness/osc7501_test.go style), nativerunner pty test with fakecodex dialogs: none becomes a pause.

11 codex-input-and-interrupt | 10 | 50K | 39,40,48,64
- Delivers: reply only at `idle` (busy guard); Tab send, verbatim multi-line paste; Interrupt sends no `/stop`, marker stays; Background agent promptable, not idle.
- Seams: nativerunner conformance (nativerunner/pty_test.go), server reply test.

12 interactive-launches-follow-default | 03,09 | 55K | 1,11,12,13,18
- Delivers: Agents tab Create, `gx agents new`, design launches, Fix agent, TUI Investigate, project worktrees read the resolved default; `--harness` overrides one launch; tier models use the project default; session records its harness.
- Seams: servertest (server/sessions, designs_test.go, fixagent_test.go), cmd/agents_test.go, ui/tickets test for the route.

13 missing-harness-block | 12 | 50K | 20,32,83
- Delivers: one PATH check at the shared process-start seam, before anything stops. Shared message stem + queue/interactive suffix. Queue claim waits (shown by `gx server tickets explain`); interactive launches and Restore refuse; charting handoff refuses before stopping the old agent. Claude without `claude` waits likewise.
- Seams: servertest; cmd test for `tickets explain`.

14 codex-resume-and-slot | 10,11,12 | 55K | 45,46,47,49
- Delivers: `codex resume <id> --cd` when id + rollout exist, else Handoff; process stays after `gx tickets ask` (extend `keepsAskedAgent`); answer with agent gone reopens; Restore of Exited session.
- Seams: servertest + fakecodex.

15 shared-message-reader | 08 | 60K | 50,51,55
- Delivers: one reader, one message list; Claude reader behind it unchanged; Codex reader over `response_item` lines, cached by size+mtime; hidden items; `$gx-` as command; "cannot read" notice when turns exist and zero messages.
- Seams: Go tests in transcript/ and codexsession/; server chat test (server/sessionchat_test.go).

16 archived-rollout-decode | 15 | 35K | 54
- Delivers: `.jsonl.zst` archive notice and an on-demand decode route (choose a zstd lib); no decode on page load.
- Seams: servertest with a `.jsonl.zst` fixture; new route needs routeTable, routeVerbs, web mux, 404 allowlist.

17 session-gates-own-harness | 15,12 | 45K | 86
- Delivers: chat read, prompt, turn settle, failure detection, chat watch, charting handoff read the session's recorded harness; `chatHarness()` stops being a source (test seam only).
- Seams: servertest: after a switch, running Claude session still works and the reverse; config reload without restart.

18 skill-prefix-function | 12,17 | 45K | 53,70,81
- Delivers: one function owns `/` vs `$`; all typing sites use it; run-end line names `/new`+`/gx-design` or `$gx-design`; a test fails on a `"/gx-` literal in non-test Go.
- Seams: Go lint-style test (new, e.g. server/skillprefix_test.go); servertest run-end line.

19 grilling-on-shared-list | 18 | 40K | 52
- Delivers: rounds, grilling counts and charting handoff run on the shared list for Codex sessions.
- Seams: servertest (server/sessionround tests, statsgrilling, conversationsweep) with fakecodex rollout.

20 web-codex-chat-and-composer | 16,15 | 45K | 40,50,51,54,55
- Delivers: web chat view for the shared list, archive button, unreadable notice with doctor pointer, composer `$` hint on `/gx-`.
- Seams: vitest, DOM environment (webui/src/design/ChatLog.test.tsx, webui/src/agents/AgentsPage.composer.test.tsx; confirm first lines for `@vitest-environment`).

21 codex-error-and-usage-class | 10,15 | 55K | 58,59
- Delivers: OSC 7501 `error` -> `needs-answer` after dwell with rollout error text and Retry; usage-limit (100% sample, unexpired reset, or error kind) -> rate-limit hold keeping slot, "continue" at reset.
- Seams: servertest on fake clock (server/ratelimit_hold_run_test.go style); sessionfailure tests.

22 codex-stall-and-recovery-reads | 21 | 40K | 60,61
- Delivers: idle stall nudge for silent Codex; recovery and session lookup read the rollout; no rollout = did nothing, retry from Handoff.
- Seams: servertest (server/nudge, server/recovery tests); sessionlookup tests.

23 codex-approvals | 10,08 | 55K | 41,42
- Delivers: approval card from `PermissionRequest` hook with Approve/Deny (reason); screen-parse fallback (0.166.0 text).
- Seams: servertest + fakecodex; hook-not-fired variant.

24 codex-blocked-dwell-retry | 23 | 40K | 43
- Delivers: blocked past 15s -> `needs-answer` (kind blocked-pane); under 15s no pause; Retry interrupts and sends retry prompt on non-approvable.
- Seams: servertest.

25 codex-usage-gate | 03,21 | 50K | 56,57
- Delivers: per-harness usage gate from rollout `rate_limits` (98%); reason "Codex usage: ..."/"Claude usage: ..."; `gx server usage override [claude|codex]`.
- Seams: servertest fake clock (server/usagesample_test.go, usage_override_test.go, usage_gate_test.go); cmd test for the verb.

26 usage-override-web | 25 | 30K | 57
- Delivers: web override button lifts only that ticket's harness; update flow 104 `usage-gated` text.
- Seams: vitest (verdict.tsx tests); `gx brief -C webui playwright disk.e2e.ts`-style run of flow 104 (use bare flow number).

27 context-window-codexsession | 08 | 35K | 62,74
- Delivers: symlinked-cwd compare fix; `--fits` reads "no usage yet"; `gx agent link`/`step` only `GX_AGENT_SESSION`; context window via rollout.
- Seams: codexsession/session_test.go; cmd/agent_fits_test.go.

28 codex-compaction-gate | 27,21 | 40K | 63
- Delivers: finish gate counts `compacted`; unreadable rollout waits to 5-min cap; drop Codex special case in `readCompactBoundaries`.
- Seams: servertest + ralphloop tests.

29 cost-extra-usage-claude-only | 03 | 35K | 67,68
- Delivers: budget ledger and dollar totals skip Codex; extra-usage check only for `claude` items.
- Seams: server/budget_test.go, server/extra_usage_test.go.

30 row-labels-server | 03,12 | 45K | 22,25,26
- Delivers: wire fields for harness, tier, model, effort, "switched by hand" note; queue row text (follows default / pinned / keeps (ran before)); regenerate with `make web-types`.
- Seams: servertest row text cases (pinned beats keeps; did-nothing run shows no keeps).

31 scenario-default-harness | 08,09,12 | 40K | (scenario)
- Delivers: gxfixture `--fakecodex` flag (installs as `codex`) and scenario `default-harness`.
- Seams: testutil/gxfixture/main_test.go.

32 scenarios-codex-queue-design | 31,03,17 | 40K | (scenarios)
- Delivers: scenarios `codex-queue` (incl. waiting ticket that ran on Claude) and `codex-design`.
- Seams: gxfixture tests.

33 web-agent-rows-design-header | 30,31,32 | 50K | 21,23,25,26 | agents-rows, design-header
- Seams: vitest (webui/src/agents/AgentsPage.sessionView.test.tsx, webui/src/design/DesignPage tests).

34 web-queue-rows | 30,32,33 | 45K | 22 | queue-rows
- Seams: vitest for epic rows (webui/src/epic/rows.ts tests).

35 settings-api | 03,12,13 | 50K | 6,7,9
- Delivers: server route + CLI verb to read/write default harness (global/project), symlink-safe, keys kept, Remove override, PATH refusal, switch-notice counts.
- Seams: server test on temp config dir; route needs routeTable/routeVerbs/web mux/404 allowlist.

36 settings-agents-page | 35,31 | 60K | 2,3,4,10 | settings-home, settings-agents
- Seams: vitest webui/src/settings/SettingsPage.test.tsx (check env).

37 tui-default-harness-picker | 35 | 35K | 5
- Seams: TUI model test (teatestv2); no server = standard refusal.

38 tui-harness-rows | 30 | 45K | 24,66
- Seams: ui/agents/model_test.go, ui/tickets queue tests, design view.

39 codex-cost-columns-and-watch | 29,38,33 | 45K | 65,66
- Seams: vitest, TUI tests (Watch PTY vs herdr).

40 skills-install-root | - | 40K | 69,73
- Seams: skills/install_test.go (temp home), bundle_test.go.

41 agent-model-tier-effort | 40 | 35K | 72
- Seams: cmd/agent_model_test.go; skills test that every skill reading `gx agent model` matches output.

42 skills-neutral-1 | 41 | 50K | 71
43 skills-neutral-2 | 42 | 55K | 71
44 skills-neutral-3 (design, verify-epic, evolve, investigate, to-tickets) | 43 | 55K | 71,78
- Seams (all three): one shared test file per skill area (skills/bundle_test.go); structural anchors only.

45 codex-doctor-framework | 15,08 | 55K | 75,77
46 codex-doctor-live-record | 45,05 | 55K | 35,75,76,77
47 codex-setup | 45 | 40K | 36
- Seams: unit tests on temp CODEX_HOME; fixture-mode doctor in CI.

48 docs-adr-glossary-changelog | 47,44,46,13,35,36 | 30K | 80
- ADR, glossary entries, README, changelog entry, skills guide paragraph, smoke checklist.

49 flow-default-harness-setting | 36,33,31 | 50K | 1,2,3,4,7,10,11,21,25,26 | settings-agents-switched (flow 117)
50 flow-codex-queue-rows | 49,34,32,13 | 50K | 1,15,19,20,22 | queue-rows missing-harness-reason (flow 118)
51 flow-codex-approval-card | 50,24 | 45K | 38,41,43 (flow 119, scenario codex-blocked built here)
52 flow-codex-rate-limit-wait | 51,25,26 | 45K | 56,57,58 (flow 120, scenario codex-rate-limited)
53 flow-codex-error-pause | 52,21 | 40K | 59 (flow 121, scenario codex-error)
54 flow-codex-design-run | 53,19,20 | 40K | 12,52,23 (flow 122)

Highest flow number now: 116 (webui/e2e/CATALOG.md); new flows start at 117.
Then `ensure-code-review`.

## Coverage table (story -> implement ticket -> verify part)

1: 03,12,49,50 -> P2b | 2,3,4: 36 -> P1a | 5: 37 -> P6a | 6: 35 -> P6a | 7: 35,36 -> P1a | 8: 01 -> P6b | 9: 35 -> P6a | 10: 36 -> P1b | 11: 12 -> P1b | 12: 12 -> P3 | 13: 12 -> P6a | 14: 04 -> P6a | 15: 02,03 -> P2a | 16: 03 -> P6a | 17: 02 -> P6a | 18: 02,12 -> P6a | 19: 03 -> P2b | 20: 13 -> P2a | 21: 33 -> P1b | 22: 30,34 -> P2a | 23: 33 -> P3 | 24: 38 -> P8 | 25,26: 30,33 -> P1b | 27-30: 09 -> P7 | 31: 01 -> P6b | 32: 13 -> P6b | 33,34: 05 -> P6b | 35: 05,46 -> P6b | 36: 47 -> P6b | 37: 06 -> P6b | 38: 10 -> P4 | 39: 11 -> P4 | 40: 11,20 -> P8 | 41,42: 23 -> P4 | 43: 24 -> P4 | 44: 10 -> P4 | 45,46,47,49: 14 -> P7 | 48: 11 -> P4 | 50,51: 15,20 -> P8 | 52: 19 -> P3 | 53: 18 -> P3 | 54: 16,20 -> P8 | 55: 15,20 -> P8 | 56,57: 25,26 -> P5a | 58: 21 -> P5a | 59: 21 -> P5b | 60,61: 22 -> P5b | 62: 27 -> P5b | 63: 28 -> P5b | 64: 10,11 -> P4 | 65,66: 38,39 -> P8 | 67,68: 29 -> P8 | 69,73: 40 -> P9a | 70,81: 18 -> P9a | 71: 42-44 -> P9a | 72: 41 -> P9a | 74: 27 -> P9a | 75,77: 45,46 -> P9b | 76: 46 -> P9b | 78: 44 -> P9a | 79: 07,08 -> P9b | 80: 48 -> P9b | 82: 02 -> P6a | 83: 13 -> P6b | 84: 02,03 -> P6a | 85: 04 -> P6a | 86: 17 -> P6a

## Verify parts (base 15K, mocks 8K once per part, cap 70K)

- P1a stories 2,3,4,7 | scenario default-harness | mocks settings-agents, settings-home, settings-agents-switched | 67K
- P1b 10,11,21,25,26 | default-harness | mock agents-rows | 64K
- P2a 15,20,22 | codex-queue | mock queue-rows | 52K
- P2b 1,19 | codex-queue (+default-harness for story 1) | mock settings-agents-switched | 49K
- P3 12,23,52,53 | codex-design | mock design-header | 53K
- P4 38,39,41,42,43,44,48,64 | codex-blocked | none | 61K
- P5a 56,57,58 | codex-rate-limited | none | 53K
- P5b 59,60,61,62,63 | codex-error | none | 41K
- P6a 5,6,9,13,14,16,17,18,82,84,85,86 | none (servertest/cmd) | none | 63K
- P6b 8,31,32,33,34,35,36,37,83 | none | start-error | 62K
- P7 27,28,29,30,45,46,47,49 | none | none | 47K
- P8 24,40,50,51,54,55,65,66,67,68 | none | none | 67K
- P9a 69,70,71,72,73,74,78,81 | none | none | 53K
- P9b 75,76,77,79,80 | none | none | 37K
- verify-collect: 30K, blocked by all 14 parts.
Caveats: story 1 spans two scenarios (kept in P2b with the queue scenario, the default-harness half is also covered by P1); story 80 is hand-run, so its part only checks the checklist exists (2K); 5-6 stories (e.g. 83) lump several launches in one 4K entry.

## Risks to raise

- Ticket 46 `--record` needs a real Codex 0.166.0 and login (codex is installed here at 0.166.0). An unattended agent can build the flag and seed fixtures from fakecodex; real recordings may need a person.
- Chain of 54 tickets is long; flows 49-54 are chained because they share webui/e2e/CATALOG.md.
- `server/runner.go` (987 lines) and `server/sessions.go` (1033, over the file-size rule) are touched by 03, 09, 12, 14: all chained.

---

# Where-to-look detail, keyed by slug

01 default-harness-config: config/agents.go (AgentsConfig, defaults), config/project_overrides.go:projectKeys (`agents` already whitelisted; add sub-key if nested), server/configreload.go:changedKeys/reloadableSettings, agentrunner/select.go:Select (`LookPath("claude")` at lines 43 and 53), cmd/server_preflight.go:preflightAgentRunner. Existing tests: agentrunner/select_test.go, config/project_overrides_test.go, config/parse_test.go. Server env: server/env.go. Not verified: README doc section name.

02 resolver-and-queue-defaults (callers verified at b97febaa): ticketAgent(server/ticketwrites.go:305) used at herdrwatch.go:102, runner.go:373 and :959, ticketwrites.go:251,258,290, verdicts.go:137. queuedAgent(server/claimhold.go:52) used at herdrwatch.go:102, recovery.go:735, ticketwrites.go:290, verdicts.go:137. Other sites (rg, not verified): server/queue.go:resolveAgent (defaults to claude), server/epicmerge.go:ensureRootQueued (`newQueueItem(addr, AgentClaude)`), ui/tickets/server_mode.go:serverEnqueueDefaultAgent, ui/tickets/queue_epic.go:20, cmd/server_reads.go:483,509 (help says default claude), server/oneoff.go:262. Test export: server/export_test.go:24 `TicketAgent`.

03 resolver-at-claim-sites (verified at b97febaa): launchRunner(server/runner.go:905) callers: claimhold.go:27, herdrwatch.go:103, runner.go:502 and :961, ticketwrites.go:251. iterationRunner(runner.go:953) falls back to AgentClaude; ticketRelaunch(ticketwrites.go:225) calls herdrHolds, usageGate (:258), claimAndLaunch (:290). claimHold(claimhold.go:23) feeds claimRoot and scheduleVerdict (verdicts.go:137). Usage gate: server/usagesample.go:usageGate(180). Run struct has `Agent` and `Runner` fields (server/runner.go keepsAskedAgent). How a run's session/rollout is recorded: not verified, implementer finds (nativerunner Meta.SessionID, conversations.Record).

04: server/oneoff.go (Agent at :52, :141, :262-266), server/retrostamp.go, server/epicmerge_root.go, cmd/server_oneoff.go:63, cpuspike.go (not found by that name; rg `cpuspike` in cmd and server, not verified).

05: ralphloop/loop.go:AgentArgs/agentArgs/skillPrompt (line ~181-215, `--add-dir` for extraDirs), harness/harness.go:Lookup (only Claude registered), harness/typing.go:Codex (typing only), server/sessions.go:launchArgs (~line 423, Codex model/effort args), config/agents.go (Codex tier models). Test helpers: harness/harness_test.go.

06: cmd/agent_guard.go:guardBashCommand/newAgentGuardBashCmd, harness/systemprompt.go, ralphloop/loop.go:claudeGuardSettings.

07/08: testutil/agentfake/{harness.go,program.go,tui.go,claude.go}, testutil/agentfake/cmd/fakeclaude/main.go (pattern), testutil/herdrfake/codexrollout.go (existing rollout writer to reuse), codexsession/session.go (production reader). gxfixture builds fake via `binary(fakeBin, binDir/"claude", ".../cmd/fakeclaude")` in testutil/gxfixture/main.go:191. nativerunner/fakeclaude_test.go is the pattern for a PTY test.

09: nativerunner/pty.go:ptyHarness (line ~377, "always claude") and holder args `--harness harness.Claude` (~line 365), nativerunner/holder.go:91 harness.Lookup(kind), server/runner.go:launchRunner/keepsAskedAgent, server/herdrwatch.go, agentrunner/select.go. Test helpers: nativerunner `shortTempDir`, servertest runnerfake, testutil/herdrfake.

10/11: harness/osc7501.go, harness/claude.go (Watch pattern), harness/harness.go (Watcher, Status.Background), harness/typing.go (codexTyping), nativerunner/pty_personinput.go, pty_test.go (conformance).

12 (chatHarness refs verified at b97febaa): designs.go:366,517; fixagent.go:143; projectworktrees.go:37; sessionchat.go:169; sessionchatwatch.go:96; sessionfailure.go:100; sessionhandoff.go:28; sessionprompt.go:95; sessionturnsettle.go:26; sessionwebcreate.go:74. Launch sites: server/sessionwebcreate.go, cmd/agents.go:274 (`--harness`), ui/tickets/start_session.go:118, server/fixagent.go:91. Tests with `servertest.WithChatHarness` are the seam to keep.

13: shared process-start seam: nativerunner/pty.go start and server launch paths (not verified which single function; implementer picks), cmd/server_reads.go (tickets explain), server/claimhold.go.

14: server/runner.go:keepsAskedAgent, server/sessions.go (Restore), codexsession/session.go:VerifyIdentityIn, ralphloop/reconcile.go:105.

15: transcript/chat.go:ReadChat (consumers: cmd/tickets_migrate_grilling.go:112, server/sessionchat.go), transcript/{last_assistant_text,turn_end,api_error}.go, codexsession/session.go, conversations/conversations.go (Record.Harness).

16: .ai/index.md "Adding a server route"; no zstd dep in go.mod (not verified: rg go.mod).

17/18/19: see 12 list; sessionactionable.go:108, sessionhandoff.go:93, recovery.go:735, designs.go:355, ui/tickets/start_session.go:118, ralphloop/loop.go:skillPrompt, testutil/agentfake/design.go:17 (test-only literal). grillround/*.go, server/sessionround.go, server/conversationsweep.go.

21-24: server/sessionfailure.go, ralphloop/ratelimit_hold.go, ralphloop/waitforfinish.go (Codex branches at ~76-101, 168-175), recovery/catalog.go:246, ADR docs/adr/0042-few-pauses-all-recoverable-from-the-web.md, e2e flows 115/116 in webui/e2e/CATALOG.md.

25/26: server/usagesample.go (usageGate:180, usageOverride:206), nativerunner/pty_usage.go, webui/src/verdict.tsx, CATALOG flow 104.

27/28: cmd/agent_fits.go, cmd/agent.go:60, codexsession/session.go, ralphloop/waitforfinish.go:readCompactBoundaries(370), ralphloop/deps.go:320 (ReadCodexContext).

29: server/budget.go, server/extra_usage.go, server/queue.go:withExtraUsage(250).

30-39: webui/src/agents/SessionView.tsx:modelLabel (already prints `<Tier> · CLI default`), webui/src/settings/SettingsPage.tsx (367 lines), server/sessiontypes.go, ui/agents/, ui/tickets/queue_server_live.go. Mock: `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html` states settings-home(480), settings-agents(640), settings-agents-switched(640), agents-rows(640), queue-rows(640), design-header(640), start-error(640).

40-44: skills/roots.go (CodexSkillsRoot already `~/.agents/skills`), skills/install*.go, scripts/configure-codex-global-approvals.sh (replaced by 47), skills/codex_discovery_test.go, 16 skills with `disable-model-invocation: true`; `gx agent model` readers: gx-research, gx-wayfinder, gx-triage-follow-ups, gx-design, gx-code-review, gx-evolve, gx-local-tracker.md.

45-47: cmd/claude_doctor.go, claudedoctor/{doctor,fixtures,pty*}.go and testdata/ as the pattern; cmd/claude.go for the command group.
