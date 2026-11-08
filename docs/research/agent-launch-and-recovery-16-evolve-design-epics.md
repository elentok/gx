# Evolve on design epics: what evolve reads, what a design epic has

Research for gx:agent-launch-and-recovery/16. Facts only, no design decisions. Sources are the code in this worktree and `~/.claude/skills/gx-evolve/SKILL.md`. Anything not confirmed is marked **unconfirmed**.

## 1. What evolve reads from a queue ticket

`gx evolve inputs <epic>` (`cmd/evolve/evolve_inputs.go:277` `runEvolveInputs`) builds one `InputsTable` for the epic. The gx-evolve skill runs it itself (SKILL.md step 2). Steps below name each source and its contract.

### 1.1 Entry and target resolution

- `resolveEvolveTarget` (`cmd/evolve/evolve.go:94`) parses `project:epic`, runs `evolveGuard` (`:69`, refuses maintainer mode with no `gx` project) and loads the epic with `tickets.Load`. It has no map check. A map epic resolves like any other.
- The run log is read from `runlog.ReadEvents(filepath.Dir(t.epic.Path), t.name)` (`evolve_inputs.go:282`). The file is `<project dir>/<epic>/run-log.jsonl` (`ralphloop/runlog/eventlog.go:195`). A missing file returns `ok=false` and no events, not an error.

### 1.2 The per-ticket row (`InputsRow`, `evolve_inputs.go:52`)

| Field | Source | Contract |
|---|---|---|
| `id`, `type`, `parent`, `status` | ticket frontmatter via `tickets.Load` | Always present. |
| `expected_context` | `expected_context_window` frontmatter (`tk.ExpectedContextWindow`) | Set only on tickets written with an estimate (verify parts). 0 otherwise. |
| `actual_context` | `actual_context_window` frontmatter, stamped by the server at finish | 0 when never stamped. |
| `elapsed_seconds` | `elapsed_time` frontmatter, stamped at finish | 0 when never stamped. |
| `cost` | `actual_cost` frontmatter | 0 when never stamped. |
| `sessions` | `schema.ParseTicket(...).SessionIDs` (`session_ids` frontmatter, appended at claim by `ralphloop/ticketops/claim.go:136,200`) plus `ev.AgentSession` of every `iteration-started` run-log event (`digestTicketEvents`, `evolve_inputs.go:170`) | Deduplicated, frontmatter first. More than one entry means a relaunch. |
| `conflict_sessions` | `AgentSession` of `conflict-hit` and `conflict-resolved` run-log events | Listed apart from `sessions`, so they never count as relaunches. |
| `events` | per-type counts of the run-log events whose `Ticket` equals the ticket (`byTicket`, `evolve_inputs.go:103`) | A `paused` event is counted under `paused-answer` or `paused-repair` by its `Kind` (`eventCountKey`, `:204`, via `events.Kind.AwaitsAnswer`). |
| `merged` | `Ralph-Loop-Ticket` trailer on the epic's branch (`defaultMerged`, `:364`, `history.TrailerMap(repo.Root, epic, merge.TicketTrailerKey)`) | `nil` when the project has no repo or the branch is unreadable (an error is swallowed at `:289`; only the column is lost). |
| `fix_tickets` | children of a `code-review` ticket, minus `conflict-resolution` and `investigate` forks (`:116-126`, `e.ForkParents()`) | Derived from ticket frontmatter `parent`. |
| `flags`, `severity` | `inputsFlags` and `inputsSeverity` (below) | |
| `digests` | `writeDigests` (`evolve_digests.go:440`) | One text file per session, see 1.4. |

### 1.3 Flags (`inputsFlags`, `evolve_inputs.go:214`)

