# Codex's question tool (`request_user_input`) in a plain chat

Ticket: `gx:regular-questions/02`. Installed Codex: `codex-cli 0.166.0` (verified-by-run). Source read at `openai/codex` `main` and tag `rust-v0.166.0`.

Claim tags: **[run]** verified by a real run, **[source]** read in the Codex or gx source, **[docs]** from docs, **[open]** not confirmed.

## Summary

- The tool works in the interactive TUI gx launches, but only when gx turns on the feature `default_mode_request_user_input`. Without it the tool exists only in Plan mode, which gx never starts.
- The full question (ids, headers, texts, options with descriptions) is in the rollout file while the question waits, as an open `function_call` with no `function_call_output` yet. gx's existing in-flight reader already sees that call.
- An answer goes back as keystrokes through `POST /v1/sessions/keys`. The call closes when the rollout gains the `function_call_output` with the same `call_id`.
- Lifting the guard for plain chats means two launch changes (no guard hook, plus the feature flag) and dropping the whole GX system prompt for plain Codex chats.
- Recommended: use the real tool with the feature flag, and keep printed ❓ rounds as the fallback if the flag ever disappears.

## 1. Does it work in the interactive TUI?

Gating, in order:

1. The tool is registered with the modes where it may run: `request_user_input_available_modes(features)` keeps a mode when `mode.allows_request_user_input()` (Plan only) or when `Feature::DefaultModeRequestUserInput` is on and the mode is Default. [source: `codex-rs/tools/src/tool_config.rs`, `codex-rs/protocol/src/config_types.rs`]
2. The handler refuses a call outside those modes with "request_user_input is unavailable in Default mode". [source: `codex-rs/core/src/tools/handlers/request_user_input_spec.rs`]
3. The feature is `default_mode_request_user_input`, stage "under development", default off. [source: `codex-rs/features/src/lib.rs`; run: `codex features list` on 0.166.0 prints `default_mode_request_user_input  under development  false`]
4. Every real rollout under `~/.codex/sessions` (48 sessions with the tool advertised) says "This tool is only available in Plan mode", and none holds a call. [run: grep over rollouts]
5. The old `collaboration_modes` feature is "removed ... true", so there is no collaboration-mode flag to set. [run: `codex features list`]

Real run (fixture-free, one scratch dir, one tmux pane, killed after): `codex -c features.default_mode_request_user_input=true '<prompt asking for a request_user_input call>'` in Default mode. The model called the tool and the TUI showed the overlay. [run]

```
  Question 1/1 (1 unanswered)
  Which color?

  › 1. Red                Choose red.
    2. Blue               Choose blue.
    3. None of the above  Optionally, add details in notes (tab)

  tab to add notes | enter to submit answer | esc to interrupt
```

- The overlay has one block per question, `Question i/n (k unanswered)`, each option's label plus description, and an automatic "None of the above" row. The overlay takes over the bottom pane. [run; source: `codex-rs/tui/src/bottom_pane/request_user_input/mod.rs`]
- In Default mode `is_blocking` is false and `auto_resolution_ms` is unset, so no countdown shows. In Plan mode the call is blocking. [source: `request_user_input.rs` handler]
- Codex has no version gate on the tool. The flag exists in 0.166.0 and on main. It is "under development", so it can change or go away in any release. [source]

## 2. Where gx can read the question

- **Rollout file (best source).** While the question waits, the rollout holds a `response_item` of type `function_call`, `name: "request_user_input"`, with `arguments` a JSON string, and no `function_call_output` for that `call_id` yet. [run]

  ```
  {"type":"function_call","name":"request_user_input","call_id":"call_...",
   "arguments":"{\"questions\":[{\"id\":\"pick\",\"header\":\"Color\",\"question\":\"Which color?\",
     \"options\":[{\"label\":\"Red\",\"description\":\"Choose red.\"},{\"label\":\"Blue\",...}]}]}"}
  ```

  `codexsession.ReadInFlightTools` (`codexsession/inflight.go`) already lists open function calls by `call_id` for the current turn, so it finds this one. [source: gx]
