# Where commands come from: Claude Code and Codex

Research for ticket gx:webui-agents-skills/01. Run on 2026-11-01 against Claude Code 2.1.316 and
codex-cli 0.166.0 on macOS. The question is which commands a running session can run and where gx
can read them. It does not decide what the web UI menu lists (ticket 02).

Evidence tags used below:

- **[V]** verified by running the installed binary or reading files on this machine.
- **[S]** read in first-party source (OpenAI's `openai/codex` repo at tag `rust-v0.166.0`) and
  checked against strings in the installed binary.
- **[D]** read in official docs only, not reproduced here.

## Summary

- Claude Code has a no-model way to list commands: start `claude -p` in stream-json mode and send an
  `initialize` control request. The reply has `commands` (name, description, argument hint,
  aliases, built-in flag). It takes about 3 seconds, makes no model call and writes no session
  file. [V]
- That list is incomplete. It omits 68 of the 120 commands in the docs table, all of them built-ins
  that need the terminal UI (`/help`, `/exit`, `/theme`, `/resume`, `/permissions`, `/skills`,
  `/status`, `/diff`, `/plan` and more). No CLI verb or file lists them. gx must keep its own list
  of those. [V]
- Codex has a no-model way to list skills only: `codex app-server` over stdio, method
  `skills/list`. It returns name, description, path, scope, enabled flag, plugin id and display
  metadata, plus per-file load errors. [V]
- Codex has no way to list its `/` built-ins. The list is a Rust enum in the TUI; no app-server
  method exposes it. The enum is public source, so gx can copy it. [V, S]
- `/` and `$` differ by harness. Claude: `/` runs built-ins, skills, custom commands, plugin
  commands and MCP prompts. Codex: `/` runs built-ins only, `$` mentions skills. Codex 0.166.0 no
  longer reads `~/.codex/prompts`. [V]
- Both harnesses read project skills from the cwd up to the repo root. A linked worktree has its own
  checkout, so it sees only the files committed in it (or untracked there), never the main
  checkout's untracked files. [V]
- Name collisions differ. Claude hides the lower-priority skill of the same name. Codex lists both.
  [V]

## Method

Scripts are in the session scratchpad and are short enough to repeat.

Claude, no model turn, run from the directory whose commands you want:

```sh
(printf '%s\n' '{"type":"control_request","request_id":"r1","request":{"subtype":"initialize"}}'; sleep 6) |
  claude -p --input-format stream-json --output-format stream-json --verbose
```

The reply is a `control_response` line. `response.response.commands` is the list. Side effect: the
user's `SessionStart` hooks run (`hook_started` and `hook_response` lines appear first). [V]

Codex, no model turn:

```sh
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"x","version":"0"}}}' \
 '{"jsonrpc":"2.0","method":"initialized"}' \
 '{"jsonrpc":"2.0","id":2,"method":"skills/list","params":{"cwds":["/path"],"forceReload":true}}' |
  codex app-server
```

The JSON schema for the protocol comes from `codex app-server generate-json-schema --out DIR`. It
has 106 client request methods. Skill and plugin methods: `skills/list`, `skills/extraRoots/set`,
`skills/config/write`, `plugin/list`, `plugin/installed`, `plugin/read`, `plugin/skill/read`
and others. A `skills/changed` notification tells a client to call `skills/list` again. [V]

Both probes start a fresh process. They do not query a running TUI session. See "Unknown".

## 1. Claude Code

### Where skills and commands live

| Source | Location | Check |
| --- | --- | --- |
| User skills | `~/.claude/skills/<name>/SKILL.md` | [V] (here the dir is a symlink to dotfiles; symlinks work) |
| User commands | `~/.claude/commands/<name>.md`, subfolders become `a:b` | [V] with `CLAUDE_CONFIG_DIR` pointing at a test dir |
| Project skills | `<repo>/.claude/skills/<name>/SKILL.md` | [V] |
| Project commands | `<repo>/.claude/commands/<name>.md`, subfolders become `a:b` | [V] |
| Nested skills | `<subdir>/.claude/skills/...` | [V] listed only when the session starts in or below `<subdir>`; [D] also loaded when Claude reads a file there |
| Plugin skills and commands | `<plugin>/skills`, `<plugin>/commands` | [V] with `--plugin-dir`; name is `plugin:name` |
| Installed plugins | `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>` | [V] files exist; see below |
| MCP prompts | from connected MCP servers | [V] |
| Enterprise skills | managed settings directory | [D] |
| `--add-dir` directories | their `.claude/skills` and `.claude/commands` | [D] |

Sources: https://code.claude.com/docs/en/skills (locations, precedence), commands and MCP pages
listed under "Sources".

Details:

- The root is `CLAUDE_CONFIG_DIR` if set, else `~/.claude`. [V]
- Project skills load from the cwd and every parent up to the repo root. Starting in `sub/` still
  shows the root's skills. [V]
- Skills in a folder two levels below `.claude/skills` (`grp/inner/SKILL.md`) are not found. Claude
  reads one level only. Command files do recurse, with `:` in the name. [V]
- Installed plugins are recorded in `~/.claude/plugins/installed_plugins.json` (each entry has a
  scope, and a `projectPath` for project scope) and enabled in `settings.json` `enabledPlugins`.
  The cache keeps stale version folders (more than a dozen for `frontend-design` here). Reading these files
  directly would mix stale and live copies, so asking Claude is safer. [V]
- Live change detection: Claude watches the skill folders, and `/reload-skills` forces a re-scan.
  A brand-new top-level skills folder needs `/reload-skills`. [D, not tested]

### How names and descriptions are stored

- A skill's command name is the frontmatter `name`, else the folder name. When `name` differs from
  the folder, the folder name becomes an alias: `name: Pretty-Name` in folder `renamed` listed as
  `Pretty-Name` with alias `renamed`. User skill `Bash` in folder `bash` lists alias `bash`. [V]
- The description is the frontmatter `description`. With none, the first paragraph is used. For a
  command file with no frontmatter, the first line is used. [V]
- The `initialize` list does not truncate: a 2000-character description came back whole. The docs
  say the model-facing listing cuts `description` plus `when_to_use` at 1,536 characters. The
  `when_to_use` field is not in the `initialize` description. [V for list, D for the cap]
- Claude appends the source to the description text: ` (user)`, ` (project)` for files, a
  leading `(plugin-name) ` for plugin items, and ` (MCP)` on the name for MCP prompts. Built-ins
  and bundled skills have no suffix and carry `"builtin": true`. [V]
- Claude HTML-escapes `<` and `>` in descriptions, in both the `initialize` list and the TUI menu.
  The file has `docs/specs/<epic-slug>.md`; the list has `docs/specs/&lt;epic-slug&gt;.md`. A gx
  menu that reuses these strings must unescape them. [V]
- `argumentHint` comes from `argument-hint` frontmatter. For an MCP prompt it was empty even though
  the prompt declared a required argument. [V]
- `user-invocable: false` removes the skill from the list. `disable-model-invocation: true` keeps it
  in the list (the user can still run it). [V]
- Command files accept the skill frontmatter except `name` and `paths`. [D]

### Collisions and namespacing

- Project skill `tdd` and user skill `tdd`: only the user one is listed. The project one is hidden.
  [V] Docs order: enterprise over personal over project. [D]
- A skill and a command file with the same name (`deploy`): one entry, the skill. [V]
- A user skill named like a bundled one replaces it. Here the user skill `code-review` shows once
  with `(user)` and no `builtin` flag. [V]
- Plugin items are always namespaced, so a plugin `tdd` does not collide with user `tdd`: both are
  listed (`tdd`, `myplug:tdd`). Nested folders add parts (`myplug:sub:deeper`). [V]
- When the plugin and the skill have the same name, the list also carries the short alias:
  `frontend-design:frontend-design` has alias `frontend-design`. [V]
- Skills synced from claude.ai appear as `docs` with alias `anthropic-skills:docs`. [V]
- MCP prompts: the list shows `probe srv:review_pr (MCP)` (server name kept raw, including the
  space). The typed form in the docs is `/mcp__servername__promptname`, with characters outside
  `A-Za-z0-9_-` in the server name replaced by `_`. The typed form was not run. [V for list, D for
  typed form]

### Worktrees

A linked worktree is a separate checkout. In a test repo with a linked worktree:

- Committed `.claude/skills` and `.claude/commands` appear in the worktree. [V]
- An untracked skill in the main checkout does not appear in the worktree. [V]
- User-level skills and plugins are the same everywhere. [V]
- A skill folder that is a symlink into another checkout (as gx's installer can create) follows the
  link, so editing the target changes every worktree. [V for symlinked user dir]

### Built-in commands

Does Claude expose a list? Only a partial one.

- The `initialize` list holds 53 entries flagged `builtin` (these include bundled skills such as
  `/simplify`). They are the ones that also work without the TUI: prompt-style commands and simple ones (`/model`, `/compact`, `/context`,
  `/clear`, `/config`, `/init`, `/mcp`, `/usage`, `/effort`, `/loop`, `/simplify`, `/doctor`...).
  [V]
- 68 commands from the docs table are absent from it: `add-dir`, `branch`, `btw`, `bug`, `cd`,
  `copy`, `diff`, `exit`, `export`, `feedback`, `fork`, `help`, `hooks`, `ide`, `keybindings`,
  `login`, `logout`, `memory`, `permissions`, `plan`, `plugin`, `release-notes`, `resume`,
  `rewind`, `sandbox`, `skills`, `status`, `statusline`, `tasks`, `theme`, `tui`, `vim`, `voice`
  and more (the full diff is reproducible from the docs table). Some of these are gated by plan
  or platform. [V]
- The TUI menu does list them. `/exit`, `/help`, `/diff`, `/resume`, `/permissions` and `/theme`
  were seen in the popup. [V]
- The two lists use different descriptions for the same command. `/fewer-permission-prompts` is
  "Pre-approve safe read-only commands based on your usage" in the TUI menu and "Scan your
  transcripts for common read-only Bash and MCP tool calls..." in `initialize`. [V]
- `claude --help` has no verb that prints commands. [V]
- Availability depends on plan, platform and environment. Unavailable commands are left out of the
  menu and answer "Unknown command" when typed. [D]
- The menu also hides a few commands until the full name is typed (`/heapdump`). [D]

So gx would need its own built-in list for the TUI-only commands, or accept that they cannot be
listed. The `initialize` list covers everything else.

### Which built-ins open a dialog, print, or end the session

Tested in the TUI of 2.1.316 (pressing Enter on each):

| Behavior | Commands |
| --- | --- |
| Opens a dialog or picker, blocks input until Esc or Enter | `/theme`, `/help` (tabs), `/skills`, `/status`, `/usage` and `/cost`, `/model`, `/config`, `/permissions`, `/mcp`, `/resume`, `/memory`, `/plugin`, `/hooks`, `/release-notes`, `/export` (method picker) |
| Prints a line into the transcript, no model call | `/context` (grid), `/copy` ("No assistant message to copy"), `/agents` (text reminder), `/diff` outside a git repo (notice), an unknown command ("Unknown command: /nosuchcmd") |
| Opens an external editor in the pane | `/keybindings` (opened vim on `keybindings.json`) |
| Ends the session | `/exit` (process exits and prints "Resume this session with: claude --resume <id>") |
| Starts a model turn | any skill, `/doctor` |

Not run: `/clear`, `/compact`, `/rewind`, `/init`, `/add-dir`, `/rename`, `/fork`, `/branch`,
`/login`, `/logout`. The docs say: a message sent while Claude is working is queued, a few commands
run at once (`/status`, `/tasks`, `/usage`), and in fullscreen mode dialog commands such as `/theme`
and `/help` open at once. [D] Text typed in the pane that is not a recognised command becomes a
prompt (this happened by accident when a `/` was swallowed by vim mode). [V]

## 2. Codex

### Where skills live

`skills/list` result for a test home, repo and worktree [V]:

| Source | Location | `scope` |
| --- | --- | --- |
| User | `~/.agents/skills/<name>/SKILL.md` | `user` |
| Codex home | `$CODEX_HOME/skills/<name>/SKILL.md` (default `~/.codex/skills`) | `user` |
| System | `$CODEX_HOME/skills/.system/...` (5 bundled here) | `system` |
| Repo | `<dir>/.agents/skills/...` for every dir from the cwd up to the repo root | `repo` |
| Repo, undocumented | `<repo>/.codex/skills/...` | `repo` |
| Plugins | `~/.codex/plugins/cache/<marketplace>/<plugin>/<version>/skills/...` | `user`, with `pluginId` |
| Admin | `/etc/codex/skills` | `admin` [D] |

- Both user roots are read, so a skill present in both is listed twice. This confirms the comment in
  `skills/duplicates.go`. [V]
- Codex recurses: `.agents/skills/grp/inner/SKILL.md` is found. It follows symlinks and reports the
  target path (here `~/.agents/skills` is a symlink to `~/.claude/skills`, which is a symlink to
  dotfiles, and paths come back as `/Users/david/.dotfiles/...` and `/Users/david/dev/gx/main/skills/...`). [V]
- Worktrees behave like Claude: a linked worktree with committed `.agents/skills` finds them; a
  nested `sub/deep/.agents/skills` adds to the root ones when the cwd is inside. [V]
- Change detection is automatic in the docs, with a restart if a change does not show up. [D] The
  protocol has a `skills/changed` notification. [V]
- Disable a skill with `[[skills.config]] path=... enabled=false` in `config.toml` [D]; the list
  has an `enabled` flag per skill (all true here). [V]

### How names and descriptions are stored

- `SKILL.md` needs `name` and `description`. A file with no `description` is skipped and shows in
  `errors` (`missing field \`description\``), unlike Claude, which falls back to the body. [V]
- The name is the frontmatter `name` (folder `renamed` listed as `Pretty-Codex`). [V]
- Optional `agents/openai.yaml` adds `interface` (display name, short description, icons, default
  prompt) and `policy.allow_implicit_invocation`. [D for the fields; V that the list returns
  `interface` and `shortDescription`]
- Plugin skills have names like `pdf:pdf` and `spreadsheets:excel-live-control` and a `pluginId` such
  as `pdf@openai-primary-runtime`. [V]
- Descriptions come back unescaped and untruncated. Raw `<` stays raw. [V]

### Collisions

- Codex does not merge or hide same-name skills. A repo `tdd` and a user `tdd` are both listed, and
  so are the `~/.agents` and `~/.codex` copies. The docs say both can appear in selectors. [V, D]
- The `skill-creator` name appears twice on this machine (system and the synced user copy). [V]

### What `$` and `/` offer

- `$` opens a skill popup. Each row shows the display name from `interface.displayName` when there
  is one (`Research` for `gx-research`, `Bump` for `gx-bump`) and the short description. Typing
  filters by the skill name. Choosing a row inserts `$<skill name>`, e.g. `$gx-research`. So the
  label can differ from what is typed, and two skills can show the same label (`Grilling` showed
  twice). [V]
- `/skills` opens a small menu ("List skills", "Enable/Disable Skills") and the tip says to press
  `$` to open the list directly. [V]
- `/` opens a popup of built-in commands only. A file `~/.codex/prompts/<x>.md` (temporary test
  file, removed) did not show. `/gx` showed no match, so skills are not offered under `/`. Typing an
  unknown command prints: `Unrecognized command '/nosuchcmd'. Type "/" for a list of supported
  commands.` [V]
- Upstream removed custom prompts: commit 48144a7fa "Remove remaining custom prompt support
  (#16115)" (2026-03-28), and `docs/prompts.md` is gone at `rust-v0.166.0`. [V via `gh api`]
- The binary has no `.codex/prompts` string. [V]
- `/presets` ("run or save reusable prompts and slash commands") is listed in the popup. Where it
  stores things was not examined.

### Built-in commands

Source: `codex-rs/tui/src/slash_command.rs` at `rust-v0.166.0`. The enum has 65 variants; its order is
the popup order. All sampled description strings are present in the installed binary. [S, V]

Facts that matter for a menu:

- Each variant has a description (`description()`), an inline-argument flag
  (`supports_inline_args`), and gates: `available_during_task`, `available_in_side_conversation`,
  `available_when_thread_unavailable`. [S]
- Hidden in release builds: `rollout`, `test-approval`. Shown only on macOS and Windows: `app`.
  Hidden on Android: `copy`. Two debug commands (`debug-m-drop`, `debug-m-update`) are described
  "DO NOT USE" but are not hidden by `is_visible`. [S]
- Aliases: `cwd` for `pwd`, `pet` for `pets`, `clean` for `stop`; `quit` and `exit` both exit;
  `approve` is the name of the auto-review command. [S]
- Not allowed while a task runs: `new`, `delete`, `fork`, `worktree`, `init`, `compact`, `recap`,
  `export`, `keymap`, `tui`, `vim`, `setup-default-sandbox`, `experimental`, `memories`, `import`,
  `review`, `plan`, `cd`, `clear`, `logout`, `config`, `theme`, `pets` and the two debug ones. [S]
- The popup has entries that are not in the enum (`/fast`: "2x speed, increased usage"). So the
  enum is not the complete menu. [V]

Behavior, tested in the TUI of 0.166.0: `/model` opens a picker; `/skills` opens a menu; `/status`
prints a block of session info into the transcript; `/exit` ends the process. [V] The rest is
inferred from descriptions and not tested: `/theme`, `/permissions`, `/resume`, `/mcp`, `/plugins`,
`/statusline`, `/title`, `/keymap` are pickers or editors; `/diff`, `/status`, `/usage`, `/pwd`,
`/warnings`, `/rollout` print; `/quit` and `/exit` leave; `/new`, `/clear`, `/fork`, `/compact`,
`/init`, `/review` start or change a thread. [S, not run]

## 3. What gx has today

- `harness/skillprefix.go`: `SkillCommand` types `/skill` for Claude and `$skill` for Codex. It
  matches the facts above. [V by reading]
- `skills/roots.go` and `cmd/skills.go`: gx installs its bundle into `~/.claude/skills` and
  `~/.agents/skills`. Both roots are scanned by the harness they target. [V]
- `skills/duplicates.go` reports skills that also sit in `$CODEX_HOME/skills`. Correct: Codex
  lists both. [V]
- `skills/codex_discovery_test.go` already uses `codex debug prompt-input` to check discovery
  without a model turn. `codex app-server` `skills/list` is a more direct probe. [V by reading]
- `ralphloop/deps.go` (`verifySkillWith`, around line 477) checks `~/.codex/prompts/<skill>.md`
  for Codex. In 0.166.0 Codex does not read that path. On this machine `~/.codex/prompts` holds only
  empty `gx-*` folders and two symlinks, left from older installs. The check is stale for current
  Codex. Not fixed here; it is outside this ticket. [V]

## Unknown

1. **A running session's list.** Both probes start a new process. Neither harness lets another
   process ask a live TUI session what it can run. Claude's file watcher, `/reload-skills`, nested
   on-demand loading and Codex's change detection mean a live session may differ from a fresh probe.
   Not tested.
2. **Nested skills in a live Claude session** load on first file access per the docs. Cannot be
   reproduced without a model turn.
3. **Claude's TUI-only built-ins** have no machine-readable list. The Claude binary was not
   searched for one; any extraction would break on updates.
4. **Plan, platform and policy gating** of Claude commands is known only from docs. The
   `initialize` call showed the commands for this account only.
5. **Claude project-scope plugin installs** (`scope: project`, `projectPath`): whether another
   project sees them was not tested. The `initialize` call applies the right rules.
6. **MCP prompt timing and typed form.** The prompt showed after waiting; whether an immediate
   request can miss slow servers was not tested. `/mcp__server__prompt` was not typed.
7. **Which duplicate Codex runs** for `$tdd` when two skills share the name.
8. **Codex trust prompt.** The TUI asks "Trust this folder?" for new folders and says the choice is
   saved. `skills/list` returned repo skills in an untrusted folder, but whether the TUI withholds
   them before trust was not tested (accepting would have changed `config.toml`).
9. **Codex plugin skills** invoked with `$pdf:pdf`, and any plugin-provided `/` commands, were not
   run. `/presets` storage was not examined.
10. **Behavior of untested built-ins** in both harnesses (see the tables).
11. **Admin and managed locations** (`/etc/codex/skills`, Claude managed settings) are docs only.
12. **Other platforms.** Only macOS was used.

## Side effects of the research

- A tmux session sent text to a real Claude TUI in the scratchpad project dir. One stray line
  (`suchcmd`) and `/doctor` caused two short model turns, so session files exist under
  `~/.claude/projects/` for that scratchpad path.
- A temporary file `~/.codex/prompts/gxresearchprobe.md` was created and deleted.
- Codex was started in the design worktree (a trusted path). No config was saved.

## Sources

- https://code.claude.com/docs/en/skills (locations, precedence, frontmatter, live detection)
- https://code.claude.com/docs/en/commands (built-in table, menu rules)
- https://code.claude.com/docs/en/mcp#use-mcp-prompts-as-commands
- https://code.claude.com/docs/en/plugins-reference (namespacing, layout)
- https://learn.chatgpt.com/docs/build-skills (redirect target of developers.openai.com/codex/skills)
- https://github.com/openai/codex at `rust-v0.166.0`: `codex-rs/tui/src/slash_command.rs`, `docs/`
- Commit 48144a7fa in `openai/codex` (custom prompts removal)
- Installed: `claude` 2.1.316, `codex` 0.166.0 (`codex app-server generate-json-schema`)
