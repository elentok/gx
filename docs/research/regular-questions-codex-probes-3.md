# Codex probes, round 3: unknown feature keys, resume/fork under `--no-daemon`, overlay cursor

Ticket `gx:regular-questions/12`. Follow-up to `regular-questions-codex-claude-probes.md` (07) and `regular-questions-codex-claude-probes-2.md` (09); raised by the design review of `docs/specs/regular-questions.md`.

Version: **codex-cli 0.167.0**, model GPT-6.1-Sol, tmux 120x40, macOS. **0.166.0 was not run**: it is no longer installed (the global Codex was upgraded in round 2) and the answers needed only the installed binary. Every run used a scratch cwd (`<scratchpad>/work/cx`) with `CODEX_HOME=<scratchpad>/ch` (copied `auth.json`, a `config.toml` that trusts the cwd and sets `check_for_update_on_startup = false`), never the repo. Scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-regular-questions-design/de133784-1bf3-4563-9cd0-fbf51238279a/scratchpad/` (`doctor.json`, `exec_unk.txt`, `c3_*.txt`, rollouts under `ch/sessions/2026/11/10/`). Claim tag **[run]** = seen in a real run. The launch is the one from 09: `codex --no-daemon -c features.default_mode_request_user_input=true`. The scratch home's managed daemon (started by a plain `exec`) was stopped with `codex app-server daemon stop` afterwards.

## Answer in short

1. **Unknown or removed `features.<name>` key: accepted, exit 0, and warned only for an unknown name.** Interactive/`exec` startup prints `Codex is ignoring 1 unrecognized configuration setting ... session-flags: features.bogus_zzz is ignored.` (twice in `exec`). A key that is a known but `removed` feature (`apply_patch_freeform`) produced no warning at all. `codex features list` and `codex --version` print nothing extra and exit 0. So a typo in the flag name cannot be caught from the exit code.
2. **`gx codex doctor` can read the flag two ways, both verified.** `codex features list -c features.default_mode_request_user_input=true` prints the row `default_mode_request_user_input  under development  true` (`false` without the override). `codex doctor --json -c ...` has `checks.config.load.details["feature flag overrides"]` (`default_mode_request_user_input=true`, or `none`) and `["enabled feature flags"]` (a comma list containing the name). Neither needs a model call or a chat.
3. **Resume and fork work under `--no-daemon`, with the flag, and `request_user_input` still works afterwards.** `codex --no-daemon -c <flag> resume <id>` and `... fork <id>` (and the flags placed after `resume`) open straight to the chat with no dialog. Without `--no-daemon` both show the same "Background server has incompatible feature settings" dialog, and Cancel exits 1.
4. **Overlay cursor resets to the top row (row 1) on every question.** After answering Q1 on row 3 (`Down Down Enter`), Q2 opened with the cursor on `1.`. With 3 options: option k is digit `k` (or `Down` x (k-1) then `Enter`); "None of the above" is digit `4` (or `Down` x 3, or `Up` once from the top, which wraps).
5. **What gx can re-read before each question's keys:** the overlay's first line `Question N/M (U unanswered)`, the question text line under it, the numbered rows with the `›` marker on the cursor row, and the footer (`enter to submit answer` vs `enter to submit all` on the last question). The cursor row is always row 1 on a fresh question, so no cursor tracking is needed.

## 1. Unknown or removed `features.<name>` via `-c` [run]

Evidence: `exec_unk.txt`, `doctor.json`, shell output of the runs below.

| Command (cwd scratch, `CODEX_HOME` scratch) | Result | Exit |
|---|---|---|
| `codex -c features.bogus_zzz=true features list` | Output byte-identical to `codex features list` (201 lines), no warning on stdout or stderr | 0 |
| `codex -c features.bogus_zzz=true --version` | `codex-cli 0.167.0` | 0 |
| `codex --no-daemon -c features.bogus_zzz=true exec --skip-git-repo-check "Reply with exactly PONG" </dev/null` | Header, then **two** identical warnings (below), then `PONG` | 0 |
| `codex --no-daemon -c features.apply_patch_freeform=true exec ...` (a `removed` feature per `features list`) | No warning; `PONG` | 0 |
| `codex doctor --json -c features.bogus_zzz=true -c features.default_mode_request_user_input=true` | Report; the unknown key shows as `checks.config.load.details["startup warning"]`; overall run completes | 0 |

The warning text, from `exec_unk.txt` (stderr merged into the log):

```
warning: Codex is ignoring 1 unrecognized configuration setting. Check for typos or deprecated settings.
  session-flags: `features.bogus_zzz` is ignored.
