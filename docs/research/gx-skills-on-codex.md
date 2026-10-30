# gx skills on Codex (gx:codex-support/02)

Checked 2026-10-30 against codex-cli 0.147.0. Source read at tag `rust-v0.147.0` of openai/codex (`codex-rs/`), and `rust-v0.166.0` where noted.

Labels used below.

- Observed means I ran it against real Codex.
- Source means I read it in the Codex source and did not run it.
- Untested means neither.

## How the tests were run

- All runs used a throwaway git repo, a temp `HOME` and a temp `CODEX_HOME` (auth copied, nothing printed), a temp gx ticket store, and a gx built from `main`. No real tracker, no `~/.codex` writes, no herdr.
- `codex exec --json` for prompt-level checks, and a private `tmux -L` server for TUI-only checks. Model `gpt-5.6-luna`, effort `low`. One TUI probe used `gpt-5.6-sol`.
- `codex debug prompt-input` prints the model-visible prompt without a model call. It was the main tool for discovery tests.
- Late in the run Codex answered `429 Too Many Requests`. I stopped spending quota then. The checks that were cut are in "Untested".
- Two runs of mine hit a different Codex version than 0.147.0 (see "Incident" at the end). I repeated the ones that mattered on a private 0.147.0 copy.

## 1. Discovery and invocation

### Is `~/.codex/prompts` still right?

No.

- Observed. With the gx bundle installed only under `<home>/.codex/prompts`, the skill catalog in `codex debug prompt-input` lists none of the gx skills. `/prompts:gx` in the TUI answers `Unrecognized command`.
- Source. `rust-v0.147.0` has no custom-prompt loader. The slash-command list (`tui/src/slash_command.rs`) has `Skills` and no prompts command.
- Observed. The same files copied to `<home>/.agents/skills` are all listed (24 of 26 skills, see the hidden pair below).
- Source. `ext/skills/src/host_roots.rs` reads, for the user scope, `$CODEX_HOME/skills` (the code calls it deprecated, kept for compatibility), `~/.agents/skills`, and for a repo `.codex/skills` and `<dir>/.agents/skills` from the cwd up to the repo root. `~/.agents/skills` comes from the real home directory, not from `CODEX_HOME`. `rust-v0.166.0` has the same roots.
- Observed. If the same skills sit in both `~/.agents/skills` and `$CODEX_HOME/skills`, every skill is listed twice (48 entries for 24 skills). This machine's `~/.codex/skills` is a symlink to the dotfiles skill tree, which also holds gx-* skills. After a move to `~/.agents/skills` those would double.
- Observed. A skill whose `SKILL.md` is a symlink to a file is not discovered. A symlink to a whole skill directory is. `gx skills install --dev` links file by file, so dev mode shows no skills in Codex.

The right install folder is `~/.agents/skills`.

### How a person and gx invoke a skill

- Observed. `/gx-design foo` in the TUI prints `Unrecognized command '/gx-design'` and leaves the text in the composer. Nothing reaches the model.
- Observed. `$gx-unslop ...` typed in the TUI, and in `codex exec`, makes Codex add a `<skill><name>..</name><path>..</path>` message with the full `SKILL.md`. The `$name` can sit anywhere in the text, also after a leading newline and mid-sentence.
- Observed. Typing `$gx-unslop` with nothing after it opens a picker, and the first Enter only inserts the mention. It does not submit. A prompt with arguments (`$gx-implement <addr>`) has a space, so no picker is open and Enter submits. The no-argument form needs a second Enter. I did not press the second Enter.
- Source. `mentions_v2` is on, so the picker key is `@`. `$` still works typed.
- Observed. A skill hidden from implicit use (below) still runs with an explicit `$gx-wayfinder`.

gx already types `$skill` for Codex in one place, `ralphloop.skillPrompt` (used by `server/launch.go`). Every other place types `/gx-...` for all harnesses, so each would stall on Codex.

### Implicit invocation