| Flag | Condition | Needs |
|---|---|---|
| `paused-answer` | a `paused` run-log event with an answer kind, or the ticket's current paused class is answer | run log, or status and `pause_reason` frontmatter |
| `paused-repair` | same, repair class | same |
| `recovery` | any run-log event type starting `recovery-` | run log |
| `deadlock` | a `deadlocked` run-log event on the ticket | run log |
| `conflict-hit` | a `conflict-hit` run-log event | run log |
| `relaunch` | `len(row.Sessions) > 1` | `session_ids` frontmatter and `iteration-started` events |
| `nudge` | a `nudged` run-log event | run log |
| `context-overrun` | `ticketspec.Work(tk) > 2 * ExpectedContext` where `Work` is `actual_context_window` minus `prefix_tokens` (fallback prefix when unstamped, `tickets/ticketspec/estimates.go:28`) | `expected_context_window` and `actual_context_window` |
| `slow` | `ElapsedSeconds > 2 * median` of the epic's non-zero `elapsed_time` values | `elapsed_time` |
| `metrics-missing` | status `done`, not commitless, and `actual_context` or `elapsed` is 0 | frontmatter |
| `has-fix-tickets` | the ticket has fix children | `parent` frontmatter |
| `slow-launch` | added later by `flagSlowLaunches` (`evolve_launch.go:567`) when the ticket has a launch row; never changes severity | `launch-timed` run-log events |

Severity (`inputsSeverity`, `:248`): any of paused-answer, paused-repair, recovery, deadlock gives 3; `conflict-hit` 2; `context-overrun` 1; everything else 0. Rows are stable-sorted by severity descending (`:163`). Nudge counts and relaunch counts are therefore not separate numbers: they surface as the `nudge` and `relaunch` flags, and as the `events` map (`nudged=N`) and the `sessions` list.

Run-log events used by name: `events.Deadlocked`, `events.IterationStarted`, `events.ConflictHit`, `events.ConflictResolved`, `events.Nudged`, `events.Paused`, `events.LaunchTimed`, and the `recovery-` prefix. Commits are not read from the run log; "commits" appear only as the `merged` trailer column and as `commitless` in the `metrics-missing` flag.

### 1.4 Table-level inputs

- `median_elapsed_seconds` and `deadlocks` (epic-wide `deadlocked` events with no ticket) (`evolve_inputs.go:105,134`).
- `slower`, `slower_baseline_runs`, `flakes`, `flakes_no_tree` from `buildEpicChecks` (`cmd/evolve/evolve_checks.go`). It reads the check record (`check.ReadRecords(dataDir, project)`) and the brief index (`brief.ReadIndex`). It filters records by branch names built with `worktree.IterationBranch(epic, ticketID)` for every ticket plus the epic name. Slower rows need a `verify` ticket branch, a first `claimed_at` and at least 5 earlier full runs (`baselineRunsMax`, `:20`). Without `dataDir` there are no rows.
- `launches` from `launch-timed` events (`evolve_launch.go:540`): a row per launch whose total exceeds `events.SlowLaunch`, plus a ticket-less median row.
- `digest_dir`: a temp dir `gx-evolve-digests-*` (`:305`), one `<ticket>-<session>.md` per session and `<ticket>-<session>-sub-<name>.md` per subagent.

### 1.5 Digests (`cmd/evolve/evolve_digests.go`, `sessionlookup/lookup.go`)

- Session ids come only from the table rows (`row.Sessions`, `row.ConflictSessions`); a conflict session already owned by another row is skipped (`:443-459`).
- `digestSession` (`:476`) computes the native agent dir from `worktree.IterationIdentity(epic, ticket, "")` and `nativerunner.AgentsRoot(stateDir, project)`. It then calls `sessionlookup.Lookup` in the order: Claude transcript by id (`transcript.FindByID(home, id)`), native agent dir, Codex rollout. No hit gives `Source: "missing"`. An unreadable session logs a warning and also becomes `missing` (`:463-467`).
- The skill treats `missing` as a gap to report, not a clean run (SKILL.md step 2).

