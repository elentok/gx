# Claude Code model switching (ticket gx:design-models/01)

Tested on Claude Code 2.1.312. Sources: M = https://code.claude.com/docs/en/model-config,
S = https://code.claude.com/docs/en/sub-agents, K = https://code.claude.com/docs/en/skills,
C = https://code.claude.com/docs/en/commands, H = `claude --help`, T = local test.

## 1. Switching in a running session

- `/model <alias|name>` switches immediately; `/model` alone opens a picker. [M, "Setting your model"]
- Typing `/model <name>` saves the choice as your default for new sessions (writes the `model` field in user settings). In the picker, `s` switches for this session only. [M; C `/model` row]
- gx can drive it: `/model` is an ordinary slash command, so typing `/model sonnet` + Enter into the PTY should work. Docs describe no separate external control channel. Caveat: it persists to user settings, so it changes the user's default. The picker's `s` key avoids that but needs interactive keys. [M, C]
- If sent mid-response, the command is queued until the turn ends, unless Claude Code applies it without waiting (a confirm prompt may appear, see prompt-caching "Switching models"). Before v2.1.242 this depended on a feature flag. [C `/model` row]
- `/model <name>` also works in `-p` mode (v2.1.205+), session-only, not saved. [C]
- A `/model` switch also reaches subagents that inherit the main model. [M, "Subagent model configuration"]
- Survives `/clear`? NOT stated in any doc page read. `/clear` is "Start a new conversation with empty context" [C]. The session override has top precedence [M] and `/model` also writes the saved default, so the model most likely survives. UNVERIFIED: no interactive `/model` + `/clear` test was run, because typing `/model` would overwrite the user's saved default.

## 2. Startup flags and effort

- `--model <model>` accepts an alias (`sonnet`, `opus`, `haiku`, `fable`, `default`, `opusplan`) or a full name such as `claude-opus-5-5`. [H; M "Model aliases"]
- T: `claude --model opus -p "reply only your model id"` returned `claude-opus-5-5`. `--model sonnet --effort low` ran `claude-sonnet-5-5` (see `modelUsage` in `--output-format json`).
- `--model` and `ANTHROPIC_MODEL` apply only to the launched session. Precedence: `/model` > `--model` > `ANTHROPIC_MODEL` > settings `model` > `ANTHROPIC_DEFAULT_MODEL`. [M]
- Effort flag exists: `--effort <level>` with `low, medium, high, xhigh, max`. [H] Also `/effort <level|auto|status>` in session (works in `-p`), env `CLAUDE_CODE_EFFORT_LEVEL`, settings `effortLevel` (no `max`) and per-model `modelSettings`. Order: env / `--effort` / `/effort` > settings > model default. `max` is session-only. [M "Adjust effort level"; C `/effort` row]
- Available levels depend on the model. [K, `effort` row]

## 3. Subagents and skills

- An Agent tool call can pass a per-call `model`, alias only (`sonnet`, `opus`, `haiku`, `fable`); no full ID, no `inherit`. [S "Choose a model"]
- A subagent definition (`.claude/agents/*.md`) pins with `model:` frontmatter: alias, full ID, or `inherit`. `effort:` works in skill or subagent files. [S "Frontmatter reference"; M]
- Resolution order (v2.1.251+): per-call `model` > definition frontmatter > `CLAUDE_CODE_SUBAGENT_MODEL` > main model. `CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1` (v2.1.257+) forces one model on all. [S]
- Skill frontmatter `model:` applies for the rest of the current turn only, is not saved, and the session model resumes on the next prompt. With `context: fork` it sets the forked subagent's model (agent type via `agent:`). [K "model", "Run skills in a subagent"]
- Skill `effort:` overrides session effort while the skill is active; duration not specified. [K]

## Gaps

- `/clear` persistence (above).
- The last ~15-30 KB of the model-config, sub-agents and skills pages was not read by the fetch tool.
- Docs were read through a summarizing fetch tool, not raw HTML; some quotes are paraphrased.