- Observed. Codex ignores `disable-model-invocation: true`. It lists gx-design, gx-implement, gx-merge, gx-cleanup, gx-bump, gx-evolve, gx-one-off, gx-code-review, gx-triage-follow-ups, gx-verify-epic and gx-thermo-nuclear-code-quality-review in the catalog, so the model may start them on its own.
- Observed. Codex hides a skill from the catalog when `agents/openai.yaml` has `policy.allow_implicit_invocation: false`. gx-wayfinder and gx-to-spec have that file and are missing from the catalog. These two are the only flagged skills that have it.
- `skills/gx.md` says Codex "has no equivalent auto-invocation concept". That is wrong for skills.

## 2. Claude-only tools and habits, by skill

What Codex offers instead (Observed tool list in a TUI session, plus Source).

| Claude | Codex |
| --- | --- |
| `Read` tool | None. The model reads with shell commands (`sed -n`, `rg`, `cat`). Images through `view_image`. |
| `Edit`, `Write` | `apply_patch` (Observed in a run: a `file_change` item). |
| `Bash` with `timeout` | `exec_command` / `write_stdin`. A long command keeps running as a background terminal. |
| `Agent` tool | `spawn_agent` plus `wait_agent` (see section 3). |
| Skill tool | None. Skills are `$name` mentions, or the model reads the `SKILL.md` path. |
| `AskUserQuestion` | `request_user_input`, available only in Plan mode. Observed in a 0.166.0 TUI run and reported by sol on 0.147.0, "unavailable in Default mode". |
| `ToolSearch`, `TaskStop` | None. `/ps` and `/stop` are person commands. |
| `run_in_background` | None. Source only. Not tested. |
| `/clear` | `/clear` and `/new` both start a new chat (Source, descriptions). gx types `/new`. |
| Plan mode tools | `/plan` switches mode. Skills do not use it. |

Per skill (line numbers are in `main`).

- gx-implement. Lines 53-55, 69-72, 90-91 say to use `Read`, `Edit` and `Write` and never read through a shell. On Codex the shell is the only reader, so "Read files with the `Read` tool, not a shell command" contradicts the harness. Lines 76-77 name `TaskStop` and `ToolSearch select:TaskStop`. Line 79 says "Bash call". Line 215 says Bash `timeout: 600000`. Line 26 says "Skill tool" and "explicit `/command`". Line 159 says PNG `Read`.
- gx-verify (19), gx-merge (211), gx-resolving-merge-conflicts (16, 35, 38-39), gx-tdd (86), gx-verify-epic (103, 173-184), gx-ui-check (31-32, 90, MOCK-CHECK 19, 30). The same `Read`, `Edit`, Bash `timeout`, `TaskStop` and `run_in_background` wording.
- gx-code-review (24-25, 59, 79, 90-91, 125). `Edit`/`Write`, the Skill tool, "one message carrying every `Agent` tool call", `general-purpose`, "foreground", and "the `Agent` call's model".
- gx-triage-follow-ups (35), gx-evolve (44), gx-cleanup (78-105), gx-bump (37), gx-changelog (25). `Agent` calls. gx-cleanup line 105 uses `isolation: "worktree"`, a Claude-only argument. `spawn_agent` has no such field.
- gx-design (43-46, 89, 205, 232, 257). "Never call the Skill tool", "Claude may ask for permission", `/clear`, and a run-end example that tells the person to run `/gx-design`.
- gx-follow-up (43). Already says "AskUserQuestion on Claude, in plain text on Codex". Codex has no question tool in Default mode, so plain text is right.
- gx-local-tracker.md (157, 160, 333, 376). "Write tool", "guard-bash hook", and the system-prompt line "Never use the AskUserQuestion tool".
- gx-investigate (55-57, 79). `~/.claude/projects/...` transcripts and `gx claude session-path`. There is no Codex counterpart. Codex rollouts are at `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl` (Observed).
- gx-grilling (104), gx-wayfinder (77, 115). Plain "sub-agent" wording. Fine.

What worked unchanged.