### 1.6 How the skill uses the table (`~/.claude/skills/gx-evolve/SKILL.md`)

1. Validate own ticket, `gx tickets show` to read `Retro of epic <project>:<epic>`.
2. `gx evolve inputs <epic>` run by the evolve agent itself. The main session never reads transcripts.
3. Deep-read: always the epic's `code-review` ticket and its fix tickets, a conflict-hit ticket's conflict-resolution session; then by severity up to `gx config get evolve.deep-read-cap` (default 8, `config/evolve.go:11`, `config/config.go:63`). One `general-purpose` subagent per ticket on the worker tier, briefed with the ticket id, flag row and digest paths. Tickets with no flags are never read.
4. Verify parts: compare actual tokens to budget and budget to story estimates (`gx tickets verify-plan --table`); a `verify-cap-stopped` run-log event marks the part as off.
5. Check rows (`slower`, `flake`) and launch rows are findings without a deep read.
6. Join by symptom, consultant triage on the `strongest` tier against `gx:follow-ups` and the project's follow-ups epic.
7. File via the gx-follow-up skill, unattended.
8. Record `## Result`, flag table, raw findings, triage and disposition in the evolve ticket; close with `--iteration-status finished --commitless true`.

### 1.7 Which inputs a map ticket or design session has no source for

Confirmed against the real map `gx:agent-launch-and-recovery` (`/Users/david/.local/share/gx/tickets/gx/agent-launch-and-recovery/`) and the loader:

| Input | Map ticket / design session | Evidence |
|---|---|---|
| Run log (`run-log.jsonl`) | None. The map dir holds only `ticket.md` and `issues/`. Map tickets are never scheduled, so no `iteration-started`, `paused`, `nudged`, `recovery-*`, `conflict-*`, `deadlocked`, `launch-timed` or `verify-cap-stopped` event is written for them. | `ls` of the map dir; `ReadEvents` returns no events for a missing file. |
| Recovery events | None (same reason). | |
| Nudge and relaunch counts | None. `nudge` needs `nudged` events. `relaunch` needs more than one entry in `sessions`, and a hand-resolved map ticket has no `session_ids` (ticket 11's frontmatter has `claimed_at`, `done_at`, `total_tokens: 0` and no `session_ids`, `elapsed_time`, `actual_context_window`). | Ticket `issues/11-*.md` frontmatter. |
| Per-ticket metrics | Not stamped on map tickets: `elapsed_time`, `actual_context_window`, `actual_cost` are 0 (`total_tokens: 0` is present). As a consequence a `done` non-commitless ticket would get `metrics-missing`; map tickets are `commitless: true` in the sample, so that flag is not raised. `slow` has no data (median 0 disables it). `context-overrun` needs `expected_context_window` (not set on map tickets). | Same frontmatter; `inputsFlags` `:238-241`. |
| `paused-*` flags | `paused-answer`/`paused-repair` also read `status`/`pause_reason` (`schema.Ticket.PausedClass`); a map ticket resolved in a live session is not paused in the queue sense. **Unconfirmed**: whether any map ticket can carry a paused status. | `evolve_inputs.go:229`. |
| Commits / `merged` | The trailer lookup runs `history.TrailerMap(repo.Root, epic, ...)` on a git ref named after the epic (the map name). **Unconfirmed**: whether a branch of that name exists for a map (the Design uses branch `<map>-design`, `server/designs/designs.go:25`, and later `<map>-impl`). Design commits carry no `Ralph-Loop-Ticket` trailer as far as read; **unconfirmed**. A failure only drops the column (`:289`). | `designBranch`, `defaultMerged`. |
| Fix tickets | `ForkParents().IsCodeReview()` matches code-review parents; a map has none. | |
| Digests of design sessions | Not reachable through the table: session ids come from `session_ids` and run-log events, and a design session id is neither. See section 2.1 for where the ids actually live. | `writeDigests`. |
| Check rows | Branches are `IterationBranch(epic, ticketID)` for each ticket plus the epic name; design work runs on `<map>-design` (outside that set). A map has no `verify` ticket, so `slower` is empty regardless. `flakes` match records on the epic branch names. **Unconfirmed**: whether any check record carries the design branch. | `evolve_checks.go:73-85`. |
| `claimed_at` baseline | Hand-resolved tickets do carry `claimed_at` (ticket 11), so `firstClaim` is set, but `slower` still needs a `verify` branch. | |

## 2. What a design epic has

### 2.1 Design sessions and transcripts

- A design session is an interactive session (not a ticket run) managed by `server/sessions`. Its meta (`server/sessions/sessiontypes.go:25`, `Meta`) records `ID`, `Harness`, `Project`, `Dir`, `Ticket` (the map address), `Design`, `LaunchDesign`, `Tier`, `Model`, `Effort`, `ConversationID`, `CreatedAt`, `Run` (resume count), status and exit. It lives in the session state dir (`SessionsDir`).
- `gx agent link` / `linkSession` (`server/sessions/sessionlink.go:33`) sets `Meta.Ticket` and `Meta.Design` (true when the ticket is on a map epic).
- A separate, durable **conversation record** lives in `<data>/conversations/<id>.json` (`conversations/conversations.go:24`, `Record`): `id` (the harness conversation id, the same id `transcript.FindByID` and `sessionlookup.Lookup` accept), `dir`, `harness`, `ticket` (for design sessions the epic address `project:epic`, e.g. `gx:agent-launch-and-recovery`), `recorded_at`, `totals` (tokens and cost, set at conversation end), `design`, and `grilling` (list of `{message_id, at, questions, words}`). It outlives the session's state dir and needs no server to read.
- On the live map, five conversation records (all `claude`, `design: true`) are linked to `gx:agent-launch-and-recovery`, with totals from about 1.8M to 8.1M tokens and grilling item lists of 2 to 6 entries.
- Transcripts: Claude JSONL under the home dir, found by `transcript.FindByID(home, id)`; Codex rollouts under `$CODEX_HOME`. `sessionlookup.Lookup` returns markdown plus subagent markdown. It does not need a ticket or an agent dir for these (the native dir is an optional second source). So design conversations are digestible by id with the existing digest code once the ids are known.
- `conversations.SumEpicTotals(root, "project:epic", createdAt)` (`conversations/conversations.go:186`) sums the conversations recorded against the epic or any of its tickets (`project:epic/id`). It is used at map completion (below).
- Counting of grilling rounds: `grillround.Count` (`grillround/grillround.go`) turns a conversation's chat messages into one item per Grilling round and one per closing summary; `convrecord.Recorder.saveGrilling` and `SweepDesignCounts` (`server/sessions/convrecord/convrecord.go`) write them onto the record when a Design conversation ends or at a sweep. A record whose transcript is missing stays uncounted (`grilling` null).

### 2.2 Map tickets, `spec-round-` tickets and the `design_step`

- A map epic is `kind: map` in `ticket.md` (`tickets/epic.go:71`, `IsMap`). Its tickets are hand-resolved. Ticket frontmatter seen on the live map: `status`, `type` (`research`, `grilling`), `commitless: true`, `claimed_at`, `done_at`, `created_at`, `original_address`, `total_tokens: 0`. Open ticket 16 (this one) carries `claimed_at` only.
- `design_step` is a map-epic frontmatter field with the values `chart, resolve-tickets, spec, review, grill-findings, create-tickets, branch, done` (`tickets/design_step.go:12-21`). It is changed with `gx tickets set <map> --design-step ...`; moving earlier needs `--allow-backward`. The map also records `design_started_at` (server-stamped, `tickets/epic_add.go:39`). **Only the current value is kept**: no history of step moves is stored in frontmatter. **Unconfirmed**: whether a step change is logged anywhere else (the epic has no `.jsonl`; no grep hit for a design-step event writer was followed further).
- `spec-round-` tickets: the gx-design skill (`skills/gx-design/SKILL.md:145-150`, installed copy `~/.claude/skills/gx-design/SKILL.md`) records each spec grilling round as one closed `grilling` map ticket with slug prefix `spec-round-`; Question is the questions, Resolution the answers, plus a Decisions so far line. The round cap is 2, counted by these tickets. Example epics with them exist in the store (`09-spec-round-01.md`, `04-spec-round-01.md`, `05-spec-round-02.md` under other maps). The live map `agent-launch-and-recovery` has none (0 `spec-round` files); it is still at the resolve-tickets step.
- Review findings: the skill writes them as a checklist in the map body under `## Review findings` (and `## Re-review findings` for the second review) with `gx tickets section` (SKILL.md steps 5, 6, 6a, 6b, lines 161-180). A checked box carries a one-line resolution and optionally the marker `(stories/scope changed)`. Findings live in the map's `ticket.md` body, not in a separate file. Self-decided design questions go in the spec under `## Self-decided design questions` (from the third draft; skill line 152-154) in `docs/specs/<epic>.md`.
- The spec itself is `docs/specs/<epic>.md`, committed on the `<map>-design` branch (`designBranch`, `server/designs/designs.go:25`). Design-review rubric: skill lines 209-232.
- Map completion: when `<map>-impl` merges, `ticketmigrate.MarkMapDone` (`tickets/ticketmigrate/map_done.go:17`) copies `merged_at` onto the map and sets `status: done`. `tickets set <map> --completed-at` stamps `completed_at` plus `total_tokens` and `actual_cost` summed from conversation records (`tickets/epic_add.go:182`, `mapCompletionFields`; `cmd/tickets/tickets_set.go:108-110`, a map epic is the one epic kind that takes `completed_at` by hand). `mapCompletedBy` (`epic_add.go:204`) completes a map by creating `<map>-impl`.

### 2.3 Where answers to questions are recorded today

- **Design rounds (web).** The design question sheet (`webui/src/design/QuestionSheet.tsx`, Huh? button at line 120) keeps `Answer = { choice, text, note, huh }` (`webui/src/design/round.ts:3`). `Composer.tsx:111` sends `roundPrompt(round, answers, text)` to the session as a prompt. A Huh? answer becomes the fixed line `HUH_MESSAGE = "I don't understand the question, please explain in a clearer way"` (`round.ts:5`), rendered as `Q<label> - <title>: <message>` (`roundPrompt`, `round.ts:38-47`). The only server-side record is the conversation transcript (and the `grilling` item counts, which count questions and words per round, not answers). No separate click event or log line exists. **Confirmed by grep**: `huh` appears only in `webui/src/design/` and the test file.
- **Agent questions (queue tickets).** The ticket log `<NN-slug>.jsonl` (`tickets/log.go`) holds `pause.question`, `pause.answer`, `pause.permission`, `pause.closed`, `pause.repair`, `pause.prompted`, `pause.recommend`. `server/ticketops/answer.go:164` appends `pause.answer` with `Source`, `How: answer`, `Question` (seq), `Choice` (or `Text` for a free-text question) and `Note`. `AnswerForm` (`webui/src/ticket/AnswerForm.tsx`) has no Huh? control today (no `huh` match). `validateAnswer` (`answer.go:61`) refuses an empty `answer` (`ReasonEmptyAnswer`), so a note-only answer is not accepted yet (ticket 11 plans to change this). **Unconfirmed**: how "Huh?" is expressed for an agent question until ticket 11 lands.
- The `pause.answer` events of a queue ticket are not read by `evolve inputs` directly; evolve sees pauses through `paused` run-log events and the ticket's status. **Unconfirmed**: whether `server/epics/detailruns.go` or other readers expose answer notes to evolve; none of the `cmd/evolve` files read `.jsonl` ticket logs.
- Design sessions have no `pause.question` or `pause.answer` events: they are not tickets and the question is in the chat. The map's own ticket logs are absent (no `*.jsonl` in the map's `issues/` dir).

