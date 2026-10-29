# Codex model selection and switching (gx:design-models/02)

Codex CLI 0.147.0, checked 2026-10-29. Source tree: openai/codex `main` at b75d1e8 (`codex-rs/`).
Docs URLs below are the redirect targets of the old `developers.openai.com/codex/...` pages.

Agents gx supports: only `claude` and `codex` (`config/agents.go:26-27`, `AgentsConfig`). This note
covers Codex only. Claude is not researched here.

## 1. Launch: flag, config key, effort

- Model flag: `-m, --model <MODEL>` (`codex --help`). Docs: "Override the model set in
  configuration" (learn.chatgpt.com/docs/developer-commands?surface=cli).
- Config key: `model` in `~/.codex/config.toml` (learn.chatgpt.com/docs/config-file/config-reference).
- Effort key: `model_reasoning_effort`, a string "advertised by the selected model, such as `low`,
  `medium`, `high`, `xhigh`, `max`, or `ultra`"; levels depend on model and client (config-reference).
- There is no dedicated effort flag. Use `-c model_reasoning_effort="low"` (`codex --help`: `-c,
  --config <key=value>`, value parsed as TOML). gx already does this: `ralphloop/loop.go:205-221`.
- `-p/--profile <name>` layers `$CODEX_HOME/<name>.config.toml` (`codex --help`). gx does not use it.
- Local test: `codex exec -m gpt-5.6-luna -c 'model_reasoning_effort="low"' 'Reply ok'` printed
  `model: gpt-5.6-luna` and `reasoning effort: low`, and answered. Both overrides take effect.
- Valid effort levels per model come from the model catalog, cached at `~/.codex/models_cache.json`
  (fetched from the server, client_version 0.147.0). On this machine: `gpt-5.6-sol` low..max plus
  `ultra`; `gpt-5.6-terra` low..max plus `ultra`; `gpt-5.6-luna` low..max (no `ultra`). Hidden:
  `gpt-reserve`, `codex-auto-review`. gx's default `medium` is valid for all of them.

## 2. Switching in a running session

- Yes. Slash command `/model`: "Choose the model for the current chat"
  (learn.chatgpt.com/docs/reference/slash-commands). `/reasoning`: "Choose the reasoning effort for
  the current chat". `/fast` toggles a Fast service tier.
- Typed without args, `/model` opens a picker popup (`tui/src/chatwidget/slash_dispatch.rs:377-380`,
  `open_model_popup`). Someone would need to navigate it.
- Typed with args, it is one line, no popup. `supports_inline_args()` includes `SlashCommand::Model`
  (`tui/src/slash_command.rs:172-177`). Syntax: `/model <slug> [--reasoning <level>] [--speed <tier>]`
  (`tui/src/chatwidget/model_command.rs:23-33`: flags `--reasoning`, `--speed`, at most one
  positional). It dispatches `SelectSessionModel`; the file header calls it "session-only model
  selection" (`model_command.rs:1`). It validates first and shows an error message on a bad model or
  effort (`model_command.rs:148-155`).
- So an external process that types `/model gpt-5.6-luna --reasoning low` + Enter into the pane
  should work. gx's Codex typing path already types into the composer (`harness/typing.go`, prompt
  marker `›`). NOT tested live: source read only, no TUI session driven.
- Caveats: session-only (not written to config.toml). Slash-command docs do not state persistence.
  Behavior while a turn is running is untested.

## 3. Subagents and per-call model

- Yes, subagents can run another model (learn.chatgpt.com/docs/agent-configuration/subagents).
- Per-call at spawn: the `spawn_agent` tool carries `model`, `reasoning_effort`, `service_tier`
  properties, but only when override exposure is on (`expose_spawn_agent_model_overrides`;
  `core/src/tools/handlers/multi_agents_spec_tests.rs:168-240` asserts they are present when
  exposed and absent when not). Config reference: "An explicit spawn model takes precedence"
  (`agents.default_subagent_model`). The subagents doc names no spawn parameter; it says to "request
  a specific model or reasoning effort in your prompt". The parent model picks the value, not an
  external caller.
- Config defaults: `agents.default_subagent_model`, `agents.default_subagent_reasoning_effort`
  (config-reference).
- Per-role: custom agent TOML files may set `model` and `model_reasoning_effort`; the file value
  wins over the resolution below (subagents doc).
- Resolution order: explicit spawn value, then `[agents]` default, then parent's value (subagents
  doc). With nothing set, the subagent inherits the parent's model and effort.

## 4. Default model and drift

- With no config, `codex --help` says the CLI "will use config default or show selection". gx sets
  its own: `gpt-5.6-sol` / `medium` (`config/agents.go:11-12`).
- The model catalog comes from the server, not the binary. `~/.codex/models_cache.json` has
  `fetched_at`, `etag`, and per-model `priority`/`visibility`/`upgrade`, so the set and order can
  change without a CLI update. Local cache fetched 2026-10-19.
- Models get retired. This machine's config records `[notice.model_migrations]`:
  `gpt-5.3-codex -> gpt-5.5`, `gpt-5.4 -> gpt-5.5` (`~/.codex/config.toml`), and the TUI has a
  migration prompt (`tui/src/model_migration.rs`). Docs examples also differ from the cache
  (`gpt-6.1-sol` in the docs, `gpt-5.6-sol` locally), so a pinned slug can go stale.
- For gx: validate a configured slug against `models_cache.json` or a cheap `codex exec -m <slug>`
  instead of a static list.

## Not verified

- Live `/model ... --reasoning ...` typed into a running TUI (source read only).
- `/model` typed mid-turn (queued, dropped, or rejected).
- Launch with an effort the model does not support.
