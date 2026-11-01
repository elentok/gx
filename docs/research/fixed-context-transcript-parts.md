# Fixed context: which transcript records give each part

Research for ticket gx:fixed-context-monitor/05. Sampled on 2026-11-01 from `~/.claude/projects` and `~/.codex/sessions`.

## Resolution

Sampled 109 ralph-loop sessions, one per epic (newest session of each `*-item-*` project directory), Claude Code 2.1.289 to 2.1.316. 89 are 2.1.301 or newer and hold the `api-request-shape` record. The other 20 (2.1.289 to 2.1.296) do not. Skills launched: gx-implement 51, gx-verify-epic 23, gx-investigate 17, gx-evolve 14, gx-code-review 2. Method and scripts: `docs/research/fixed-context-transcript-parts.md` on branch `research/transcript-parts`.

### Main finding

Seven of the eight parts are sized from records written before the first assistant record, and the remainder against the first-request usage is under 1%. The tools part is the exception: it exists only in the private `api-request-shape` record (Claude Code 2.1.301 and newer). Also, the "55 KB" skill listing in the map's Notes is the size of the JSONL record, which stores the text twice. The text the model sees is 25.4 KB, about 8.4K tokens, not 14K. The largest part is tools (about 23K tokens on 2.1.316), and one tool, Artifact, is about 14K of that.

### Part-to-record table

Tokens are bytes divided by 2.68 for tools and system prompt, and by 3.03 for everything else (see accuracy). "v316" is the median of the 8 sessions on 2.1.316. "All" is the median of the 89 sessions with the shape record (2.1.301 to 2.1.316).

| Part | Record and field | Bytes (v316) | Tokens (v316) | Tokens (all) |
| --- | --- | --- | --- | --- |
| System prompt | `api-request-shape` `.shape.system[].text`. Also `prompt_snapshot` attachment `.systemPrompt`, which is 1.005 times the same text and exists in every version. | 10.1K | 3.8K | 3.7K |
| Tools | `api-request-shape` `.shape.tools` (JSON). Only in 2.1.301 and newer. | 61.6K | 23.0K | 27.1K |
| Skill listing | `attachment` type `skill_listing`, field `.content` (the `rendered` field repeats it) | 25.4K | 8.4K | 8.4K |
| Deferred tools | `deferred_tools_delta`, `rendered` text. Names only, not schemas. | 1.4K | 0.5K | 0.5K |
| Agent listing | `agent_listing_delta`, `rendered` | 2.7K | 0.9K | 0.9K |
| MCP instructions | `mcp_instructions_delta`, `rendered` | 2.1K | 0.7K | 0.7K |
| CLAUDE.md files | `instructions` attachment, `.files[]` with `type` User, Project or AutoMem, or its `rendered` text | 3.3K | 1.1K | 1.2K |
| Launched skill and first prompt | The user record with `isMeta: true` whose text starts "Base directory for this skill" (no `sourceToolUseID` at launch), plus the `<command-name>` user record before it | 15.6K | 5.2K | 4.5K |
| Other attachments | `environment`, `model`, `output_style`, `output_style_instructions`, `session_context`, `date`, `remote_session_change` (`rendered`) | 6.0K | 2.0K | 1.9K |
| Remainder | first-request usage minus the sum above | | -0.2K | -0.3K |
| First-request total | first `assistant` record, `message.usage` input + cache_creation + cache_read | | 45.3K | 48.3K (range 43.1K to 51.1K) |

Notes on the table:

- Tools are the largest part: half of the fixed context on 2.1.316. The Artifact tool alone is 37.8 KB (about 14K tokens, 31% of the total) and is in every session. `SendFeedback` and `Workflow` (5.5 KB each) were in 2.1.314 and are gone in 2.1.316, which explains the drop from 49.3K to 45.3K.
- `command_permissions` and `credential_org` have no `rendered` text and are not in the request. `prompt_snapshot` duplicates the system prompt and is not extra.
- The skill_listing record is 54.5 KB because it holds `content` (25.3 KB), `rendered` (25.7 KB), `names` and `digests`. Sum the text once.
- The wire request is `api-request-blob` records (system extras and messages) plus the shape. They hold the same text as the attachments and are not needed.

### Remainder

