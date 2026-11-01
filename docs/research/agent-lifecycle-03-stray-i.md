# agent-lifecycle/03: the stray `i` and `ErrInputNotEmpty`

Ticket: `gx:agent-lifecycle/03`. Code read on branch `agent-lifecycle-design`. Nothing was changed.
Claims marked (unverified) rest on Claude Code behavior that this repo cannot show.

## 1. How `ReadyToType` decides to press `i`

- `claudeHarness.ReadyToType` (`harness/vim.go:21-42`):
  1. `claudeVimMode` false -> return true (`:22-24`). It re-reads `editorMode` from
     `.claude.json` and `settings.json` on every call (`:77-99`).
  2. `insertMarkerShown` true -> return true (`:25-27`).
  3. Otherwise log, send `i`, poll `insertMarkerShown` every 50ms for 1s (`:28-41`).
     No marker after 1s -> return false ("do not type").
- `insertMarkerShown` (`:48-62`) is true only if the rows exist, a row starting with `❯` is found
  (`lastInputRow`, `harness/typing.go:118-127`), and some row below it contains `-- INSERT --`.
  A read error, no `❯` row, or no marker all mean "not INSERT". The doc comment says why: "typing
  into NORMAL mode is the worse mistake" (`:46-47`).
- So the code treats "marker missing" as "NORMAL". Nothing in it detects NORMAL. It only
  detects the marker.
- Ways the marker can be missing while Claude is in INSERT (the first two follow from the code; the
  rest are unverified):
  - `lastInputRow` matches any row that starts with `❯`: the dialog cursor (`❯ 1. Yes`) or a user
    message echoed in the transcript. No input box on screen, so no marker below it.
  - The screen is read before Claude redraws. Callers act on the pid-file "idle" status, not on
    the screen (`holdersock.go:329`). There is no settle wait, so one stale frame decides.
  - The footer is replaced by a transient hint (interrupt, "press again", queued message), by
    background-task rows, or truncated at a narrow width (unverified).
- The check is a single frame: `insertMarkerShown` has no "stable for N ms" rule.
- Only one real Claude screen with the marker is in the repo:
  `agentrunner/claudeui/testdata/startup-w250.screen.txt:29-30`. The marker shares a row with
  `⏵⏵ auto mode on ...`. Row 29 (`INSERT ...`) is gx's own statusline, not Claude's footer. The
  spec says the same (`docs/specs/session-prompting.md:262-264`).
- The unit tests use synthetic rows (`harness/typing_test.go:183-198`). No test has "marker absent,
  Claude in INSERT". The fake input box (`testutil/agentfake/programinput.go:92-110`) only models
  NORMAL as the absence of the marker, so it cannot show the flaw.

## 2. Callers that can press `i` while Claude is in INSERT

All of them can. What differs is what runs after the `i`.

| # | Caller | After the `i` | Result if `i` was typed |
|---|---|---|---|
| 1 | `server/firstprompt.go:252` (`deliver`) | `clearDraft` (`server/sessiontyping.go:38-68`), then `PromptWithNudge`, which calls `ReadyToType` again (`claudeui/nudge.go:174`) with no clear after it | First call is cleaned up by `clearDraft` (only if the holder serves styled rows, `:75`). The second call is not: `i` prefixes the prompt. |
| 2 | `server/sessionaction.go:178` (`typeStep`) | `clearDraft` at `:182` | Cleaned up. On give-up no clear runs (`:179`). |
| 3 | `nativerunner/holdersock.go:333` (`denyReason`) | Nothing; writes the reason at `:340-348` | Reason is sent as `i<reason>`. Runs right after Esc, the "right after an interrupt" case in the ticket. Highest risk. |
| 4 | `nativerunner/pty.go:747` -> `ptyPrompter.ReadyToType` (`pty_personinput.go:153`) via `nudge.go:174` | `awaitEmptyInput` ran before the step (`pty.go:738`). Nothing after. | `Submit` types onto the `i`: `i<prompt>`. A retype's `clearWhole` would clear it. |
| 5 | `agentrunner/herdrrunner/nudge.go:52` -> `herdrPrompter.ReadyToType` (`:120`) | No draft check at all | Same as 4. |

Two more routes reach the same step (`nudge.go:141`, `:174`, `:203`):
- `server/sessionsend.go:60-77` (`nudgePrompt`) uses `PromptWithNudge` with no `clearDraft`.
- `ClearInput` runs `ReadyToType` first (`nudge.go:140-145`). `clearLeftover` (`firstprompt.go:169`)
  calls it on the failure path, so recovery can press `i` again.

**A give-up leaves the `i` behind.** On the 1s timeout (`vim.go:41`) nothing removes the `i`. The
callers treat it as "NORMAL mode": `failureReason` returns `"input is in vim NORMAL mode"` with
`clearNeeded=false` (`firstprompt.go:146-147`), and `sessionaction.go:179` just errors. The screen
then shows `❯ i`.
- This is the live evidence in `~/.local/state/gx/server.stderr:12692-12693` (Oct 29, 13:05:08 and
  13:05:13): "pressing i", then "no INSERT marker after pressing i, not typing". No caller is named,
  and no `server.log` line sits next to it.

