# gx:webui-agent-views/03 - why a fresh Claude pane stops taking keys after a `/` launch prompt

Question: what makes Claude stop taking keys after gx types a launch prompt that starts with `/`
(for example `/gx-implement gx:follow-ups-2026-11-01/08`)? Is it the slash-command menu, vim INSERT
mode, typing before startup finished, or a burst of launches? Can it be reproduced, and which change
to how gx types or submits the prompt stops it?

Sources: the gx code at branch `webui-agent-views-design` (HEAD `1d64138a9`), the 1,294
`~/.local/state/gx/agents/*/*/raw.log` files on this machine, the run logs under
`~/.local/share/gx/tickets/gx/*/run-log.jsonl`, and the installed `claude` (2.1.319, driven in
scratch tmux panes). Claims marked (inferred) are not proven. "Verified" means I read the bytes or
ran the command myself.

## Summary

- **Verified:** every hung pane had Claude's vim mode on. 17 of 191 sessions with a `-- INSERT --`
  footer hung (9%). 0 of about 1,100 sessions without it hung, including 219 on the same Claude
  v2.1.316 build. Vim mode is the only variable I found that separates them.
- **Verified:** the slash-command menu is not the cause. No menu rows were drawn in any of the 17
  hung screens, and a menu does not show once the command name is followed by a space.
- **Verified:** it was not a simultaneous burst. The "burst" was a long serial run, with hung and
  healthy launches interleaved.
- **Not reproduced.** 24 scripted launches with Claude 2.1.319 in vim mode all worked. So the exact
  mechanism inside Claude is unproven.
- gx already ships the likely cure: `agent-lifecycle-impl/13` (commit `8c6ae8ba6`, 2026-11-01 23:27)
  launches every Claude with `editorMode: normal`. No hang appears in any session launched after it
  took effect (2026-11-02 about 08:10).

## 1. What gx types and how (verified, from code)

- The launch prompt for Claude is a plain key write, not a paste: `claudeHarness.PastesText` is
  always false (`harness/typing.go:122`), and `HolderClient.typeText` pastes only above
  `typedTextMax` (512 bytes) or when asked (`nativerunner/holder/holdersock.go:636-660`). The 43
  byte prompt arrives in one write.
- Enter is a separate write afterwards: `ptyPrompter.Submit` types, then `SendEnter` does a
  `Watched()` round trip and sends `\r` (`nativerunner/pty_personinput.go:165-183`, `:252-264`).
- A `/` prompt takes `Pty.PromptLaunch` (`nativerunner/pty_launch.go:30`): wait until not busy,
  `awaitEmptyInput`, `Submit` once, then `claudeui.CheckLaunchDelivered` proves the command through
  the transcript's first user record and resends every 3 s for 30 s (`agentrunner/claudeui/launchproof.go:48-54`).
  The older `PromptNudged` slash branch keeps the single-send rule (`nativerunner/pty.go:451`,
  `isSlashCommand` at `:475`).
- Ready detection: `ptyLauncher.launch` waits for the holder's `Ready` signal (pid file idle, OSC
  7501 or screen) before returning (`nativerunner/pty_launch.go:141-250`). Nothing waits a settle
  time after that. The only extra pre-type step for Claude is the vim probe
  (`harness/vim.go:15`, `harness/vimprobe.go`), which returns true at once when the `-- INSERT --`
  marker is on screen.
- There is no concurrency throttle on launches that depends on Claude startup load. Launches
  are serialised by the ralph-loop claim step (run log below).
- The clear keys that fail on a hung pane are Ctrl+U presses (`claudeui.clearWhole`,
  `agentrunner/claudeui/nudge.go:121`).

## 2. The evidence log

`~/.local/state/gx/agents/gx/follow-ups-2026-11-01-iter-08/raw.log` holds two Claude runs back to
back (845,099 bytes). The ticket read it as one. The second `\x1b[?1049h` (alt-screen enter) is at
byte 3,828.

- Bytes 57 to 3,828 are the hung run. Claude v2.1.316 draws its startup screen once, including the
  dim placeholder `Try "refactor server.ts"` and the footer `-- INSERT --  ⏵⏵ auto mode on`. It
  then queries the terminal (`\x1b[>0q`, `\x1b[?u`, `\x1b]7501;?`, `\x1b[c`). Then it draws the
  prompt: `❯ ` followed by `/gx-implement` in colour `38;2;177;185;249` and the rest of the text in
  default colour, followed only by line clears (`\x1b[K`). After that there are no more draw
  commands, only the terminal restore sequences written when the process was killed (exit code 143).
- The blue command highlight means Claude parsed the text as a known slash command, so the typed text
  went through its input handler. The Enter that gx sent next produced no output at all: no
  submit, no user line, no spinner.
- The other 16 first runs end the same way. I diffed the tail of seven of them: identical apart from
  the ticket number (`ticket-authoring-tools-impl/09`, `/15`, `follow-ups-2026-11-01/24`, `/17`,
  `/14`, `agent-lifecycle-impl/08a`, and this one).
