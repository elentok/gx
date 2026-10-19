# PTY prompt turn detection (gx:agent-runner-hardening/02)

How the PTY runner decides a prompt was delivered, why it misses fast or non-busy turns, and what a
fix inside the ticket's scope can look like. All sources are local files at `main` (82306f5f),
plus the earlier measurement on branch `research/claude-status-signals`
(`docs/research/claude-status-signals.md`, Claude Code 2.1.300). Ticket line numbers drifted:
the turn logic now lives in `harness/claudepid.go`, and `nativerunner/pidstatus.go` is only an alias
file.

## 1. How Prompt delivers text today (PTY)

Flow of `Pty.Prompt` → `PromptNudged` (`nativerunner/pty.go:573-600`):

1. `p.live(s)`: unknown session → `ErrNotFound`; a session marked died → its error
   (`pty.go:409-420`).
2. One `holder.Status()` call reads the claude status and the holder's `Turn` (`pty.go:582`).
3. Guard: only `waiting` is refused, as `ErrNotReady` (`pty.go:588-590`). **`busy`, `shell`, `idle`
   and "no pid file yet" all pass.**
4. `ptyPrompter{turn: <Turn read in step 2>}` is the baseline (`pty.go:591`).
5. `claudeui.PromptWithNudge` with caps `Grace = PromptGrace` (default 45 s), `Nudges: 2`,
   `Retypes: 1` (`pty.go:594-595`, `agentrunner/claudeui/nudge.go:22`):
   - Screen snapshot, then `Submit` = keys `text` then `\r`, as two separate `keys` ops
     (`pty.go:610-620`).
   - `WaitStart(grace)` polls `holder.Status()` every `PollInterval` (100 ms) until
     `Turn > baseline` (`pty.go:622-641`). **That is the only "delivered" test.**
   - If it times out: screen changed since the snapshot → bare Enter nudge; screen unchanged →
     Ctrl+U and retype the full text (`nudge.go:97-128`).
   - Out of nudges → `ErrStartTimeout`; unchanged screen with retypes used up →
     `ErrStuckSubmission` (`nudge.go:106-110,132-135`).
6. Both are wrapped as `ErrNotDelivered` (`pty.go:596-598`).

Where `Turn` comes from: the holder's `status` op returns `h.pid.PidState()`
(`nativerunner/holdersock.go:164-178`), which is `PidWatcher.live.turn`. `PidWatcher.poll` reads
`~/.claude/sessions/<pid>.json` every 100 ms (`harness/claudepid.go:28-29,125-136`) and does
`turn++` only when `status` differs from the last **polled** value (`claudepid.go:158-163`).

Consequences:

- **Fast turn.** If `idle → busy → idle` happens between two 100 ms reads, the poll sees `idle`
  both times: no `turn++`. `WaitStart` waits the full 45 s grace. The screen changed (the reply was
  drawn), so the first nudge is a bare Enter on an empty input. On the next timeout the screen is
  unchanged, so the text is **retyped and sent a second time** (`nudge.go:102-120`). If that turn is
  fast too, the loop ends with `ErrNotDelivered` after ~135 s. `startAndPrompt` then stops the
  session and starts a fresh one with the same prompt (`ralphloop/launch.go:337-348`): a third copy.
- **Never-busy command** (`/clear`): status stays `idle`, so the same path runs.
- **Busy claude.** The baseline is read while `busy`. The *current* turn's `busy → idle` edge moves
  `Turn`, so `WaitStart` returns true while the typed text only sits in claude's input queue.
  Prompt reports success for a turn that has not started. If the queued text later starts no turn
  (or is lost), nothing notices.

## 2. The status file and other submission signals

- The pid file `~/.claude/sessions/<pid>.json` is written **by claude itself**, not by a hook and
  not by the holder. gx only reads it (`harness/claudepid.go:49-60,140-153`). Fields gx reads:
  `status`, `waitingFor`, `sessionId`, `startedAt` (`claudepid.go:145-149`).