## 3. How an epic becomes "evolved"

### 3.1 The fields

`tickets/epic.go:128-130`: `EvolvedAt time.Time`, `EvolveSkipped bool`, `EvolveTicket string` (address of the retro ticket). Loaded from `ticket.md` frontmatter by `tickets/loader.go:450-452` (`evolved_at`, `evolve_skipped`, `evolve_ticket`). They are listed in `tickets.EpicServerStamped` (`tickets/epic_add.go:39`), and `gx tickets set` / `tickets epic set` refuse to write them (`cmd/tickets/tickets_set.go:108-110`; only `completed_at` on a map is allowed). The TUI preview shows `evolved_at` (`ui/tickets/preview/preview_frontmatter.go:110`). No web code shows them (no match in `webui/src`).

### 3.2 Who stamps

1. **Done retro**: `ticketwrite.Writer.stampRetro` (`server/ticketwrite/retrostamp.go:84`), called from `StampFinished` after any done or cancel path (`:34`). It acts only on an `evolve` ticket whose `unique` key starts with `evolve:` and parses `<project>/<epic>` from it. Done: `OverwriteEpicFields(epicPath, {evolved_at: now, evolve_ticket: <addr>})` (overwrites a forced re-run's predecessor). Cancelled: `StampEpicFields({evolved_at: now, evolve_skipped: true})` (keeps an earlier finished retro's stamp). Neither checks `IsMap`.
2. **`gx evolve skip <epic>`** (`cmd/evolve/evolve.go:178`): `StampEpicFields({evolved_at, evolve_skipped: true})`, never `evolve_ticket`. No map check.