- The second run in the same file (bytes 3,828 onward) shows the same prompt submitted normally:
  the line turns white, `Harmonizing...` appears, and the turn runs. Same ticket, same text, same
  worktree, a fresh process.

### Menu: refuted for this text form

- No suggestion rows are in the hung bytes: after the prompt line the only sequences are
  `\r\x1b[1B\x1b[K\r\x1b[51C\x1b[3B\x1b[K`, which clear lines, not draw them.
- I typed the exact prompt into Claude 2.1.319 in vim mode (`/gx-implement gx:follow-ups-2026-11-01/08`,
  no Enter). The pane showed the input line and the footer, no menu. Ctrl+U then emptied the input
  and the placeholder came back. So with an argument after the command name there is no menu to eat
  keys (verified for 2.1.319; the hung runs are 2.1.316, whose hung bytes agree).
- A menu could appear while the command name is still arriving key by key. gx writes all 43 bytes in
  one write, so Claude sees the space in the same input read. Cannot be excluded for 2.1.316 (inferred).

### Vim mode: strong association

Counted over all 1,294 raw.logs (first 20 KB of each, looking for `-- INSERT --`; hung means two
Claude runs with the first ending before byte 12,000 on a `/gx-` prompt):

| Footer has `-- INSERT --` | Hung | Not hung |
|---|---|---|
| yes (Claude in vim mode) | 17 | 176 |
| no | 0 | 1,102 |

- Same Claude build, different mode: v2.1.316 launched 82 sessions on 11-02, 83 on 11-03 and 54 on
  11-04 without a vim footer and none hung. So the Claude version is not the variable.
- Before `8c6ae8ba6` (2026-11-01 23:27) the user's own config put every Claude in vim mode. The last
  vim-mode launch on 11-02 started 08:10:37, when the server picked up the new binary. After that
  only two vim-footer sessions exist (one `conflict-08` on 11-06, one on 11-08), neither hung.
- Confound: time. Vim mode and "before the fix" are the same set, and the user's own load was highest
  in that window. Nothing here proves vim mode causes the hang. It proves no hang occurs without it.
- `ReadyToType` returned true on the marker. The old `i` theory (`agent-lifecycle-03`) is out: the
  hung input shows no leading `i`.

### Burst: refuted as simultaneous, partly as load

`agent-lifecycle-impl` run log, 2026-11-02 (`~/.local/share/gx/tickets/gx/agent-lifecycle-impl/run-log.jsonl`):
iterations started 06:47:56, 07:09:25, 07:10:53, 07:12:20, 07:13:13 and so on, 40 to 90 s apart.
08a, 08a1, 08a10, 08a12 and 08a18 hung; 08a11, 08a13 to 08a17, 08a19, 08a2 and 08a20 did not.
Healthy and hung launches sit next to each other, and no two launches overlapped at startup.
The first prompt is typed about 4 s after `iteration-started` (08 in the ticket: 21:41:18 to
21:41:22). The 12 siblings that "launched in a burst" were a long queue, not a simultaneous start.
Machine load while 6 to 12 agents ran may still raise the odds (inferred, no timing data).

## 3. Reproduction attempts (verified, none hung)

Scratch tmux panes (`tmux -L`, dir the trusted worktree), the real `claude 2.1.319 --permission-mode
auto --settings '{"editorMode":"vim"}'`, text typed as soon as `❯` appeared, then Enter as a
separate `send-keys`, then 4 s wait, Ctrl+U, 1 s wait.

- 12 sequential launches with `/zz-nonexistent gx:x/1`: 12 submitted normally.
- 4 rounds of 3 concurrent panes (12 launches, at most 3 Claude processes): 12 fine.
- `/context gx:x/1` ran and cleared as expected.
- I did not use `/gx-implement` for the Enter path: it starts a model turn and edits files.

So a fresh 2.1.319 does not hang on this input. The hang is rare (about 9% of vim-mode launches on
2.1.316) and I could not trigger it in 24 tries at one time of day with a lighter session setup
than gx's (gx adds hooks, a status line, a model and a worktree with many skills).

## 4. Ranked candidates

1. **Claude's vim-mode input handler wedges on the launch text (about 60%).** Evidence: 17/17
   hangs in vim mode, 0/~1,100 without; the process stays alive, quiet and unresponsive
   (0.2% CPU, `Ss+`), which is a stuck event loop or a handler waiting on something that never
   resolves; it receives and draws the text first. Mechanism unknown (inferred): vim mode runs a
   key state machine over a 43 key burst whose first key is `/`, and something in it can stop.
   Not reproduced, so cannot name the trigger.