## 3. A positive NORMAL signal

Claude draws no NORMAL marker, so none exists on screen (`session-prompting.md:262-264`). No real
Claude screen of NORMAL, of INSERT mid-turn, or of the state after an interrupt is in the repo.
Saved agent screens on this machine (`~/.local/state/gx/agents/*/screen.txt`, 49 files) have no
vim markers at all, so they cannot be mined either.

Capture facilities, for the capture step the ticket asks for:
- `gx tickets agents watch <epic>/<id> --screen` (`cmd/server_agents.go:90,119`) prints the live pty
  screen, or the saved `screen.txt` (`nativerunner/holder.go:31`).
- Holder ops `screen` and `screen-styled` (`nativerunner/holdersock.go:54,256,269`, client `:594,601`).
- herdr `read` with `recent-unwrapped` (`herdrrunner/nudge.go:160-167`).

Candidate signals, best first:

1. **Probe by effect (works for every runner, no config).**
   Snapshot the input text, press `i`, re-read.
   - Marker appears and the text is unchanged: it was NORMAL. Done.
   - Text grew by `i` (placeholder gone, `❯ i`): it was INSERT. Send Backspace (`\x7f`), verify the
     text is back, and return true.
   - Neither: unknown. Press nothing more and give up. Remove nothing blindly.
   - Reason: `i` has two disjoint effects. In NORMAL it only changes mode. In INSERT it only adds a
     character. The input row tells them apart. This also makes the give-up path clean: gx knows
     whether its own `i` is in the box.
   - Risk: a Backspace in NORMAL moves the cursor, which is harmless (unverified). Do the Backspace
     only after seeing the extra `i`.
2. **Claude's own `vim.mode` (pty runner only).**
   - gx already parses `vim.mode` from the statusline payload (`cmd/claude_statusline.go:64-66,148`).
   - The pty runner already injects its own `statusLine` command with `--agent-dir`
     (`nativerunner/pty.go:437-439`), and the command already writes `usage.json` into that dir
     (`cmd/claude_statusline_usage.go:43-56`).
   - It could also write `{mode, seen_at}`; `ReadyToType` would read it as a positive NORMAL signal.
   - Gaps: herdr runs the person's own statusline, so there is no feed there. Statusline refresh
     timing after a mode change (Esc, interrupt) is unverified, so a stale file is a hazard.
     Use it as a hint only, and only trust it when `seen_at` is newer than the last key press.
3. **Settle before deciding.** Require the same frame for about 150-300ms, with an input row
   present, before treating "marker missing" as meaningful. This removes the stale-frame and
   mid-redraw cases but not the hidden-footer ones.

Not recommended: scraping the `NORMAL`/`INSERT` word from the statusline row. It exists only with
gx's statusline and its glyph padding (`claude_statusline.go:137-166`).

## 4. Why the slog line never reaches `gx.log`

- `harness/vim.go` calls the package-level `slog.Info/Warn` (`:28,31,40`). That is Go's default
  logger: text lines on stderr, `INFO` level, no session or caller attributes.
- The server writes through its own handler, `s.log`: JSON into `server.log`
  (`server/server.go:507`, `server/logfile.go:11,19`), read by `gx server logs`. Default level is
  Info (`cmd/server.go:172-178`). The default `slog` is never replaced (`rg SetDefault` has no hit
  in non-test code).
- `gx.log` is a third file: the bubbletea `logger.Debug` file (`logger/logger.go:33`), not used by
  the server's slog.
- Where the line actually goes depends on how the server runs:
  - Under launchd (`com.elentok.gx.server` is loaded), stderr is `server.stderr` (`cmd/server_launchd.go:205-208`).
    That is where the one hit is.
  - Spawned detached, stderr is `server.log`, but as a plain text line among JSON lines
    (`cmd/server.go:513-515`).
- The spec wanted a debug line kept in the server log (`session-prompting.md:225`, `:385`). The
  implementation used the global logger instead.
- Same flaw: the `slog.Debug("input not empty after full clear", "screen", ...)` at
  `claudeui/nudge.go:133`. It is below the Info level and on the default logger, so the pane capture
  the ticket asks for ("capture the pane screen") is thrown away.
- `ReadyToType` has no logger parameter and no caller name. `TypingIO` (`harness/typing.go:46-49`)
  has only `Rows` and `SendKeys`. Each caller would need to pass a logger and a label.

## 5. Where `ErrInputNotEmpty` comes from

The sentinel is built in only two places, and both print the same text,
`submission never reached the pane: input not empty after clearing`:

1. `clearWhole` (`claudeui/nudge.go:111-135`): 2N+1 Ctrl+U presses, then up to 1s of re-reads. Reached
   from the retype branch (`:209`) and from `ClearInput` (`:144`). `InputIsEmpty` (`harness/typing.go:131-160`)
   says NotEmpty when any visible, non-dim, non-reverse rune follows `❯`.
