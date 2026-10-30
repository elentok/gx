# Codex research retest on 0.166.0 (gx:codex-support/08)

Run on 2026-10-30 against codex-cli 0.166.0. `codex --version` printed 0.166.0 before the first test and after the last one. The install did not change.

The two notes under test are `research/gx-skills-on-codex` (`docs/research/gx-skills-on-codex.md`, ticket 02) and `research/codex-signals-on-pty` (`docs/research/codex-signals-on-pty.md`, ticket 01). Both ran partly or fully on 0.147.0. I had no 0.147.0 binary and did not install one, so "same" below means "same as the old note says". I did not run 0.147.0 myself.

Labels.

- Observed means I ran it on 0.166.0.
- Source means I read it in `openai/codex` at the tag named, and did not run it.
- Untested means neither.

## Method and isolation

- The global `codex` only. No install, update or uninstall command was run. The update check was off (`check_for_update_on_startup = false`).
- `CODEX_HOME` pointed at a temp directory with a copy of `auth.json` and a small `config.toml`. `HOME` was a temp directory, so skills sat in a temp `.agents/skills`. The real `~/.codex` and `~/.agents` were not written by these runs. `auth.json` was never printed.
- The TUI ran inside a private `tmux -L` server (a private PTY), driven by a small Python script that sent keys, read the screen and read the pane title. No herdr.
- A throwaway git repo, plus a bare clone with a linked worktree, under `$TMPDIR`.
- Model `gpt-5.6-luna`, effort `low` (the Plan mode switch moved it to `medium`). One run used `--enable multi_agent_v2` on the same model. Short prompts.
- gx skills were copied from `main` into the temp `.agents/skills`. No gx tracker was used, and no skill ran against a real epic.
- No 429, usage-limit or capacity error appeared in any run.

## 1. Skills note (ticket 02)

### Discovery and invocation

| Finding | Old note (0.147.0) | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Skills in `~/.agents/skills` are found | 24 of 26 gx skills listed | Observed. `codex debug prompt-input` lists 24 gx skills. | Same |
| Skills in both `~/.agents/skills` and `$CODEX_HOME/skills` | Each listed twice | Observed. Two skills copied to `$CODEX_HOME/skills` were listed twice (26 entries). | Same |
| Symlinked skill | Symlink to a skill directory works. Symlink as `SKILL.md` does not. | Observed. A directory symlink was listed. A skill whose `SKILL.md` is a symlink was not. | Same |
| `allow_implicit_invocation: false` | Hides the skill. `disable-model-invocation: true` is ignored. | Observed. `gx-wayfinder` and `gx-to-spec` (the two with `agents/openai.yaml`) are missing from the catalog. `gx-design`, `gx-implement` and the other flagged skills are listed. | Same |
| gx state on `main` | Only 2 flagged skills had the yaml | Observed in the copied bundle. 15 skills carry `disable-model-invocation: true`. Only `gx-wayfinder` and `gx-to-spec` have the yaml. The other 13 show up in the catalog. | Same (gap still open) |
| `/gx-design foo` | `Unrecognized command`, text stays in the composer | Observed. `Unrecognized command '/gx-design'. Type "/" for a list of supported commands.` Text stays in the composer. | Same |
| `$gx-unslop` alone | First Enter inserts the mention. Second Enter not tried. | Observed. The first Enter inserts, the composer keeps `$gx-unslop`. The second Enter submits and the rollout gets a `<skill>` message with the path. | Same, and the second Enter is now confirmed |
| `$name` followed by text | Not tested in the TUI | Observed. `$gx-verify then reply with just the word ok ...` typed once and submitted with one Enter. The rollout got the `<skill>` message for `gx-verify`. | New observation, no conflict |

### Subagents

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Tool shape on `gpt-5.6-luna` | `multi_agent_v1__spawn_agent` with `message`, `fork_context`, `model`, `reasoning_effort` | Observed. Same name. The call used `fork_context`, `model` and `reasoning_effort`. | Same |
| Tool shape with `multi_agent_v2` | `collaboration.spawn_agent` with `task_name`, `message`, `fork_turns`, `model`, `reasoning_effort` (seen on sol) | Observed with `--enable multi_agent_v2` on luna. The model listed exactly those parameters. | Same |
| Override takes effect | `(gpt-5.6-luna medium)` shown for the child | Observed (v1). `Spawned ... (gpt-5.6-luna medium)`. | Same |
| Bad model name | Untested | Observed on v1 and v2. The call fails with ``Unknown model `no-such-model-xyz` for spawn_agent. Available models: gpt-6.1-sol, gpt-6-astra, gpt-6.1-terra, gpt-6-sol, gpt-6-luna``. The list does not include `gpt-5.6-luna`, yet that name was accepted in the same session. A skill cannot rely on the list to pick a valid name. | New |
| Full-history fork with a `model` override | Source hint says a full-history fork "does not accept overrides". Run-time behaviour untested. | Observed. v1 with `fork_context: true`, `model: "gpt-5.6-luna"`, `reasoning_effort: "medium"` was accepted. v2 with `fork_turns: "all"` and the same two overrides was accepted, and the child's `turn_context` shows effort `medium`. The model I passed equals the parent's, so only the effort override is proven on a full fork. | Changed against the source hint. No rejection at run time. |