2. **Typing before the input is truly ready (about 20%).** Evidence: Claude drops a key typed before
   it reads input (gx's own comment at `nativerunner/pty_launch.go:213`); the first prompt goes in
   about 4 s after launch; the Ready signal can fire on the pid file before the vim state is set up.
   Against: the text was drawn and highlighted, and the same build never hung in normal mode with
   the same timing.
3. **Slash menu (about 5%).** Refuted for the observed bytes; only possible while the command name
   is still arriving.
4. **Simultaneous burst (about 5%).** Refuted by the run log; load may be a mild amplifier.
5. Other (about 10%): a Claude 2.1.316 bug in the first-turn path unrelated to editor mode that
   vim-mode sessions simply hit more.

## 5. Recommended change to how gx types and submits the prompt

The first two are gx-side and testable without Claude.

1. **Keep `editorMode: normal` a hard launch invariant and detect when it is not applied.** After
   `Ready`, the first screen must show no `-- INSERT --` or `-- NORMAL --` footer. If it does,
   log an `editor-mode-override-ignored` event with the screen and relaunch once before typing
   anything. This turns today's silent dependence on `harness.MergeClaudeSettings`
   (`harness/claudesettings.go:46`) into a checked one, in `ptyLauncher.launch` and the herdr
   equivalent. Today only the vim *probe* looks at the marker, and it treats the marker as good news.
2. **Land the shipped hung-pane recovery on this branch.** `agent-launch-and-recovery-impl` tickets
   03 to 05 (`2c57791fd`, `8257f323a`, `e8a500f50`, `HungPaneError` in
   `agentrunner/claudeui/nudge.go`) are in branch `agent-launch-and-recovery-impl`, not in HEAD
   (`git merge-base --is-ancestor 2c57791fd HEAD` is false). With them, a pane that hangs for any
   reason is stopped and relaunched once, which is the right backstop.
3. **Type slash launch prompts as a bracketed paste (optional, unproven).** Claude enables
   `\x1b[?2004h` (seen in the raw.log), and `HolderClient` already has `OpPaste`. A paste reaches
   Claude as one event and skips the per-key vim state machine. `claudeHarness.PastesText` would
   return true for a text starting with `/`. Do this only if the repro ever shows a vim-mode hang;
   a paste also changes how Claude treats the text (it may insert as pasted text and Enter must
   follow after `pasteSettle`, 300 ms), and the slash highlight and command parse on paste are
   unchecked.
4. **No timing change is justified by this evidence.** A longer pre-type delay or a launch
   throttle would not touch the data above (serial launches, healthy neighbours).

## 6. What a test would check

- Unit (`harness`): `MergeClaudeSettings` with a user config that has `editorMode: vim` yields
  exactly one `--settings` whose `editorMode` is `normal` (partly covered today; assert the final
  argv for the holder launch too).
- `nativerunner` launch test with a new `testutil/agentfake` knob "ignore the editor mode override"
  that draws `-- INSERT --` after ready: the launch relaunches once, writes
  `editor-mode-override-ignored` with the screen, and never types the prompt into the first pane.
- A fake-Claude knob "vim wedge": in vim mode, a single write that starts with `/` and is longer
  than 1 byte makes the fake stop reading and drawing. Assert (a) in normal mode the same launch
  delivers and the run log has no recovery events; (b) in forced vim mode the launch ends in the
  hung-pane relaunch (`HungPaneError`, one stop, one relaunch, exit 143 on the first pane) and the
  second pane gets the prompt. The existing `TUIClearIgnoredEnv` knob (`testutil/agentfake/tui_options.go`)
  already models "Ctrl+U ignored"; extend it, do not add a second mechanism.
- If recommendation 3 is taken: with a fake that wedges on key bursts but accepts a bracketed
  paste, the launch delivers the first time.

## 7. Repro recipe

```
# 3 concurrent panes at most; do not fork-storm. Trust the cwd first.
tmux -L gxr new-session -d -s a -x 200 -y 50 -c "$WORKTREE" \
  "claude --permission-mode auto --settings '{\"editorMode\":\"vim\"}'"
until tmux -L gxr capture-pane -p -t a | grep -q '❯'; do sleep 0.1; done
tmux -L gxr send-keys -t a -l '/gx-implement gx:epic/01'   # costs a model turn if it delivers
tmux -L gxr send-keys -t a Enter
sleep 4; tmux -L gxr capture-pane -p -t a                  # hung: input row still holds the text
tmux -L gxr send-keys -t a C-u; sleep 1; tmux -L gxr capture-pane -p -t a   # unchanged
```

Use Claude 2.1.316 if it is still obtainable, with gx's full launch (`gx agent-holder`, hooks,
status line) and 6 to 12 other agents running to recreate the load. It did not trigger on 2.1.319
with a bare launch. To find more cases, rescan the raw.logs: two `\x1b[?1049h` with the second
before byte 12,000 marks a hung first run.

## Not verified

- The mechanism inside Claude. No Claude source or release note on vim-mode input was read.
- Whether Claude 2.1.316 shows a menu while the command name is being typed.
- Whether the editor-mode override fully removes the hang or the user's load simply fell.
- Timing between the Ready signal and gx's first key (not logged; raw.log has no timestamps).
- The hung-pane code was read with `git show` on branch `agent-launch-and-recovery-impl`, not run here.
- I did not read the official Claude Code docs on vim mode or slash-command input.