- States: `""` (starting, no file), `busy`, `shell` (idle with a background task), `idle`,
  `waiting` + `waitingFor` (`claudepid.go:14-22`). Claude does not tell "turn finished" from "Esc
  cancel": both are `idle` (claude-status-signals.md §2).
- Claude Code hooks (e.g. `UserPromptSubmit`) are **not allowed**: "Claude Code hooks are not used
  (blocked in some orgs)" (`docs/specs/agents-tab.md:502`).

Signals that do not depend on a polled busy edge:

| Signal | Where gx sees it | Fast turn | `/clear` | Notes |
| :- | :- | :- | :- | :- |
| OSC 7501 `state=working` | Holder parses it from the pty output stream in-band (`nativerunner/holder.go:177,271-289`) and hands it to `PidWatcher.Program` (`claudepid.go:297-311`) | Yes: pushed per change, no poll | Not measured | claude ≥ 2.1.295; off with `CLAUDE_CODE_DISABLE_TERMINAL_TITLE`; probe answered by every holder (`agents-tab.md:459,488`). Today `Program` stores the report but **does not bump `turn`**. |
| Pid file `sessionId` change | `PidWatcher.poll` already tracks it as `conversationID` (`claudepid.go:155,164`) | n/a | Yes: claude starts a new conversation id on `/clear` (`agents-tab.md:109,458`; the fake models it, `testutil/agentfake/program.go:38`) | Persists, so a 100 ms poll can't miss it. Today it wakes watchers but does not bump `turn`. |
| Transcript growth | `pollTranscript` stats the JSONL size each poll (`claudepid.go:189-212`) | Yes (a user entry is appended) | No: `/clear` moves to a new file | Trails by one write; undocumented format. |
| Screen / ptmx output | Holder sees every byte (`holder.go:177`) | Yes | Yes (redraw) | Says "something changed", not "prompt accepted". Already used by the nudge heuristic. |

## 3. What the holder can observe

- **Output**: every ptmx byte goes through one `io.MultiWriter` to `raw.log`, the screen, the frame
  hub, the display-name reader, the OSC 7501 parser and the `changes` hub (`holder.go:177`).
- **Input**: every key a client sends goes through `opKeys` → `ptmx.Write`
  (`holdersock.go:181-185`), plus attach input. So the holder can count Enter presses, but that is
  a submit *attempt*, not acceptance.
- **Claude state**: the `PidWatcher` lives inside the holder (`holder.go:139-146`). `turn` is
  already a holder-side monotonic counter that survives a server restart (`pty.go:216-218`).
- **Counter feasibility**: yes. `PidWatcher` holds `live.turn` under `w.mu`; `Program()` and `poll()`
  already take that lock, so bumping it on an OSC `working` report or a `sessionId` change is a
  few lines. `deny-reason` already waits on `turn > typedAt` inside the holder
  (`holdersock.go:210-225`), so it would benefit too.
- **How the client reads it**: the frozen `status` op already returns `Turn`
  (`holdersock.go:69-82,381-387`). Core ops are frozen; new behavior is advertised as an optional
  op in `ping` and checked with `Supports` (`holdersock.go:27-48,374-377`). A changed meaning of
  `Turn` should be announced the same way (e.g. a marker in `holderOptionalOps`), so the client
  knows an old holder's `Turn` is still edge-only.
- `agentrunner.Status.Turn` already allows extra bumps: "It may advance on other changes too: only
  equality means anything" (`agentrunner/agentrunner.go:57-62`). Ralph's stalled-since-launch check
  compares by equality (`ralphloop/launch.go:455-469`), so extra bumps are safe.
- The watch stream is not a substitute: its signal hub collapses pending wakes into one
  (`holdersock.go:274-307`), so a quick `working → done` may be seen only as `done`. A counter does
  not lose events.

## 4. Commands that never go busy