- Sessions with the shape record: the remainder is -0.6% of the total (median -314 tokens, range -1,196 to +208). The two ratios were fitted on the same sessions, so this only shows the parts are complete: no part is missing.
- Sessions without it (2.1.289 to 2.1.296, 20 sessions): the attachment parts leave 27.5K tokens in the 2.1.289 to 2.1.295 implement sessions and 29.7K in 2.1.296 implement sessions. That remainder is system prompt plus tools. Check on the 89 shape sessions: the same subtraction equals (tools + system prompt) / 2.68 within 0.96 to 1.01 (median 0.99). So the remainder can be reported as "system prompt and tools, unsplit", and the system prompt can be taken out of it from `prompt_snapshot`.

### Estimate accuracy

- Total: bytes divided by 4 gives 29% too low (range -30% to -28%). Divided by 3, 6% too low. One constant of 2.82 bytes per token gives a median error of 0.3% and a maximum of 2.0%. Two constants (2.68 for tools and system prompt, 3.03 for the rest) give a median of 0.24% and a maximum of 1.9%.
- Why two: the first request has a cache boundary after the system prompt blocks (`cacheControl` on `system[1]`). In 19 distinct session shapes, `cache_read_input_tokens` matches tools plus the first two system blocks. That prefix is 2.67 to 2.73 bytes per token in 16 of them (2.92 in the other 3, where the cache hit was probably partial). The rest of the request is 3.03 (2.65 to 3.04).
- Skill text is denser: 2.6 bytes per token. Measured two ways: a regression of the total on launched-skill bytes within groups of identical sessions (2.3 to 2.9), and the occupancy jump after 43 single-tool `Skill` calls (median 2.6, range 1.9 to 2.7). Read output is 2.0 (median of 50 Reads, range 0.9 to 2.4), because `cat -n` line numbers add tokens.
- Limit: the five prose parts cannot be separated from each other. Their sizes barely vary between sessions, so a regression gives nonsense per-part ratios. Treat per-part tokens as about 10% accurate (the launched skill at 3.03 is about 15% low) and the total as 2% accurate. The first-request total itself needs no estimate: read it.
- Prefer measuring where possible: a part's real cost is the occupancy jump when it is added (`occupancy(next) - occupancy(this) - output_tokens(this)`), and `cache_read` gives the tools plus system prompt prefix exactly.

### Skill files read during a session

- Skill tool: an `assistant` record with a `tool_use` named `Skill` and input `{skill, args}`. Then a `user` record with a `tool_result` of "Launching skill: X" (23 to 29 bytes; `toolUseResult` has `success` and `commandName`). Then a `user` record with `isMeta: true`, `sourceToolUseID` set to that `tool_use` id, and the whole skill body in its text, starting "Base directory for this skill: <dir>". Its byte size is the cost: gx-follow-up 4.7 to 5.4 KB (1.8K to 2.1K tokens measured), unslop 6.0 KB, verify 2.2 KB, gx-ui-check 2.5 KB, gx-verify 4.6 KB.
- Launch by slash command (how ralph-loop starts a ticket): a `user` record with `<command-name>/gx-implement</command-name>` and then the same `isMeta` record, with no `sourceToolUseID` and before the first assistant record. This is part of the first request, so it belongs to the fixed context, not to "skill files read".
- Read tool: a `tool_use` named `Read` with `file_path` (and `offset`, `limit`). The `user` record has `toolUseResult.file` with `filePath`, `content`, `numLines`, `startLine`, `totalLines`. The `tool_result` text is the `cat -n` form, larger than `file.content`. Example: `~/.claude/skills/gx-local-tracker.md` read by a gx-evolve session, 31,086 bytes (about 15K tokens at 2.0).
- Detecting skill reads: match `Skill` calls by `input.skill` and Reads by a path under `~/.claude/skills/`, `<cwd>/.claude/skills/` or `~/.codex/skills/`, or with basename `SKILL.md`. A Read of the gx repo's own `skills/` directory is source code being edited (verify-mocks-impl read `skills/gx-verify-epic/SKILL.md` that way), so the path rule alone over-counts there. Counting only `isMeta` skill records and Reads under the skill directories is exact; a skill's linked files are only found by directory.
- Cost: use the occupancy jump above, or bytes / 2.6 (Skill) and bytes / 2.0 (Read) when the next call is missing.