### 3.3 What starts a retro

- **Completion hook**: `Merger.submitRetro` (`server/merge/epicmerge_root.go:322`) runs at a loop epic's final outcome (merged, completed, root pause, already merged; `mergeDecision.submitRetro`, `:172`, only for the first such decision). It returns early when `e.IsMap || tickets.IsOneOffEpic(e) || !e.EvolvedAt.IsZero()` (`:323`), when the project's evolve mode is `off`, and in maintainer mode with no `gx` project. It calls `ticketwrite.RetroSubmit` through `s.oneOffs.Submit`.
- Map epics also never reach the merge path: `server/merge/epicmerge.go:106` refuses a merge of a map ("its tickets are resolved by hand"), and `handmerged.go:35` skips maps. Map completion goes through `MarkMapDone` / `completed_at`, neither of which calls the retro hook.
- **`gx evolve run <epic> [--force]`** (`evolve.go:124`): submits `RetroSubmit(project, epic, now)` through the server. It refuses an already evolved epic without `--force` and has no map or one-off check.
- **`gx evolve all`** (`evolve.go:212`, `evolveCandidates`): excludes `e.IsMap`, one-off epics and evolved epics; requires `completed_at` inside the window (default 30 days).
- `RetroSubmit` (`retrostamp.go:21`): prompt `Retro of epic <project>:<epic>`, name `evolve-<epic>-<yyyymmdd-hhmm>`, type `evolve`, dedupe key `evolve:<project>/<epic>`, `Notify: true`. The one-off is an `evolve` ticket (`tickets/schema/ticket.go:114`, top-level only, commitless; `iterationmode.go:67`).
- The launch: `LaunchSkill` maps type `evolve` to the gx-evolve skill (`server/recovery/launch.go:32`). `Launcher.launch` gives the agent extra writable dirs through `evolveDirs` (`server/sched/launch.go:107,134`): the project's follow-ups epic dir and the `gx` project dir.

### 3.4 What stops a map epic reaching "evolved" today