### Rounds, `context-window`, symlinked cwd

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| `gx-grilling` round format | 2 segments, 2 fenced blocks, `HasMarker` true | Observed. The last `final_answer` message parsed with `grillround`: `HasMarker` true, 4 segments, 4 fenced blocks. Luna asked 4 questions although I asked for one round. | Same |
| `phase` on assistant messages | `commentary` and `final_answer` | Observed. The rollout's assistant messages carry `phase: final_answer`. | Same |
| Symlinked cwd bug | `gx agent context-window` fails with `no codex session <id> found for <cwd>` when the shell cwd is `/var/folders/...` and the rollout stores `/private/var/...` | Observed with gx `v0.43.0-4-g3651c665` (the current `main`). The Codex shell's `pwd` is `/var/folders/...`, `pwd -P` is `/private/var/...`, and the rollout's `cwd` is the `/private` form. Without `--cwd` the command fails. With `--cwd "$(pwd -P)"` it returns a count (18602, then 19019). | Same, bug not fixed on `main` |
| Claude variable leak | `CLAUDE_CODE_SESSION_ID` in the Codex shell gives `ambiguous agent session` | Observed. My own shell had the variable and Codex passed it on, so the error reproduced. The cause is whatever launches Codex, not Codex itself. | Same |
| `--fits` | Fails closed when `context-window` fails | Observed, partly. `--fits chart` printed `reading auto-advance for gx session ...: dial unix .../server.sock: no such file` and exited 1, because no gx server ran. `--fits gx-design` is rejected (it wants a step name such as `chart`). I could not test `--fits` with a live server. | Not comparable |

