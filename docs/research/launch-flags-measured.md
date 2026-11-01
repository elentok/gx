# Launch flags measured: what each one removes from the first request

Ticket `gx:fixed-context-monitor/04`. No gx code was changed.

## Method

- Version: Claude Code 2.1.316. Auth: the host's default (subscription OAuth). `ANTHROPIC_API_KEY` is unset.
- Scratch directory: a throwaway git repo with a 2-line `CLAUDE.md`. No project skills, no `.mcp.json`.
- Each run is `claude -p "<prompt>" --output-format stream-json --verbose --no-session-persistence`, plus the launch args from `agentArgs` in `ralphloop/loop.go`: `--permission-mode auto`, the `--settings` guard hook, `--disallowedTools AskUserQuestion,EnterPlanMode,ExitPlanMode` and `--append-system-prompt-file` with `harness.GXSystemPrompt`.
- The number is the first `assistant` message's `usage`: `input_tokens + cache_read_input_tokens + cache_creation_input_tokens`.
- Prompts: `say ok` (the "bare" number), or `/gx-implement gx:nonexistent/00 -- launch test only ...` (adds the skill body, about 6K tokens).
- Models: haiku for the matrix (cheap), sonnet (the ralph-loop worker tier) for the confirmation runs.
- Noise: the claude.ai connectors (Docs, Drive, Gmail, Calendar) connect or stay `pending` from run to run, so a `say ok` total moves by about 1K between identical runs. Treat differences under 1K as noise.
- The 45.7K first request seen in a real interactive ticket is larger than the 26K `-p` baseline here. The real one has a 17 KB first user message, a 54 KB skill listing, more MCP and Artifact text and a 7 KB instructions block. The tmux interactive check was not run (see "Not measured").

## Results, haiku, prompt `say ok`

Baseline (current launch): **25,892** tokens. 102 skills, 50 tools, 11 plugins.