- Local slash commands run inside claude with no model call. gx's own spec says "local slash
  commands like `/clear` never show Working" (`agents-tab.md:520-523`). Other local commands (e.g.
  `/model`) open a dialog, which is `waiting` and does move `Turn` (the doctor relies on this:
  `cmd/claude_doctor.go:96-101`).
- `/compact` makes a model call. Ralph sends it through `Runner.Prompt` (`ralphloop/waitforfinish.go:336`)
  and expects Prompt to return once compaction *started* (`agentrunner.go:141-142`). On herdr a
  "premature idle" during compaction is special-cased (`herdrrunner/herdrrunner.go:116-125`). The
  PTY runner has no special case; whether claude shows `busy` for `/compact` was not measured here.
- `/clear` through gx: gx-design calls `gx agent actionable "..." --send "/clear" --send "/gx-design <epic>"`
  (`skills/gx-design/SKILL.md:125`). That Action runs in the agents-tab path, **not** through
  `Pty.Prompt`: `sendAction` types with `typeSessionPrompt` (raw keys + `\r`,
  `server/sessions.go:709-718`) and treats a `/`-prefixed step still Idle after the start timeout as
  finished (`server/sessionaction.go:96-99,120-139`). In the loop, `/clear` reaches `Pty.Prompt` only
  if a person sends it via `gx agent prompt` or a ticket nudge (`server/agentverbs.go:32-44`,
  `server/nudge.go:82`).

## 5. herdr parity

`herdrrunner.Runner.PromptNudged` (`agentrunner/herdrrunner/herdrrunner.go:108-144`):

- Guard: only `StateBlocked` → `ErrNotReady` (`:113-115`). Busy is accepted, like PTY.
- Submit + wait is one herdr `AgentPrompt` call waiting `until: working` (or `blocked/idle/done` too
  for `/compact`), inside the same `PromptWithNudge`, with `DefaultNudgeCaps` (45 s, 3 nudges,
  2 retypes) (`herdrrunner/nudge.go:22-63,97-113`). herdr's own stall error
  (`agent_prompt_stalled`) counts as a nudgeable timeout (`nudge.go:143-151`).
- Delivered = herdr's `StateChangeSeq` moved past the pre-prompt `Turn` (`herdrrunner.go:137-139`),
  plus for `/compact` a pane check that the input line no longer reads `/compact`
  (`:153-180`). herdr detects state from the screen, in herdr; gx has no edge-loss window of its
  own, but busy acceptance and never-busy commands have the same gaps.
- Headless refuses `Blocked` (`ErrNotReady`) and `Done` (`ErrNotDelivered`), and accepts busy
  (`nativerunner/headless.go:372-377`).

## 6. What callers expect

- Contract: "Prompt returns once a new turn started, or ErrNotDelivered" (`agentrunner.go:141-143`).
  `ErrNotDelivered` means "start a fresh session rather than retrying" (`agentrunner.go:101-103`);
  `ErrNotReady` means "can't accept input" (`:104-105`).
- `startAndPrompt`: `ErrNotDelivered` → stop and relaunch once, then park `agent_prompt_stalled`;
  any other error (incl. `ErrNotReady`) returns with the session live for the blocked-pane park
  (`ralphloop/launch.go:302-349`, `ralphloop/iteration.go:98-105`). The session is fresh and idle
  after `Start`, so a busy guard does not affect it.
- Smart-zone recovery: `Interrupt` (Esc, returns at once, `pty.go:650-656`) then immediately
  `Prompt("/compact")` (`ralphloop/smartzonerecovery.go:89-98`, `waitforfinish.go:336`). Claude may
  still be `busy` for a moment after Esc. **Today this is the busy-acceptance bug in action**: the
  Esc's `busy → idle` edge can count as `/compact` delivered. **With a plain busy guard it would get
  `ErrNotReady`** and recovery fails as "best effort" (`waitforfinish.go:341-347`). A busy guard needs
  a short wait for idle, or `Pty.Interrupt` must wait for idle (as `escThenPrompt` does,
  `pty.go:733-751`).