- Automatic: the hook guard `e.IsMap` (`epicmerge_root.go:323`) and the fact that maps never take the loop merge path (`epicmerge.go:106`).
- `gx evolve all`: `e.IsMap` exclusion (`evolve.go:220`; covered by `evolve_test.go:243`, the `mapped` epic).
- Not blocking: `gx evolve run <map>` and `gx evolve skip <map>` accept a map, and `stampRetro` would stamp one. So a map can be evolved by hand today. **Unconfirmed**: whether `gx evolve inputs <map>` completes successfully; reading the code, `schema.ParseTicket` per map ticket, `buildEpicChecks`, `defaultMerged` and `writeDigests` all tolerate missing data, but it was not run and no test covers a map.
- `gx tickets set` cannot write the three fields on any epic, a map included (`tickets_set.go:108`).
- The migration side: no code reviewed ties `evolved_at` to a migration step. **Unconfirmed** whether a map would need a `migrations/steps.go` entry; the fields already exist on the `Epic` type for all kinds.

## 4. What a design epic can reuse unchanged

| Part | Reusable unchanged? | Why (file) |
|---|---|---|
| Epic target resolution (`resolveEvolveTarget`) | Yes | No map check; takes any `project:epic`. |
| `evolve run` / `skip` stamping path | Yes | Map-agnostic: `RetroSubmit`, `stampRetro`, `StampEpicFields`, `OverwriteEpicFields`. |
| The evolve one-off (type, launch skill, extra dirs, follow-ups outbox, `gx evolve export`) | Yes | The one-off is keyed by `project/epic` only; `evolveDirs`, `LaunchSkill` and `runEvolveExport` (`evolve_export.go`) do not look at the epic kind. Export reads the `gx` project's `follow-ups` epic drafts. |
| Session digests (`sessionlookup.Lookup`, `writeDigests` mechanics) | Mechanism yes, input no | The lookup takes any conversation id and finds Claude and Codex logs. The row-driven loop (`writeDigests`) only digests `row.Sessions` and `row.ConflictSessions`; design conversation ids (from `conversations/` records with `ticket == project:epic`) are not on any row. |
| Deep read (skill steps 3-4) | Mechanism yes, input no | One subagent per ticket and its digest paths works on any text digests. Its selection rule (severity from run-log flags, the `code-review` ticket, conflict sessions, verify parts) has no data for a map. |
| Consultant triage, follow-up filing (skill steps 5-6), Result sections (step 7), close-out (step 8) | Yes | Generic to the findings; they depend on the table only through step 4's "not read: past cap" rows. |
| `deep-read-cap` config | Yes | One global integer. |
| Inputs table, flags and severity | No | Every flag derives from run-log events, `session_ids`, per-ticket metrics, fix children, check records or the epic branch trailers (sections 1.2-1.4); a map has none of these (section 1.7). |
| Check rows (`slower`, `flake`) and launch rows | No | They need `verify` tickets' branches, check records and `launch-timed` events; none exist for a map. |
| Completion hook (`submitRetro`) | No | Excludes maps by `e.IsMap`, and map completion does not reach it. |
| `evolve all` | No | Excludes maps; its window uses `completed_at`, which a map does get (`--completed-at`). |

## 5. Gaps and unconfirmed items

- No test or code path was run against a map for `gx evolve inputs`; behavior on a map is read from code only.
- Whether `design_step` moves or step changes are logged anywhere: not found, not exhaustively ruled out.
- Whether any commit or check record is keyed by the `<map>-design` branch for the `merged` and check rows: not verified.
- How a Huh? on an agent question will be recorded: ticket 15 and ticket 11 are open; today's design-card Huh? exists only in the prompt text sent to the session.
- Whether the `friction` epic field (`EpicServerStamped` includes `friction`, `ticketops.StampFriction`) is stamped for map tickets: not examined.
- No `docs/specs/agent-launch-and-recovery.md` exists yet (tickets 15 and 17 are still open); there is no spec to cross-check.