### Sandbox, approvals and commits

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Write outside the workspace roots, inside the sandbox | Untested (429) | Observed. `echo x > /Users/david/rt166-probe-x.txt` failed with `zsh:1: operation not permitted`. No file was created. | New |
| Approval dialog for an outside write | Untested | Observed. When the model asks for escalation the TUI shows `Would you like to run the following command?`, `Environment: local`, the model's `Reason:`, the command, and options `1. Yes, proceed (y)`, `2. Yes, and don't ask again for commands that start with ... (p)`, `3. No, and tell Codex what to do differently (esc)`. The title reads `Action Required`. Esc denied it (`You canceled the request`) and no file was created. | New |
| Default writable roots | Not stated | Observed with `codex sandbox -c sandbox_mode="workspace-write"`. A file under `$TMPDIR` but outside the repo was written. `exclude_tmpdir_env_var=true` and `exclude_slash_tmp=true` made `$TMPDIR` writes need approval. | New |
| `git commit` in a plain checkout | Fails with `.git/index.lock: Operation not permitted`. `--add-dir <repo>/.git` fixes it. | Observed. Fails the same way. With `-c sandbox_workspace_write.writable_roots=["<repo>/.git"]` the commit succeeds. I tested the config key, not the `--add-dir` flag. | Same |
| `git commit` in a linked worktree of a bare repo | Worked with no extra flag | Observed, three layouts (`bare.git`, `.bare`, and a TUI run in a worktree). Commit fails: `Unable to create '<bare>/worktrees/<name>/index.lock': Operation not permitted`. Writes elsewhere in the bare directory succeed. Only `worktrees/<name>` is read-only. Adding that gitdir (`git rev-parse --git-dir`) as a writable root makes the commit succeed. Adding the bare repo root alone does not. | **Changed.** I could not reproduce the old result. Source: `default_read_only_subpaths_for_writable_root` in `codex-rs/protocol/src/permissions.rs` has the same gitdir-pointer rule at `rust-v0.147.0` and `rust-v0.166.0`, so the old run probably used a different layout or extra root. |

### Background terminals

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| `exec_command` with a long command | Untested | Observed. `sleep 90` kept running after the turn ended. Title went back to `Ready`. The footer showed `1 background terminal running · /ps to view · /stop to close`. A finished turn with a live terminal blocked nothing I tried. | New |
| `/ps` | Untested | Observed. Prints `Background terminals` and `• sleep 90`. | New |
| `/stop` | Untested | Observed. Prints `Stopping all background terminals.` The `sleep` process was gone from `ps` afterwards. | New |

## 2. Signals note (ticket 01), claims tagged only O147

### PermissionRequest hook

Hooks were inline (`-c hooks.PermissionRequest=[...]`) and ran with `--dangerously-bypass-hook-trust`. The hook read a control file, so one fixed command covered every case. The timeout was 4 s.

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Allow | Runs the command, no dialog | Observed. No dialog, the file was written. | Same |
| Deny with message | Command does not run, turn continues. TUI showed `PermissionRequest hook (blocked) feedback: <message>`. Model sees `Rejected("<message>")`. | Observed. Command did not run, turn continued, `Stop` fired. The TUI shows `Blocked by hook` and the message on the next line. The model's tool error is ``exec_command failed: CreateProcess { message: "Rejected(\"<message>\")" }``. | Behaviour same, TUI wording changed |
| Exit code 2 with stderr | Deny (Source) | Observed. Same as a deny. The stderr text appears under `Blocked by hook` and in the `Rejected(...)` error. | Same, now observed |
| Hold | A 5 s wait kept the TUI on `Working` with no dialog, then allow ran the command | Observed with a 2 s hold. No dialog appeared, then the command ran. | Same |
| Timeout | Source only: normal dialog | Observed. TUI shows `Hook failed` and `hook timed out after 4s`, then the normal approval dialog. | Same, now observed |
| Exit 1 | Source only: normal dialog | Observed. `Hook failed`, `hook exited with code 1`, then the normal dialog. | Same, now observed |
| Invalid JSON | Source only: normal dialog | Observed. `Hook failed`, `hook returned invalid permission-request JSON output`, then the normal dialog. | Same, now observed |
| Empty output, exit 0 | Normal dialog | Observed. Normal dialog. | Same |
| Which tools fire it | Not stated | Observed. `PermissionRequest` and `PreToolUse` fired for the shell (`tool_name` `Bash`) and for `apply_patch` (`tool_name` `apply_patch`). Neither fired `PermissionRequest` for `request_user_input`. | New |

### Approval flags

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Values for `--ask-for-approval` | The note launches with `--ask-for-approval on-request`. Other values not discussed. | Observed. `codex --help` lists only `on-request` and `never`. `--ask-for-approval untrusted` is rejected (`possible values: on-request, never`). `-c approval_policy="untrusted"` fails with `approval_policy = "untrusted" is no longer supported; remove this setting`. | **Changed** against older Codex, where `untrusted` existed. |

### `request_user_input`

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| Plan mode only | Default mode needs `--enable default_mode_request_user_input` (under development) | Observed. The tool schema is in the Default-mode tool list with the description `This tool is only available in Plan mode.` The developer text says to use it only when it is listed. When I asked for a call in Default mode the model declined and said it is "not callable in Default mode". It made no call, so I did not see a rejected call. `codex features list` still shows `default_mode_request_user_input` as under development, off. In Plan mode (`/plan`) the call worked. | Same |
| More than one question | Untested | Observed. Two questions gave `Question 1/2 (2 unanswered)`, then `Question 2/2 (1 unanswered)`. Digit `1` answered question 1 and moved on. Digit `2` on question 2 submitted both. Result: `{"answers":{"preferred_color":{"answers":["Red"]},"preferred_pet":{"answers":["Dog"]}}}`. The tool description says one to three questions. | New |
| `PreToolUse` payload | `questions` array, ~150 ms before the dialog | Observed. `tool_name` `request_user_input`, `tool_input.questions[]` with `id`, `header`, `question`, `options[{label,description}]`. | Same |
| `PostToolUse` payload | Answer in `tool_response` | Observed. `tool_response` is the JSON string above. | Same |
| Plan mode in hook payloads | Not stated | Observed. `permission_mode` reads `default` in every payload, also during Plan mode. A hook cannot tell Plan mode from it. | New |
| Switching to Plan mode | Not stated | Observed. `/plan` printed `Model changed to gpt-5.6-luna medium for Plan mode`. The effort moved from `low` to `medium`. | New |

### Startup, timing, errors

| Finding | Old note | 0.166.0 | Same or changed |
| --- | --- | --- | --- |
| `SessionStart` timing | At the first prompt, not at launch | Observed. No hook fired during 6 s at launch. `SessionStart` (`source` `startup`) fired about 0.4 s after the first Enter, next to `UserPromptSubmit`. | Same |
| Typing while "Hooks need review" is up | Enter chose "Review hooks"; a prompt sent early is lost | Observed. Typed text (`hello there`) was discarded and the composer stayed empty. Enter opened the hooks review screen (`t trust all · enter review · esc close`). The pane title was the terminal default, with no session id. | Same |
| `--dangerously-bypass-hook-trust` with inline hooks | Source only | Observed. Inline hooks for six events ran with no review dialog and no stored trust. The tool sandbox did not refuse the flag. The footer showed `2 warnings` where a run without the flag showed 1. I did not read the warning text. | Confirmed |
| `developer_instructions` on resume | Instruction survives a resume without the flag. Passing it again untested. | Observed. `codex resume <id> --cd <dir> -c developer_instructions="..."` answered the secret word. The rollout holds one developer item with the text, before and after the resume. I did not inspect the request payload, so a duplicate sent to the model is not ruled out. | Same, partly. No duplicate in the rollout. |
| 429 and model at capacity in `task_complete.error` | Seen on 0.147.0 | None occurred. I cannot cause one without spending quota against a limit. | Untested |
| Turn end against the screen hint | Hint vanishes up to seconds before the turn ends | Observed. A 25-line answer: the `esc to interrupt` hint left the screen at +2.0 s. The title stayed `Working` until +3.6 s, in the same poll as the rollout's `task_complete`. The `Stop` hook ran within about 50 ms of `task_complete` (my hook logged after its own start). | Same |
| Esc during answer streaming | 0.147.0 ignored it. One 0.166.0 run interrupted. | Observed twice more on 0.166.0. After the hint left the screen and the title still said `Working`, Esc produced `turn_aborted` and the title read `Ready` within about 0.12 s. The screen shows `Conversation interrupted`. I did not check the `Stop` hook for these two runs. | Same as the earlier 0.166.0 run. 0.147.0 not re-run. |
| Apply-patch approval dialog | Not seen | Observed. Text: `Added <path> (+1 -0)`, `Would you like to make the following edits?`, `Description: Apply proposed file edits`, `Destination: <path>`, options `1. Yes, proceed (y)`, `2. Yes, and don't ask again for these files (a)`, `3. No, and tell Codex what to do differently (esc)`. Esc denied it and ended the turn. | New |