- Rate-limit "continue" (`waitforfinish.go:736`) and the corrective retry (`iteration.go:497-503`)
  prompt an idle/finished session; the adopted-launch prompt runs only when finished
  (`launch.go:424-431`).
- `gx agent prompt` maps `ErrNotReady` to a `ReasonAgentNotReady` refusal; other errors pass through
  (`server/agentverbs.go:37-43`). Ticket nudge passes every error through as a failure
  (`server/nudge.go:82-84`), so a busy guard would turn "nudge a working pane" into an error there.
- `escThenPrompt` (deny-reason on an old holder) prompts only after it saw `idle`
  (`pty.go:743-744`).

## 7. Candidate designs

All stay inside scope: no new status protocol; a holder-side counter is allowed.

**A. Busy guard only (with a short idle wait).** In `PromptNudged`, if status is `busy`/`shell`,
wait up to a few seconds for `idle`, else `ErrNotReady`. Mirror it in herdr (refuse
`working`).
- + Fixes false "delivered" on a busy claude and the Interrupt→`/compact` race. Tiny change.
- − Does nothing for fast turns or `/clear`. Changes ticket-nudge behavior on a working pane
  (refusal or error instead of queueing).

**B. Holder turn counter fed by push signals (recommended core).** In `PidWatcher`, also `turn++`
on an OSC 7501 session-wide `working` report and on a pid-file `sessionId` change. Advertise it with
an optional-op marker; the client keeps today's logic for an old holder.
- + Catches every turn claude announces, however short, with no poll window; `/clear` moves `Turn`
  through the new conversation id. Uses the existing `status` op and `Turn` field; ralph's equality
  checks are unaffected; `deny-reason` gets the same fix for free.
- − OSC 7501 needs claude ≥ 2.1.295 and is lost under `CLAUDE_CODE_DISABLE_TERMINAL_TITLE`; then it
  falls back to today's poll. Whether claude sends any OSC report for `/clear` or other local
  commands is unmeasured (the `sessionId` bump covers `/clear` either way). A fast-turn race also
  remains on old holders until claude restarts.

**C. Faster or edge-latched polling in the holder.** Keep the pid file as the only source but poll
faster, or use a file watcher (kqueue/fsnotify) on `<pid>.json` and bump on every write.
- + No dependence on OSC 7501.
- − A file event per write still can be coalesced; faster polling costs idle CPU (see the
  idle-cost rules); claude may rewrite in place without a status change. Narrows the window, does
  not close it.

**D. Documented slash-command handling in Prompt.** For text starting with `/` (except
`/compact`), treat "status still `idle` and the input line cleared" after a short wait as delivered,
never nudge/retype, like `sendAction` does (`server/sessionaction.go:120-139`).
- + Stops double delivery of `/clear`-like commands even on old holders and old claude.
- − Heuristic: reads the screen; a slash command that does start a turn is still judged by `Turn`.

**Recommendation: B + A, with D as the fallback for old holders/claude.**
1. Holder: bump `turn` on OSC `working` and on `sessionId` change; advertise via an optional-op
   marker.
2. `Pty.PromptNudged`: refuse `busy`/`shell` with `ErrNotReady` after a short wait for idle
   (covers Interrupt→`/compact`); do the same in herdr (`working` → `ErrNotReady`) for parity.
3. For `/`-prefixed text other than `/compact`, do not retype: treat an unchanged `Turn` with a
   cleared input line as delivered (D). Document that local commands never go busy.
4. Decide whether ticket nudge should refuse a working pane (likely yes: map `ErrNotReady` to a
   refusal in `server/nudge.go`).

## Open questions

- Does claude emit OSC 7501 `working`/`done` for `/compact`, and anything for `/clear` or other
  local commands? Needs a short live measurement like `research/claude-status-signals`.
- Does the pid file show `busy` during `/compact` on the PTY runner?
- How long after Esc does claude's pid file read `idle`? That sets the idle-wait bound in A.