2. `ptyPrompter.awaitEmptyInput` (`nativerunner/pty_personinput.go:89-107`): waits `promptGrace` for
   a person's draft to go, before anything is typed. It never clears. Called only from
   `Pty.PromptNudged` (`nativerunner/pty.go:738`).

The retro lines (`stall nudge ...: prompt not delivered: ... input not empty after clearing`,
"the first sessions never took a turn") come from `PromptNudged`. That is path 2: no key was typed,
and `clearWhole` is not even reachable until a retype. The wording "after clearing" is misleading
there. A lone `i` makes the error in two ways:
- An earlier give-up left an `i` (section 2), and the loop's Retry then meets a non-empty input.
- Or an earlier delivery's `i`+prompt stayed unsent.
The third possibility is that `InputIsEmpty` misreads a fresh startup row (a non-dim rune). That
needs the real screen. Today the two sites cannot be told apart from the error, and neither writes
the screen to a log.

## 6. Is auto-retry-once safe?

| Site | Safe? | Notes |
|---|---|---|
| `clearWhole` in the retype branch (`nudge.go:209-214`) | Yes | It returns at once instead of using the remaining `Retypes`. Ctrl+U on an input gx just typed into is idempotent. Safest fix: on `ErrInputNotEmpty`, do one more clear round (or `continue` the attempt loop) before `NudgeStuck`. Needs `ReadyToType` to have passed, which `:203` already does. |
| `clearWhole` via `ClearInput` / `clearLeftover` (`firstprompt.go:169`) | Yes | Same clear-only action. Already a best-effort recovery. |
| `awaitEmptyInput` (`pty_personinput.go:98`) | Not blindly | It protects a person's draft. A retry that clears could delete it. Only gx's own residue is safe to remove. Two safe options: re-wait once with a screen capture; or clear only when the whole input text is exactly `i` or equals the last prompt gx typed. |
| The loop (ralphloop) above `PromptNudged` | Yes | `ErrNotDelivered` means no new turn started (`agentrunner/agentrunner.go:192`), so one automatic re-`Prompt` before pausing and spawning an investigate ticket cannot double-send. Gate on `ErrInputNotEmpty` + `ErrNotDelivered`, and have the retry log the captured screen. |

If the section 3 probe lands, `ErrInputNotEmpty` from gx's own `i` stops happening, and the retry
only covers real drafts and misreads.

## 7. Other harnesses

- Codex: `codexTyping.ReadyToType` is a no-op (`harness/typing.go:102`) and the test pins that
  (`typing_test.go:239-246`: no reads, no keys). Codex has no `i` step, so it cannot have this
  flaw today. If a Codex vim composer is ever added, it must reuse the shared probe, not copy
  `vim.go`.
- `ErrInputNotEmpty` / `clearWhole` / `awaitEmptyInput` are harness-neutral. Codex uses them with
  `ctrl+u` and a bold `›` (`typing.go:81`). A retry in `clearWhole` helps Codex too. `awaitEmptyInput`
  is only in the pty runner, where Codex also runs (`pty.go:379` `ptyKind`).
- herdr: the plain `Screen` read is `recent-unwrapped` (`herdrrunner/nudge.go:161`), so the "last
  `❯` row" scan can pick a scrollback row when the live box is hidden. The probe should read the
  styled screen there as well.

## 8. Recommended fix (in order)

1. Replace the press-`i`-and-hope with the probe in section 3 (snapshot, press, compare, Backspace
   if `i` was typed). On a genuine give-up, leave the box as it was. Add settle (3.3) in front.
2. Make `ReadyToType` take a logger and a caller label via `TypingIO` (or a returned reason), and log
   through `s.log` at Info, with the decision (`marker`, `normal->insert`, `insert-typed-i`,
   `unknown`). Log the captured screen when the probe is unknown or `clearWhole` fails.
3. In `PromptWithNudge`, let `clearWhole` failure use one more round before `NudgeStuck`
   (`nudge.go:209`); give `awaitEmptyInput` its own error text and a screen capture.
4. Ralph loop: one automatic retry on `ErrNotDelivered`+`ErrInputNotEmpty` before pausing.
5. Tests: marker absent while INSERT (assert the input is back to empty and no `i` stays); NORMAL
   with a draft; a give-up leaves the input unchanged; a `clearWhole` retry. Add a fake knob for
   "INSERT without marker" in `testutil/agentfake/programinput.go`.

## Open design questions

- Is the statusline `vim.mode` file worth adding (pty only), given the probe already covers both
  runners? It is extra state with a staleness risk.
- Should `awaitEmptyInput` be allowed to clear a lone `i`, or should it only ever wait?
- Is one extra automatic retry in the loop acceptable before the investigate ticket, or should the
  investigate ticket still open with a captured screen first?
- Real captures (NORMAL, INSERT mid-turn, after interrupt) are still missing. Take them with
  `gx tickets agents watch --screen` on a vim-mode pty agent before fixing the hidden-footer cases.