```

- So the key is **ignored and warned**, not failed. The warning is printed once per config load, and `exec` loaded the config twice. In the interactive TUI the same text is presumably a status-bar warning (the TUI run with only the real flag showed `⚠ 1 warning`; the TUI was not run with a bogus key, so its rendering is **not run**).
- A removed-stage feature is a known name, so it is not "unrecognized"; the silence for `apply_patch_freeform` is that case. Whether gx's flag could ever hit it depends on a future Codex release dropping `default_mode_request_user_input` from the table; then the name becomes unrecognized and the warning above is the only signal.
- The binary's own strings contain the deprecation texts (for example `[features].transcript_v2 is deprecated and ignored`), which is where per-feature "deprecated" messages come from. None applies to `default_mode_request_user_input` in 0.167.0.

### What `gx codex doctor` can read

**Option A, features listing [run].**

```
$ codex features list | grep default_mode_request_user_input
default_mode_request_user_input          under development  false
$ codex features list -c features.default_mode_request_user_input=true | grep default_mode_request_user_input
default_mode_request_user_input          under development  true
$ codex -c features.default_mode_request_user_input=true features list | grep default_mode_request_user_input
default_mode_request_user_input          under development  true
```

Columns: name, stage (`under development`, `experimental`, `stable`, `deprecated`, `removed`), effective state. The `-c` placement before or after `features list` both work. `codex features list --help` takes `-c`, `--enable`, `--disable`. A missing row means the name is not in this build's table (so the flag would be ignored). A stage of `removed` means the row exists but is dead. Exit 0 in all cases.

**Option B, `codex doctor --json` [run].** `schemaVersion`, `generatedAt`, `overallStatus`, `codexVersion`, `checks`. Under `checks.config.load.details`:

```
"feature flag overrides": "default_mode_request_user_input=true"      (or "none")
"enabled feature flags":  "daemon_auto_start, shell_tool, ... default_mode_request_user_input, ..."
"feature flags enabled":  "67"
"startup warning":        "Codex is ignoring 1 unrecognized configuration setting ... `features.bogus_zzz` is ignored."   (only with a bad key)
```

`--json` is a "redacted machine-readable report". A plain `codex doctor --json` with no `-c` reported `"feature flag overrides": "none"` and did not list the name among the enabled flags. `doctor` also launches a network check (websocket, reachability), so it takes seconds; option A is lighter.

**Not needed / not run:** a short probe chat. Neither listing proves the overlay will fire (the model must call the tool), but both prove the flag reached the effective config, which is what the doctor check is for. A real enabled run does exercise the overlay: the `request_user_input` overlay appeared under `--no-daemon` in every probe in section 2 and 3.

Recommendation for the doctor check: run `codex features list -c features.default_mode_request_user_input=true` with the same `-c` string gx launches with, require the row to exist, stage not `removed`, state `true`. Fail with "this Codex does not know the flag" when the row is missing, and treat a `startup warning` entry (option B) as the same failure.

## 2. Resume and fork under `--no-daemon` [run]

Evidence: tmux captures during the run; rollout files `rollout-2026-11-10T15-43-50-01a1c5b3-...jsonl` (first chat and the resumed turn) and `rollout-2026-11-10T15-45-27-01a1c5b4-...jsonl` (fork).

Setup: first chat `codex --no-daemon -c features.default_mode_request_user_input=true`, prompt `Reply with exactly: PONG`, `• PONG`; exited. Session id `01a1c5b3-6231-7f21-868d-2693bab78f15` read from the rollout file name.

| Launch | Result |
|---|---|
| `codex --no-daemon -c features.default_mode_request_user_input=true resume <id>` | No dialog; the transcript of the first chat is replayed (`› Reply with exactly: PONG` / `• PONG`), input box ready, `⚠ 1 warning` |
| `codex resume <id> --no-daemon -c features.default_mode_request_user_input=true` (flags after the subcommand) | Same: no dialog, history replayed |
| `codex --no-daemon -c features.default_mode_request_user_input=true fork <id>` | No dialog; history replayed; the fork got its own rollout file (a new session id `01a1c5b4-e069-...`) |
| `codex -c features.default_mode_request_user_input=true resume <id>` (no `--no-daemon`) | The daemon dialog: `Background server has incompatible feature settings`, rows `1. Run without daemon this time`, `2. Restart with these settings (recommended)`, `› 3. Cancel` |
| `codex -c features.default_mode_request_user_input=true fork <id>` (no `--no-daemon`), digit `3` | `Error: Cannot use the shared background server: This session requires default_mode_request_user_input to be enabled.` / `To work without the background server, rerun the same command with --no-daemon (including resume or fork and its arguments).` then `EXIT=1` |

- **`request_user_input` after resume [run].** In the resumed session I asked for a 3-question `request_user_input` call. The overlay appeared (`Question 1/3 (3 unanswered)`), took keys, and Codex answered with the picks. The call and its `function_call_output` landed in the **original** rollout file (the resume appends to it: the file holds 33 lines and 12 mentions of `request_user_input`), so gx's rollout reader keeps following the same path after a resume.
- **After fork [run].** Same prompt in the fork: overlay and answers worked; the output went to the fork's own rollout file, not the parent's.
- So "same arguments as the first launch, plus the subcommand" is enough: gx should keep `--no-daemon` and the `-c` flag on resume and fork. Without `--no-daemon` the dialog returns and Cancel (the default row) kills the process with exit 1, same as for a fresh launch (09).
- Resume with the flag **dropped** (a chat first created with the flag, resumed without it) was **not run**; not part of the question.
- 0.166.0 resume/fork: **not run** (not installed any more). The 0.166.0 error text from 09 already named `--no-daemon` for resume and fork.

## 3. Multi-question overlay: cursor and keys [run]

Evidence: `c3_q1_down2.txt`, `c3_q2_start.txt`, plus the captures quoted below (taken from the fork run; the same overlay code ran in the resumed session).

Prompt: a 3-question `request_user_input` call (`a` Size: Small/Large/Huge, `b` Shape: Round/Oval/Square, `c` Mode: Fast/Slow/Mid). Every question has 3 options and so a 4th row.

**Where the cursor starts.** Q1 starts on row 1. On Q1 I pressed `Down Down` (cursor on `3. Huge`) and `Enter`. Q2 opened with the cursor on **row 1**, not on row 3:

```
  Question 2/3 (2 unanswered)
  Shape
  › 1. Round              Round shape.
    2. Oval               Oval shape.
    3. Square             Square shape.
    4. None of the above  Optionally, add details in notes (tab)
  tab to add notes | enter to submit answer | ←/→ to navigate questions | esc to interrupt