### Codex

- Yes, readable, but `codexsession` has no first-call reader today. It exposes `PeakContext` (maximum) and `LastContextTokensIn` (last). The file holds it: the first `event_msg` of type `token_count` has `info.last_token_usage.input_tokens`, which already includes `cached_input_tokens` and equals `total_token_usage.input_tokens` on that first event. `sessionLine` already parses the field.
- Correct fix: add a `FirstContextTokensIn(home, cwd, sessionID)` next to `LastContextTokensIn` (same identity check, stop at the first `token_count` with `input_tokens > 0`). Do not use `PeakContext`.
- Values: 39 gx-launched Codex CLI sessions (cwd under `~/dev/gx/<worktree>`, not `main`, source `cli`): median 18.7K, range 14.4K to 19.5K. Interactive sessions in `main` sit near 20K to 22K. No `token_count` has null `info` before the first real one in 350 files. No breakdown is possible: the rollout holds `base_instructions`, `additional_tools` and skill instruction messages, but the decision already limits Codex to the total.

### Can the `prefix_tokens` stamp serve this map

- Same number: yes. The stamp is the occupancy of the first model call (input + cache_read + cache_creation) of the session whose peak is the actual window, which is the first `assistant` record's usage above. It is the map's "fixed context", launched skill and first prompt included. Reuse it for the per-ticket headline, as the Notes say.
- Not enough on its own: the stamp is one value per ticket (the merging session) and is empty when that session was resumed. The map measures every session gx launches, and the history view needs one value per session. So the analysis package reads the first assistant record of each session file and the runner copies that number into `prefix_tokens`. They cannot disagree because both come from one function.
- Each session is its own file with its own attachments and first call (checked on three directories with 2 to 48 sessions; first occupancy 45K to 50K in each). Whether `claude --resume` copies old history into a new file was not tested.

### Recommendations

1. Read the first-request total from the first `assistant` record. Never estimate it.
2. Split it with the attachment records plus `prompt_snapshot` for the system prompt. Take tools from `api-request-shape` when present. When absent, report "system prompt and tools" as the remainder, labelled unsplit. Do not guess a tools size.
3. Estimate tokens with 2.68 (tools, system prompt) and 3.03 (other parts), 2.6 for skill text and 2.0 for Read output. Show tokens as approximate.
4. `api-request-shape` is a private record (absent in 2.1.296 and older, present from 2.1.301; 2.1.297 to 2.1.300 not sampled). The reader must treat it as optional and the report must say when it is missing.
5. Add `FirstContextTokensIn` to `codexsession`.
6. The biggest reduction target is tools, not the skill listing: the Artifact tool is about 14K tokens, the skill listing about 8.4K. Ticket 04 should measure launch options against the tools part.

### Not established

- Per-part accuracy for the skill listing, deferred tools, agent listing and MCP instructions separately.
- Whether `claude --resume` writes old history into a new file.
- Which version first wrote `api-request-shape` (between 2.1.296 and 2.1.301).
- Codex sessions from `codex-support-impl`: no rollout with an `item-` cwd exists under `~/.codex/sessions`, so the 39 Codex sessions come from other gx worktrees.

## Method

- One session per ralph-loop project directory (`-Users-david-dev-gx-<epic>-item-*`), the newest file of the most recently written directory, 109 epics. Parts were read up to the first `assistant` record. Bytes are UTF-8 bytes of the text (tools: compact JSON).
- Ratios: total tokens = first assistant `usage` input + cache_creation + cache_read. Prefix ratio from `cache_read_input_tokens` against tools + `system[0..1]`. Skill and Read ratios from the occupancy jump between consecutive main-thread requests with a single tool call, minus the earlier request output tokens.
- Codex: first `event_msg` `token_count` with non-null `info` in each rollout.
- Code read: `transcript/transcript.go` (`Usage.Occupancy`), `ralphloop/report_metrics.go` (merged metrics), `codexsession/session.go` (`readStats`, `readContextTokens`), `docs/specs/ticket-authoring-tools.md` at commit 4cf9ebbd (`prefix_tokens`).
- Scripts were throwaway and are not committed.