- Observed. `$gx-implement toy:demo-impl/01` in a linked worktree. The Codex agent ran `gx tickets validate`, `gx tickets show`, edited with `apply_patch`, ran `gx check`, wrote `## Result` with `gx tickets section`, and set `--iteration-status finished --commitless true`. It first guessed a `gx tickets claim` command that does not exist (the skill's description says "claim and implement"). It used about 410K input tokens (373K cached) for a one-line ticket on luna, mostly because the skill and the tracker doc are long.
- Observed. `$gx-design toy:greet` on a throwaway map: it read `gx-local-tracker.md` from the absolute path, ran `gx agent link` (printed the "server not running" note) and `gx agent step chart`, loaded gx-wayfinder, gx-grilling, gx-domain-modeling and gx-unslop by path, and ended the turn with a `❓ **Q1**` round and a `gx-question` block as the last text. The relative `../` links resolve because the model is told each skill's path. It did not run `gx tickets set --design-step chart` before stopping. I treat that as model quality on `low`, not a Codex limit.
- Observed. A `gx-grilling` round written by Codex has 2 segments and 2 fenced blocks under `grillround.Segments` and `SplitFences`, and `HasMarker` is true. The run-end regex matches the gx-design example line.

## 3. Subagents and the tier rule

- Observed. In the TUI on luna (multi-agent v1) the tool is `multi_agent_v1__spawn_agent`. It takes `message`, `items`, `fork_context`, `model`, `reasoning_effort`, `service_tier` (reported by the model). A spawn with `model="gpt-5.6-luna"` and `reasoning_effort="medium"` showed `Spawned ... (gpt-5.6-luna medium)`, so the override took effect. The default spawn showed the session's `(gpt-5.6-luna low)`.
- Observed. On sol (multi-agent v2) the tool is `collaboration.spawn_agent` with `task_name`, `message`, `fork_turns`, `model`, `reasoning_effort` (reported by the model).
- Observed. `codex exec` showed no spawn tool at all on luna. The TUI had one. gx runs the TUI, so this only matters for test scripts.
- Source. The `model` and `reasoning_effort` fields are on by default. A config can hide them (`features.multi_agent_v2.expose_spawn_agent_model_overrides = false`, or the v1 hide flag). Without them the child inherits the parent's model and effort. The tracker's rule "if the spawn call has no model field, the subagent runs on the session model" is therefore right as a fallback.
- Source. For v2 the injected hint says to set `model` or `reasoning_effort` only when the user, AGENTS.md or "skill instructions" ask for it. It also says a full-history fork inherits and "does not accept overrides", so set `fork_turns` to `none` or a number when overriding. A skill line that asks for a tier model counts as the explicit request. Whether a full-history fork rejects the override at run time is Untested.
- Source. Limits. v1 models allow 6 concurrent subagents and depth 1 (a subagent cannot spawn). v2 allows 4 threads in the session, so 3 children. Skills that say "about 8 in parallel" (gx-triage-follow-ups) or "one per picked ticket" (gx-evolve) will hit the cap.
- Source. v2 sessions get "Do not spawn sub-agents unless the user or applicable AGENTS.md/skill instructions explicitly ask". The skills do say "dispatch a subagent", so they qualify. Keep those words.

The tier rule works in part. `gx agent model <tier>` prints the model only (`cmd/agent_model.go`). The tier also has an effort, and Codex lets the child take its own effort. Without it the child inherits the parent's effort, which may be invalid for the tier's model or just wrong. The clean fix is a way to print the tier's effort and a rule to pass it as `reasoning_effort` on Codex. Claude's `Agent` tool has no effort field, so Claude stays model-only.

`gx agent model` finds the harness from `GX_AGENT_SESSION`, then `GX_AGENT`, then `claude`. Both variables reach the Codex shell (Observed, with `GX_AGENT_SESSION` and `GX_AGENT` exported).

## 4. gx-grilling rounds on a Codex session

- Observed. A Codex model follows the round format (see section 2).
- Source. The server cannot parse rounds from a Codex session today. `server/sessionactionable.go` reads `claudeTranscript`, which returns nothing for Codex (the comment says so). `grillround.Recount` and `grillround.Count` take Claude `ChatMessage`s from `transcript.ReadChat`. The Codex code only has `ExportMarkdownIn` and `LastAssistantTextIn` (`codexsession/export.go`), neither of which gives per-turn messages.
- Observed. A Codex rollout has `response_item` assistant messages with `phase` (`commentary` or `final_answer`) and `event_msg` `agent_message` items. The rule "questions are the last thing in the turn" maps to the `final_answer` message.
- Server change. A Codex rollout reader that returns `ChatMessage`s (person text, assistant text with phase, tool calls) so that rounds, Actionable, run-end lines and the recovery read work. That belongs to gx:codex-support/04.
- Skill change. None for the format itself.

## 5. `gx agent context-window --fits` and `gx agent link` inside Codex

- Observed. The Codex shell has `CODEX_THREAD_ID`, `CODEX_SESSION_ID`, `GX_AGENT_SESSION` and `GX_AGENT`. `context-window` reads `CODEX_THREAD_ID`.
- Observed bug. From the Codex shell `gx agent context-window` failed with `no codex session <id> found for <cwd>`. The shell's cwd was `/var/folders/...`. The rollout stores `/private/var/folders/...`. `codexsession.readIdentity` compares the two strings with `==`. With `--cwd "$(pwd -P)"` it returned a count (14754, then 16155 and 16655 on 0.147.0). Any worktree path that contains a symlink hits this. `--fits` then fails closed, so every design chain ends after one step. `tickets/store.go` already has `canonicalPath` (EvalSymlinks) for the same problem. This is a Go fix in `codexsession`, not a skill edit.
- Observed. On the very first tool call of a session, `context-window` says `codex session <id> has no context usage yet`. After one finished model call it returns a count. Design chains run after real work, so this rarely matters. `gx-implement` treats an error as over budget, which would fork at once if it checked at the very start.
- Observed. `CLAUDE_CODE_SESSION_ID` leaked from my own Claude Code shell into the Codex shell, and `context-window` then said `ambiguous agent session`. The Codex launch should not pass Claude variables. When `GX_AGENT_SESSION` is known, `context-window` should ask the server for the harness, as `gx agent model` does.
- Observed. `gx agent link` and `gx agent step` inside Codex print the "server not running, nothing linked" note. Source. They use only `GX_AGENT_SESSION` and a server call that does not look at the harness (`server/sessionlink.go`). With a live server it should work. Untested with a live server.
- Ceiling. `SmartZoneCeiling` is 130K for every model. Codex's window here is 258K (from `task_started`). Whether 130K is right for Codex is a design question for the spec.

## 6. Hooks

- Observed. Codex has hooks (`hooks` is on). A `PreToolUse` hook with matcher `Bash` in `$CODEX_HOME/hooks.json`, command `gx agent guard-bash`, blocks `cat > note.txt <<EOF ...`. Codex shows `Command blocked by PreToolUse hook: <gx message>`. The hook JSON (`tool_input.command`) and exit code 2 are the same as Claude's, so the command works unchanged.
- Observed. Until trusted, the hook is skipped silently in `codex exec`, and the TUI opens a startup dialog "Hooks need review" with "Trust all and continue". An unattended session would stop on that dialog. Trust is stored in `config.toml` as `[hooks.state."<path>/hooks.json:pre_tool_use:0:0"] trusted_hash = "sha256:..."`. gx can read the hash back after a trusted run, but I did not find a way to compute it. gx:codex-support/05 owns this.
- Observed. The guard's message says "Use the Edit or Write tool instead". Codex has `apply_patch`. The text must name the right tool for the harness.
- Other hooks the skills rely on. None. `--disallowedTools AskUserQuestion,EnterPlanMode,ExitPlanMode` has no counterpart in the Codex flags I read in `codex --help`. On Codex `request_user_input` and Plan mode exist only after a person switches to Plan mode.
- Observed. Under `--sandbox workspace-write` in a normal checkout, `git add` fails with `Unable to create '.../.git/index.lock': Operation not permitted`. `--add-dir <repo>/.git` fixes it. In a linked worktree of a bare repo (the gx layout) `git add` and `git commit` worked with no extra flag. So the commit step works for ticket worktrees and design worktrees, and fails in a plain trunk checkout. That is a launch concern for gx:codex-support/05.
- Writes outside the workspace roots were not tested (see Untested).

## 7. The system prompt

Claude gets the "announce and stop" text through `--append-system-prompt-file`. Codex has no gx system prompt today.

- Observed. `-c developer_instructions="..."` adds a developer-role message at the start of the prompt (`codex debug prompt-input`). It is the Codex counterpart.
- The Codex run of `$gx-implement` and `$gx-design` found the same rule because the skills read `gx-local-tracker.md`, but a ticket agent that never reads it would not.

## Skill and bundle edits

Packaging and code (needed for skills to load at all).

1. Change `skills.CodexSkillsRoot` to `<home>/.agents/skills`. Update its comment and `skills/gx.md`. Note that the path ignores `CODEX_HOME`.
2. On `gx skills install`, remove the managed copies under `~/.codex/prompts` through the manifest, and warn when `$CODEX_HOME/skills` also holds gx-* skills (duplicates).
3. In `--dev` mode, link whole skill directories for the Codex root. Keep file links for the root-level files (`gx.md`, `gx-local-tracker.md`, `gx-failing-tests.md`).
4. Add `agents/openai.yaml` with `policy.allow_implicit_invocation: false` to each skill that sets `disable-model-invocation: true`. Missing today for gx-bump, gx-cleanup, gx-code-review, gx-design, gx-evolve, gx-implement, gx-merge, gx-one-off, gx-thermo-nuclear-code-quality-review, gx-triage-follow-ups and gx-verify-epic. Add a bundle test that pairs the flag with the file in both directions. Correct the "Invocation policy" paragraph in `skills/gx.md`.
5. Use `ralphloop.SkillPrompt(agent, skill, arg)` at every place that types `/gx-...`. Found in `server/designs.go` (two sites), `server/sessionactionable.go` (`designContinueAction`), `server/sessionhandoff.go`, `ui/tickets/start_session.go`, `server/fixagent.go`, and `ui/tickets/suggested_actions.go`. `ralphloop/iteration.go` already uses `skillPrompt`, but its no-argument merge-conflict prompt (`$gx-resolving-merge-conflicts`) opens the picker and needs a second Enter.
6. `gx agent model <tier> --effort` (or a second line) prints the tier's effort.
7. Fix the symlinked-cwd mismatch in `codexsession` (compare after EvalSymlinks).
8. Make the guard-bash hint name the harness's edit tool (`apply_patch` on Codex). Deliver the hook to Codex and pre-trust it (ticket 05).
9. Send the announce-and-stop text to Codex with `-c developer_instructions=...`. Update `GXSystemPrompt`, the copy in `gx-local-tracker.md`, and the test that keeps them equal.
10. Add a harness-neutral session lookup for `gx-investigate` (today `gx claude session-path` is Claude-only), or document the rollout path.

Skill text.

11. Add one shared "Tool names" table to `gx-local-tracker.md` (Claude column, Codex column, as in section 2). Every skill then says "file edit", "file read", "shell call" and "start a subagent", and points to the table. This is the clean change. Per-skill "(Claude) / (Codex)" pairs would drift.
12. gx-implement. Split the read/edit rules into the harness-neutral part (read each file once and whole, never edit unread code, one check per shell call, no stdin readers) and a pointer to the table. Drop "Read tool, not a shell command" for Codex. Replace the `TaskStop`/`ToolSearch` lines with "stop every background process you started" plus the table's per-harness way. Replace the Bash `timeout: 600000` note in gx-implement and gx-verify with the table entry for long foreground commands. Replace "explicit `/command`" with "explicit invocation (`/name` on Claude, `$name` on Codex)". Fix the "claim and implement" description so a model does not look for a claim command.
13. gx-code-review, gx-triage-follow-ups, gx-evolve, gx-cleanup, gx-bump, gx-changelog. Say "start every subagent in one turn, then wait for all of them" instead of `Agent` tool and "foreground". Pass the tier model, and on Codex the tier effort, and set `fork_turns` to `none` for a fresh child. State the parallel cap (Claude no fixed cap, Codex 3 to 6) and tell the skill to start the next as each finishes. Remove `isolation: "worktree"` from gx-cleanup and say to create the child's own worktree with `git worktree add`.
14. gx-code-review and gx-merge lines about the Skill tool. Say "read the callee's `SKILL.md` by path". The Codex reason (the catalog hides it) now comes from the yaml policy, not a tool refusal.
15. gx-design. Replace "Never call the Skill tool" with the same "read by path" wording. Make the run-end example name both forms (`/gx-design` on Claude, `$gx-design` on Codex). Replace "Claude may ask for permission" with the harness-neutral point that edits outside the start folder can need approval. Keep `/clear` or `/new`.
16. gx-ui-check, gx-verify-epic, gx-resolving-merge-conflicts, gx-merge. Replace `run_in_background` and `TaskStop` with the table entry. Replace PNG `Read` with "open the image" (`view_image` on Codex) in MOCK-CHECK.md and gx-implement line 159.
17. gx-local-tracker.md. Change "Never use the AskUserQuestion tool" to "Never use an interactive question tool (`AskUserQuestion` on Claude, `request_user_input` on Codex)". Change "Write tool" to "file-writing tool". Update the tier convention to pass the effort too and to keep the no-field fallback.
18. gx-investigate. Add the Codex transcript path and the lookup from edit 10.

## Gaps no skill edit can close

- Rounds, Actionable, run-end lines and recovery do not work on a Codex session until the server reads Codex rollouts (ticket 04).
- Plan mode and `request_user_input` cannot be disabled by flag. Skills can only tell the agent not to use them.
- `--fits` cannot be right until the symlink bug (edit 7) and the Claude-variable leak are fixed. The 130K ceiling is a number, not a skill matter.
- The hook trust dialog, hook delivery and sandbox write roots belong to ticket 05. Commit in a plain checkout fails without `--add-dir <repo>/.git`.
- Codex shells have no `run_in_background` flag and no way for a skill to list or stop background terminals. gx's background-task gate reads Claude transcripts. A Codex equivalent signal is for ticket 01.
- `codex exec` has no spawn tool, so scripted checks of subagent skills need the TUI.
- A weak model (luna, low) makes small mistakes (the invented `claim` command, skipping `--design-step`). Skills cannot fix model quality.

## Untested

- `gx agent link` and `gx agent step` with a live gx server, and `gx designs worktree` on Codex.
- A spawn with a bad model name, and whether a full-history fork rejects a `model` override on v2.
- Writes outside the workspace roots under `workspace-write`, and what the TUI shows as an approval request there. The test hit `429`.
- Background terminals (`exec_command` with a long command), `/ps` and `/stop`, and whether a finished turn with a live terminal blocks anything.
- The second Enter after the `$name` picker.
- Behaviour on 0.166.0 beyond the source read and the `request_user_input` message. The installed global Codex is now 0.166.0.
- A full `gx-design` chain, review and spec steps, and a code-review or merge run on Codex.
- `gx skills install` writing to `~/.agents/skills` (I copied files by hand).

## Incident

- My first TUI probe of `/prompts` pressed Enter on a "Update now" dialog and ran `npm install -g @openai/codex`, which moved the global install to 0.166.0. I reinstalled 0.147.0 at once. Around 10:29 the global package changed to 0.166.0 again. I did not run that install and do not know what did. I left the global install alone after that and used a private 0.147.0 copy for the rest. The global install is 0.166.0 as I finish.
- Runs after that point used a temp config with `check_for_update_on_startup = false`. Any session that starts a TUI with the default config can hit that dialog.
- Unrelated: one `pkill -f "codex exec --json --skip-git-repo-check"` of mine could have hit another agent's matching process.