```

Q3 also opened on row 1 (capture after the note-and-Enter on Q2). So every new question starts at the top row, whatever the previous cursor position was. When I went back with `Left`, Q2 reopened on its answered row (`› 4. None of the above` with its note `› custom x`), so a revisited question keeps its state; gx should not navigate backwards.

**Keys with 3 options.**

| Target | Keys | Result |
|---|---|---|
| Option k (k = 1..3) | digit `k` | Submits that option and advances (probe 07: no `Enter`). On the last question it submits all |
| Option k | `Down` x (k-1), then `Enter` | Same result, needed when a note is wanted (`Tab` after the `Down`s, text, `Enter`; from 07) |
| "None of the above" | digit `4` (option count + 1) | Submits at once, no note (07; confirmed for a 2-option question, digit `3`) |
| "None of the above" | `Down Down Down`, `Enter` | `Enter` opens the notes line (`› Add notes`, footer `tab or esc to clear notes | enter to submit answer`); a second `Enter` (empty) or typed text then `Enter` submits. Typed `custom x` gave `answers: ["None of the above","user_note: custom x"]` |
| "None of the above" | `Up` once from row 1 | The cursor **wraps** from row 1 to the last row (capture: `› 4. None of the above` after `Up` on a fresh Q3) |

Rollout `function_call_output` for the run (Q1 `Down Down Enter` = Huge; Q2 `Down Down Down Enter`, `custom x`, `Enter`; Q3 `Up`, `Enter`, `Enter`):

```
{"answers":{"c":{"answers":["None of the above"]},"a":{"answers":["Huge"]},"b":{"answers":["None of the above","user_note: custom x"]}}}
```

The key order is not the question order (matches 07). The Codex summary printed for each question is `answer: <label>` and `note: <text>`.

**What the overlay shows that gx can re-read before each question's keys.** All from the pane text, no state needed:

- Line 1: `Question N/M (U unanswered)`. N advances by one after each answered question (`1/3`, `2/3`, `3/3`); U counts down (`3 unanswered`, `2`, `1`).
- Line 2: the question text (the `question` field, here `Size`, `Shape`, `Mode`), which gx can compare with its own question list.
- Option rows: `<marker> <n>. <label>   <description>`. The marker is `›` on the cursor row, a space otherwise. The last row is always `<count+1>. None of the above  Optionally, add details in notes (tab)`, so gx can read the option count from the number of rows.
- Notes line: appears as `› Add notes` (or `› <typed text>`) under the list once Tab or Enter-on-None opens it; the footer then reads `tab or esc to clear notes | enter to submit answer`.
- Footer: `tab to add notes | enter to submit answer | ←/→ to navigate questions | esc to interrupt`; on the last question `enter to submit all` replaces `enter to submit answer`.

Use: before sending the keys for question N, read `Question N/M` and the question text to confirm the overlay is on the expected question, and read the row count from the numbered list. Because the cursor always starts on row 1, digit keys (or `Down` x (k-1)) are position-independent; no cursor re-read is needed for correctness, only the header check for sync. If the header's N is not the expected one (for example a late redraw), wait and re-read instead of sending keys.

## Could not determine

- **0.166.0**: none of the three parts run on it (not installed; round 2 already covered its dialog and `--no-daemon`).
- The TUI rendering of the unrecognized-key warning (only `exec` and `doctor --json` run); a known-but-removed key warns nothing in `exec`, TUI not checked.
- Resume with the flag removed, and resume after the daemon dialog's option 1 (`Run without daemon this time`).
- Digit keys on the 2nd and 3rd questions of this specific 3-option run were not re-pressed in this round (07 covered digits; this round used `Down`/`Enter`); cursor-start behavior was observed on the 2nd and 3rd questions.