## 3. Findings that changed

1. Commit in a linked worktree of a bare repo fails under `workspace-write`. The old note said it worked. The fix that worked is a writable root on the worktree's own gitdir. A plain checkout needs `<repo>/.git`. This reaches ticket 05 (launch and sandbox flags). Every ticket and design worktree gx launches needs the gitdir added.
2. `--ask-for-approval` takes only `on-request` and `never`, and `approval_policy = "untrusted"` is an error. Any launch or test recipe that used `untrusted` to force approvals no longer works. To get an approval dialog, ask the model to run something outside the sandbox. This reaches ticket 05 and any test rig for tickets 01 and 05.
3. A full-history fork accepted `model` and `reasoning_effort` on v1 and v2. The source hint said it would not. The skill edit that tells skills to set `fork_turns` to `none` for a tier override still works, but it is no longer required to avoid a rejection. Ticket 02's follow-up skill edits can drop the rejection worry.
4. The denial message on screen changed from `PermissionRequest hook (blocked) feedback: <message>` to `Blocked by hook` plus the message. The model still sees `Rejected("<message>")`. A screen parser for ticket 01 or 05 must match the new text. Hook-based flows are not affected.
5. `permission_mode` is `default` even in Plan mode. A hook cannot detect Plan mode from the payload.

## 4. Untested

- 429, model at capacity and usage-limit errors. None occurred.
- Anything on 0.147.0. There was no binary to compare, so "same" means same as the old note.
- `gx agent link`, `gx agent step` and `gx agent context-window --fits` with a live gx server.
- A forced `request_user_input` call in Default mode. The model declined to call it.
- A full-history fork with a model different from the parent's.
- The `--add-dir` flag itself. I tested the `sandbox_workspace_write.writable_roots` config key.
- `~/.codex/prompts`, `gx skills install`, a full `gx-design` chain, and review, spec, code-review and merge runs.
- The "Update now" dialog. I kept the update check off on purpose.
- Whether a duplicate `developer_instructions` reaches the model on resume.