- **Question shape.** Per question: `id` (snake_case), `header` (12 chars or fewer), `question` (one sentence), `options` (2 or 3 entries of `label` and `description`; the recommended one is first with "(Recommended)" in its label). 1 to 3 questions per call. Options are required and single-choice. The client adds the free-text "Other" row; the model must not. There is no multi-select. [source: `request_user_input_spec.rs`, `protocol/src/request_user_input.rs`]
- **The event itself is not persisted.** `EventMsg::RequestUserInput` is in the non-durable list, so only the `function_call` item carries the content. [source: `codex-rs/rollout/src/policy.rs`]
- **Hook state / OSC 7501.** The TUI reports `state=blocked:kind=question` through OSC 7501 while the overlay is open, and gx's `codexWatcher` already maps that to `BlockedQuestion`. The report carries no text, so the status tells gx a question is open but not what it says. [source: `codex-rs/tui/src/terminal_program_status.rs`, `harness/codexwatch.go`]
- **PreToolUse hook (not needed).** The `request_user_input` PreToolUse hook gets `tool_input` and could copy it to the agent dir, but the rollout already has it. [open: not run; the guard hook's payload shape is from gx's `guard-bash` reading `tool_input`]
- **Screen scraping** works but is the fragile fallback, not the source.
- **Timing.** The rollout line was on disk about 4 seconds after the prompt, and the overlay was already visible. [run]

## 3. Sending the answer and seeing it close

Keys in the overlay (option list focused). [source: `request_user_input/mod.rs`; run for Down, Tab, text, Enter]

| Intent | Keys |
|---|---|
| Pick option k (1-based) | the digit `k`: it selects, commits and advances or submits |
| Move the highlight | Down or `j`, Up or `k` |
| Pick the highlighted option | Enter |
| Add a note to an option | Tab, type the text, Enter |
| Free text (an answer no option fits) | move to the "None of the above" row, Enter (focuses the notes box), type, Enter |
| Next or previous question | Right or Left |
| Interrupt the turn | Esc |

- Run: Down, Tab, `because`, Enter produced `function_call_output.output = {"answers":{"pick":{"answers":["Blue","user_note: because"]}}}`. The answer for a question is the chosen label, then an optional `"user_note: <text>"` entry.
- With several questions, each digit or Enter answers the current question and advances. The last one submits. If one is unanswered, a "submit with unanswered?" confirm opens (Enter on its first row submits, Esc goes back). gx should answer every question, so it never sees the confirm. [source]
- All of this goes through the existing `POST /v1/sessions/keys`. The server must sequence the keys per question and send nothing for a question it has no answer for.
- **Closed.** Three signals, all observable: the rollout gains a `function_call_output` with the same `call_id` (answered), the OSC 7501 state leaves blocked (answered, or interrupted), and a turn abort appears in the rollout (Esc). [run for the first; source for the others]
- **Stale-answer guard.** Before sending keys, gx re-reads the in-flight call and compares its `call_id` with the one the card showed. A different or missing id means the question closed or changed, so the submit is refused. Keys are blind input, so this check is the only protection.
- **Terminal answer (Screen).** The same `function_call_output` appears, so the card clears on the same signal.

## 4. Lifting the guard for plain chats only

What every Codex launch carries today: [source: gx]

- `harness.CodexArgs` always installs the PreToolUse group for `request_user_input` running `gx agent guard-question` (`harness/codex.go`, `CodexHookArgs`).
- `codexHarness.Launch` always appends `-c developer_instructions=<GX system prompt>`; `Resume` does not (`harness/codex.go`).
- `CodexArgs` has two callers outside tests: `server/sessions/sessions.go` `launchArgs` (every server session: plain Agents chat, Design, Triage, Fix agent) and `ralphloop/agent/agent.go` `AgentArgs` (Queue agents, conflict agents; called from `server/sched/launch.go` and `ralphloop/conflict.go`). `gx codex doctor` also calls it.

How gx tells a plain chat at launch: `server/sessions.Meta` already separates them. A plain chat has `Flow == ""`, `Ticket == ""` and `FixOf == ""`. A Design or Triage session has `Flow` set (`IsStepped`). A Fix agent has `FixOf`. A ticket-linked session has `Ticket`. Queue agents never go through `launchArgs`; they use `AgentArgs`. [source: `server/sessions/sessiontypes.go`, `sessions.go`]

What the change takes:

1. `CodexArgsInput` gets an opt-in field (for example `AllowQuestions bool`, zero value = today's behaviour). When set, it omits the `request_user_input` hook group and adds `-c features.default_mode_request_user_input=true`. Only `launchArgs` sets it, and only for a plain Meta. Opt-in keeps every other caller (Queue, conflict, doctor, Design, Triage, Fix) blocked by default.
2. `launchArgs` output is reused by `Resume`, so the flag and the missing hook come back on resume with no extra work. A session whose Meta says plain keeps behaving the same after a crash or restart.
3. `Launch` needs a way to skip `developer_instructions` for plain chats (a `LaunchOptions` field). Plain Claude chats already get no GX system prompt (only loop launches call `WithGXSystemPrompt`), so a plain Codex chat then matches.
4. Existing tests that pin the exact hook list or `-c` args (`harness/codex_test.go`, `testutil/agentfake/codex/codexhooks_test.go`, the `fakeagent` Codex parser `parseCodexArgs`) change with it.

Wrong in the GX system prompt for a plain Codex chat: all of it, not one line. [source: `harness/systemprompt.go`]

- "Never use request_user_input" is the opposite of what a plain chat now wants.
- Every step is the `gx tickets ask` flow: commit what is green, run `gx tickets ask <addr>`, write a `## Handoff`, end the turn. A plain chat has no ticket address, so those instructions cannot be followed.
- "Never end a turn with a question in plain text and no `ask` call" bans exactly the printed-❓ fallback. The prompt is also mostly about commits and handoffs, which a plain chat does not owe.
- So plain chats get no GX system prompt at all. If the fallback in section 5 is ever needed, a short ❓ instruction replaces it.

The hook trust note: `--dangerously-bypass-hook-trust` is already passed, so dropping one hook group does not need a hook review. [source: `harness/codex.go`]

## 5. If the tool cannot be used

Not needed today, but this is the fallback if the flag is removed. A plain Codex chat prints a ❓ Grilling round as text. The server already parses printed rounds for every session (`server/sessions/chat/round.go`, `grillround/`), so the card path exists. It needs a short developer instruction naming the round format, because the GX prompt is dropped. [source: gx; ticket notes] Trade-off: printed rounds are free text the model must format correctly, with no structured options, no stale-answer check and a prompt reply instead of key input. The tool is structured, so it is the better primary path.

## Recommended approach

1. Use the real `request_user_input` tool in plain Codex chats, enabled with `-c features.default_mode_request_user_input=true`. Do not switch Codex into Plan mode: that changes the agent's behaviour to planning.
2. Launch decides by `Meta` (`Flow`, `Ticket`, `FixOf` all empty), through an opt-in `CodexArgsInput` field that drops the guard hook group and sets the flag; Queue, conflict and doctor callers stay blocked by default.
3. Plain Codex chats get no GX system prompt, matching plain Claude chats.
4. Read the question from the rollout's open `function_call` (`request_user_input`, by `call_id`), built on `codexsession.ReadInFlightTools`; use OSC 7501 `kind=question` only as the "a question is open" signal.
5. Answer with keys through `POST /v1/sessions/keys`: a digit per option question; Down to the "None of the above" row, Enter, text, Enter for free text; Tab for a note on an option. Multi-question calls are answered in order.
6. Refuse a stale submit by re-reading the in-flight `call_id` before sending keys.
7. Clear the card when the rollout gains the matching `function_call_output`, the blocked state ends, or the turn aborts; this also covers an answer typed on the Screen.
8. Keep printed ❓ rounds as the documented fallback if the feature flag is removed.

Trade-off: the feature is "under development", so a Codex upgrade can break it. That is a version-pinned smoke check (the doctor run below), not a reason to build on a worse path.

## Open facts

- **Flag stability.** The flag is under development; whether it stays or becomes default-on is unknown. A `gx codex doctor` probe that asks for a question and reads the rollout would catch a change.
- **Startup warnings.** The probe pane showed "5 warnings". Whether enabling an under-development feature adds one, and whether it blocks gx's startup-dialog detection, is not checked.
- **Free-text keys, other cases.** Only the Down, Tab, text, Enter path ran. The digit shortcut, the "None of the above" Enter path and the multi-question confirm are from source, not run.
- **Rollout timing.** The `function_call` was on disk before the overlay was visible in the one run. That ordering is not guaranteed for every model or load.
- **Interrupt.** The exact rollout event written when the person presses Esc in the overlay is not seen (the source has a TODO about persisting interrupted results).
- **Probe side effect.** The probe made Codex save a trust entry for its scratch directory in `~/.codex/config.toml` (`[projects."...scratchpad/probe"]`); deleting that block is left to the person (the edit was denied in this session).