| Option | First request | Saved | What broke |
|---|---|---|---|
| `--strict-mcp-config` (no `--mcp-config`) | 23,658 | 2,234 (9%) | gopls-mcp gone and claude.ai connectors gone. |
| `--strict-mcp-config --mcp-config <gopls>` | 23,966 | 1,926 (7%) | Nothing, but gx must copy the MCP server config. User and project MCP servers are ignored. |
| `ENABLE_CLAUDEAI_MCP_SERVERS=false` (env) | 23,966 | 1,926 (7%) | Nothing. gopls-mcp stays. Same saving as the line above, without copying config. |
| `--disable-slash-commands` | 17,014 | 8,878 (34%) | **`/gx-implement` is not resolved, and no error shows.** The model gets the literal text `/gx-implement ...` and answers "ok". 0 skills. |
| `--setting-sources project,local` | 18,788 | 7,104 (27%) | Same silent failure: user skills are gone (20 bundled skills remain). gopls-mcp gone (user MCP). `~/.claude/CLAUDE.md` gone. User plugins gone. Auth, the `--settings` guard hook and subagents still work. |
| `--setting-sources ""` | 18,598 | 7,294 | Same as the line above. |
| `--bare` | not measurable | n/a | `Not logged in`. OAuth and keychain are never read. It needs `ANTHROPIC_API_KEY` or an `apiKeyHelper`. Not usable with a subscription. |
| `--safe-mode` | 16,320 | 9,572 (37%) | Drops CLAUDE.md, skills, hooks, MCP. The guard hook and `/gx-implement` are gone. |
| `--restricted` | 15,866 | 10,026 (39%) | Removes Bash and ignores user settings. A ticket agent cannot run `gx`. |
| `--tools Bash,Read,Edit,Write,Grep,Glob,Skill,Agent,ToolSearch` | 24,272 | 1,620 (6%) | Drops the other inline tools. MCP tools stay. |
| `--disallowedTools` + `ScheduleWakeup,ReportFindings,ListAgents` | 22,971 | 2,921 (11%) | Those three tools are unavailable. A ticket agent does not use them. |
| `skillOverrides` -> `off` for all but 8 gx skills (in `--settings`) | 18,404 | 7,488 (29%) | Nothing. 9 skills listed. `/name` of an `off` skill stops working. |
| `skillOverrides` -> `user-invocable-only` for the same skills | 18,404 | 7,488 (29%) | Nothing. The listing is as small as `off`, and `/name` still works. The model cannot call these through `Skill` (it gets a clear error). |
| `skillOverrides` -> `name-only` | 18,918 | 6,974 (27%) | Nothing. Slightly larger than the two above. |
| `skillListingBudgetFraction: 0.002` | 20,254 | 5,638 (22%) | Descriptions are cut to fit, so skills are harder to find. |
| `disableBundledSkills: true` | 23,956 | 1,936 (7%) | Nothing breaks, but see the trap below. |
| `enabledPlugins` all `false` | 25,728 | 164 | LSP plugins gone. They cost almost nothing. |
| `CLAUDE_CODE_DISABLE_WORKFLOWS=1` | 25,814 | 78 | Only two skills fewer. The `Workflow` tool schema stays. |
| `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` | 25,153 | 739 | No memory section. The test cwd has no memory, so a real worktree may differ. |
| `--exclude-dynamic-system-prompt-sections` | 25,654 | 238 | It moves the sections, it does not remove them. |
| `outputStyle: "default"` (drop the user's `Concise`) | 25,410 | 482 | Changes the agent's reply style. |

### Trap: `disableBundledSkills` with `--setting-sources project,local`

`disableBundledSkills` shrinks the skill listing from 15 KB to 1 KB, but the inline `Workflow` tool schema grows from 5.5 KB to 22.9 KB (read from the transcript's `api-request-shape`). With `--setting-sources project,local` the total went **up** from 18,788 to 22,723. Do not use `disableBundledSkills`. Set the bundled skills to `user-invocable-only` in `skillOverrides` instead.

### The `/gx-implement` check

Prompt `/gx-implement gx:nonexistent/00 ...` on haiku. "Resolves" means the first request grew by the skill body (about 6K tokens) and the init message lists the skill.

| Launch | First request | `/gx-implement` |
|---|---|---|
| current | 31,967 | resolves |
| `--disable-slash-commands` | 17,193 | **does not resolve, silent** |
| `--setting-sources project,local` | 19,049 | **does not resolve, silent** |
| `skillOverrides` -> `user-invocable-only` (keep 8 gx skills) | 24,479 | resolves |
| `--setting-sources project,local --plugin-dir <dir with 8 gx skills> --strict-mcp-config` | 25,363 | resolves. The skill is named `gxloop:gx-implement`; `/gx-implement` still works as a short name. |

A launch that does not resolve the slash command gives no error. The agent starts, answers the literal text and the ticket does nothing. Any option that drops user skills needs a startup check that the skill resolves.

## Results, sonnet (the worker tier), prompt `/gx-implement ...`

| Launch | First request | Saved | Notes |
|---|---|---|---|
| current | 29,241 | n/a | 102 skills |
| `skillOverrides` only (`user-invocable-only`, keep 8) | 23,531 | 5,710 (20%) | |
| `--disallowedTools` + 3 tools only | 28,246 | 995 (3%) | Within noise for sonnet. |
| **Option A**: `skillOverrides` + `ENABLE_CLAUDEAI_MCP_SERVERS=false` + 3 more disallowed tools | 18,684 | 10,557 (36%) | Resolves. Everything below passed. |
| **Option B**: `--setting-sources project,local --plugin-dir --strict-mcp-config --mcp-config <gopls>` | 24,871 | 4,370 (15%) | Resolves. The 20 bundled skills stay listed. Setting them to `user-invocable-only` would take it near Option A. |
| `--disable-slash-commands` | 16,370 | 12,871 | Does not resolve `/gx-implement`. |

## What still works under Option A (haiku)

- `/gx-implement` resolves (above).
- The `--settings` guard hook still blocks `sed -i`, and the agent falls back to Edit.
- `~/.claude/CLAUDE.md` is still loaded (the agent quoted the GNU/`fd`/`rg` rules).
- A subagent (`Agent` with `Explore`) runs and reports back.
- gopls-mcp connects and a tool call works (`go_list_tools` returned 14 tools).
- `Skill gx-tdd` (kept) loads. `Skill gx-wayfinder` (hidden) returns `cannot be used with Skill tool due to disable-model-invocation`.
- Auth is unchanged.

## What Option B drops

- `~/.claude/CLAUDE.md` (a test with the global rule answered `NONE`).
- User MCP servers (gopls-mcp has to be passed again with `--mcp-config`), user hooks, permissions, `enabledPlugins` and `outputStyle`.
- The guard hook, a subagent, and the `/gx-implement` check all still worked.

## Can a launch keep only a named list of skills?

Yes, two ways.

1. **`skillOverrides` in `--settings`.** It is a deny list: gx lists the installed skills at launch and sets every skill outside its keep list to `user-invocable-only`. It works from a `--settings` JSON string and merges with the guard hook. A skill added later is listed until gx's next launch regenerates the map. The init message's `skills` list misses plugin skills and built-ins (`init`, `security-review`, `keybindings-help`, `ralph-loop:*`); take the names from the real skill listing instead.
2. **An allow list by plugin.** `--setting-sources project,local --plugin-dir <dir>` where the dir holds copies or symlinks of the wanted skills. New personal skills never load. The price is everything the user source holds (above), so it needs more flags to put back.

## Recommendation

Use Option A.

- It cuts about 10.5K tokens (36%) of a sonnet ticket's first request and breaks nothing that was tested.
- Each part is a documented setting or env var, so it is a correct fix, not a workaround.
- Keep the loop-skill list in gx. Include every skill the loop sessions call: at least `gx-implement`, `gx-tdd`, `gx-verify`, `gx-ui-check`, `gx-code-review`, `gx-research`, `gx-resolving-merge-conflicts`, `gx-follow-up`, `gx-unslop`. Read the skill names from the session's skill listing, not from the init `skills` field.
- Use `user-invocable-only`, not `off`: the launch prompt `/<skill>` keeps working.
- Do not use `--disable-slash-commands`, `--setting-sources`, `--bare`, `--safe-mode`, `--restricted` or `disableBundledSkills`. Each breaks `/gx-implement`, auth, tools or the token count.
- Add a startup check that the launched `/<skill>` resolves (the init message lists it), because a miss fails silently.
- Where the extra plumbing is acceptable, Option B (allow list) is the stricter choice (not measured with the bundled skills hidden). It was not chosen because it drops the user's `CLAUDE.md`, MCP servers and hooks.

## Not measured

- Interactive (tmux) first-request numbers. The first interactive run in a new directory shows a trust dialog that writes to `~/.claude.json`; that was not allowed. The `-p` totals (about 26K) are below the 45.7K seen in real interactive tickets, so the percentages may be lower in the real launch while the token counts should carry over.
- `--bare` tokens (needs an API key).
- Codex.
- A project with its own `.claude/skills`, `.mcp.json` or a long `CLAUDE.md`.
- Repeat runs for noise. Each cell is one run.
