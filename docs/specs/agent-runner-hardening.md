# Agent runner hardening: prompts, answers and deny reasons reach the agent once

Source: wayfinder map `gx:agent-runner-hardening`. Tags like `[H-05]` point to the map ticket that
settled the point. Scope is tickets 01, 02 and 04, plus the headless parity from ticket 12.
Tickets 05-14 hold the design and the measurements behind it. Builds on `docs/specs/agent-runner.md`
(stories there are cited as "agent-runner story N") and ADR 0031 (two native agent runners).

Order of work: tickets 02 and 04 (PTY) first, then 01 (herdr), with the headless and Codex parity
alongside. PTY may become the default runner, so its fixes matter most. Herdr stays supported, with
lighter effort. [map]

## Problem Statement

ralph-loop, the server's agent verbs, the ticket nudge and the budget stop send text and keys to a
live agent through an agent runner. Today that text can arrive twice, arrive mixed with other text,
or land in the wrong place:

- **Fast or busy turns (PTY and herdr).** `Prompt` refuses only a `blocked` agent. A prompt sent
  while claude is busy is only queued, yet claude's next busy→idle edge counts as "delivered". A
  turn that starts and ends between two 100 ms polls of the pid status file moves no `Turn`, so the
  nudge presses Enter and then retypes: the text is sent twice. After the nudge budget the prompt
  fails with `ErrNotDelivered`, and ralph-loop relaunches with the same prompt: a third copy.
  [H-02]
- **Busy turns (headless).** A busy claude queues the stream-json message, but `Prompt` waits only
  for the turn to start. A running turn longer than the prompt timeout (30 s) gives
  `ErrNotDelivered` while the queued prompt still runs later, so a retry duplicates it. [H-12]
- **Slash commands.** `/clear` changes only the session id; dialog commands (`/help`, `/cost`,
  `/model`, `/status`) only set `waiting`. Neither looks like a turn today, so they get nudged and
  retyped. [H-02, H-06]
- **Interrupt (PTY).** PTY `Interrupt` sends a lone Esc. Measured on claude 2.1.303, a lone Esc
  never interrupts a busy claude. Claude holds it and fires it on the next byte, which eats the
  start of the next typed text (smart-zone's `/compact` right after an interrupt is garbled). An
  interrupt that works also puts the cancelled prompt back in the input line, where the next prompt
  is appended to it. [H-06, H-07]
- **Interrupt (herdr).** Herdr `Interrupt` always sends Ctrl+C. One Ctrl+C on an idle Codex quits
  it, and Codex context-exhaustion recovery sends that Ctrl+C on a pane that is likely idle. Two
  Ctrl+C on idle claude exit it. [H-13, H-14]
- **Keys during a deny-reason (PTY).** While the holder runs its deny-reason step (Esc, wait for
  idle, type the reason, Enter), a `keys` op or a person typing on an attach can write to the pty
  between those bytes. The reason then reaches claude mixed with other input. [H-04]
- **Deny with a reason (herdr).** The herdr runner types the reason into the open dialog instead of
  pressing Esc and then sending the reason as a prompt. [H-01]
- **Budget stop.** Once `Interrupt` can wait, the budget stop's one-by-one Interrupts would delay
  the kill by up to N per run. [H-11]

## Solution

Every prompt, answer and deny reason reaches the agent exactly once, unmixed, on the PTY runner
(claude), the herdr runner (claude and Codex) and the headless runner (claude). The runners act the
same for the same verb. Where one runner cannot match, this spec names the gap.

- **The PTY holder sees more turns.** Besides any status change in the pid file, the holder's
  `Turn` also goes up on claude's session-wide OSC 7501 `working` report and on a new `sessionId`
  in the pid file. A fast turn and `/clear` now move `Turn`. [H-05]
- **Prompt waits out a busy agent (all three runners).** If the agent is busy, `Prompt` waits up to
  N (default 5 s) for it to go idle, then sends. Still busy at N → `ErrNotReady`, and nothing is
  sent. Background tasks alone do not count as busy. [H-05, H-12]
- **Slash commands are never retyped.** A `/` command other than `/compact` is sent once. It counts
  as delivered when `Turn` moves, or when the agent is still idle after N. [H-07, H-12, H-14]
- **Interrupt picks its key by state (PTY and herdr).** Busy → Ctrl+C. Blocked → Esc. Any other
  state → nothing. After a key it waits up to N for idle and presses Ctrl+U to clear the restored
  prompt. Not idle in time → `ErrNotReady`. Headless keeps its control-protocol interrupt.
  [H-07, H-09, H-14]
- **The PTY holder locks the pty during a deny-reason.** A `keys` op is refused at once, which the
  PTY runner reports as `ErrNotReady`. Attach input is held and sent in order once the step ends.
  Terminal replies and the program-status probe still go out. [H-04]
- **Deny with a reason on herdr matches PTY.** Esc, wait up to N for idle, then the reason as a
  normal prompt. While this runs, the herdr runner reports the session as `blocked` to every caller
  in the server process and refuses Prompt, Interrupt and Answer. If the reason is not delivered,
  `Answer` returns `ErrNotDelivered` and the session stays alive. [H-01, H-08]
- **Callers handle `ErrNotReady`.** ralph-loop's rate-limit "continue", corrective prompt and
  adopted-session prompt re-read `Status`: busy → skip the prompt and keep waiting; blocked → the
  existing `blocked-pane` park. The ticket nudge and `gx server agents interrupt` give the
  `agent-not-ready` refusal. Smart-zone and Codex context-exhaustion recovery retry on the next
  poll. The budget stop logs it and goes on. [H-05, H-08, H-14]
- **The budget stop runs Interrupts in parallel inside its grace,** so the stop takes about
  max(grace, N), 15 s by default. [H-11]

## User Stories

### Prompt delivery: busy guard (all runners)

1. As ralph-loop, I want `Prompt` to send my text exactly once, so that the agent never works on a
   doubled prompt. [H-02, H-05]
2. As ralph-loop, I want `Prompt` on a busy agent to wait up to N for it to go idle before it sends,
   so that my text is never only queued behind a running turn. [H-05]
3. As ralph-loop, I want `Prompt` on an agent still busy after N to return `ErrNotReady` without
   sending anything, so that I never queue text behind a running turn. Each `Prompt` caller then
   acts as set out under "ralph-loop `Prompt` callers" below: the rate-limit "continue", the
   corrective prompt and the adopted-session prompt go back to waiting on a busy agent; the initial
   launch and smart-zone keep today's handling. [H-05, review]
4. As ralph-loop, I want a background task to count as idle for this guard on every runner (PTY pid
   `shell`, herdr's background-task probe, headless `bgTasks`), so that a long background task does
   not block every prompt. Claude accepts input while one runs. [H-05, H-12, map parity]
5. As a maintainer, I want the reported `Status` to stay as it is today (a background task still
   reads `working` where it does now), so that waiters such as finish-up keep waiting for background
   tasks. Only the guard looks past them. [H-12]
6. As ralph-loop, I want `Prompt` on a `blocked` agent to keep returning `ErrNotReady` at once, so
   that Enter never picks a dialog option nobody chose. [agent-runner, H-05]
7. As ralph-loop on headless, I want a running turn or a queued gx command to count as busy, so that
   a prompt is never queued behind them and later reported as not delivered. [H-12]
8. As ralph-loop on headless, I want the busy guard to end the duplicate where a queued prompt timed
   out with `ErrNotDelivered` and then ran anyway after a retry. [H-12]
9. As a maintainer, I want N to be one runner config knob, default 5 s, used by every runner and
   every caller, so that it can be tuned from the measured Esc→idle time (under 200 ms). [H-05,
   H-06, H-07]
10. As a maintainer, I want no special `/compact` wait, so that the rule stays small. A prompt sent
    during a long compaction gets `ErrNotReady`; nothing in gx prompts during its own compaction.
    [H-07]

### Turn detection (PTY holder)

11. As ralph-loop on PTY, I want `Turn` to go up when claude reports OSC 7501 `working` for the
    whole session, so that a turn that starts and ends between two polls of the pid file still
    counts. [H-05, H-06]
12. As ralph-loop on PTY, I want `Turn` to go up when the pid file shows a new `sessionId`, so that
    `/clear` counts as delivered. [H-05, H-06]
13. As ralph-loop on PTY, I want `Turn` to keep going up on every pid-file `status` change, `waiting`
    included, so that a dialog command counts as delivered. [H-07]
13a. As ralph-loop on PTY, I want the holder's `status` op to also report claude's session-wide
    OSC 7501 state, so that the busy guard and `Interrupt` see a busy claude before the next pid-file
    poll does. Without it, `Prompt` can type into a busy claude and `Interrupt` can send no key in
    that ~100 ms gap. [review]
14. As a maintainer, I want per-task OSC 7501 reports to leave `Turn` alone, as they leave status
    alone today, so that a sub-task does not look like a new turn. [H-05]
15. As a maintainer, I want the holder to advertise the new turn rule with a marker in `ping`'s
    optional-op list, so that the client knows which `Turn` it reads. [H-05]
16. As a maintainer, I want the PTY runner to keep today's polled-edge logic for a holder without the
    marker, so that a gx upgrade never breaks a live run on an older holder (agent-runner "frozen
    core"). [H-05]
17. As ralph-loop, I want the new bumps to live in the holder's restart-safe counter, so that `Turn`
    still survives a server restart (agent-runner story 104a). [H-02, H-05]

### Slash commands (all runners)

18. As ralph-loop, I want a `/` command other than `/compact` never nudged or retyped, so that a
    command that shows no turn is never run twice. [H-05, H-07]
19. As ralph-loop, I want a `/` command to count as delivered when `Turn` moves (PTY: OSC `working`,
    new `sessionId` or any status change; herdr: `StateChangeSeq`; headless: a turn move or a
    lifecycle event for the command's uuid), so that `/clear` and dialog commands return at once.
    [H-07, H-12]
20. As ralph-loop, I want a `/` command to count as delivered when the agent is still idle after N,
    so that a command with no visible signal does not fail. There is no input-line check. [H-07,
    H-12, H-14]
21. As ralph-loop, I want `/compact` to keep the normal prompt path (busy guard, nudge, retype, and
    herdr's Codex compact confirmation), so that a lost `/compact` is still retried. [H-05, H-06,
    H-10]
22. As a user of claude, I want a dialog that a dialog command opened to stay open and the session
    to report `blocked`, so that I see it and close it with `answer deny` (Esc). gx never closes it,
    and `Prompt` refuses until it is closed. [H-07]
23. As ralph-loop on herdr Codex, I want `/compact` and `/new` delivered on the `StateChangeSeq` move,
    and `/status` and `/model` delivered through idle-after-N, so that each is sent once. Those two
    take up to N to return, and the `/model` picker does not read `blocked`. [H-13, H-14]

### Interrupt (PTY and herdr)

24. As ralph-loop, I want `Interrupt` on a busy agent to send Ctrl+C, so that the turn really ends.
    A lone Esc does not interrupt claude and eats the next input. [H-06, H-07]
25. As ralph-loop, I want `Interrupt` on a blocked agent to send a lone Esc, so that the dialog or
    approval closes (on Codex this denies the approval). [H-07, H-13]
26. As ralph-loop, I want `Interrupt` on an agent in any other state (idle, PTY `shell`, herdr `done`
    or `unknown`) to send nothing and return at once with no Ctrl+U, so that a Ctrl+C never reaches
    an idle agent: one quits idle Codex, two exit idle claude. [H-07, H-09, H-14]
27. As ralph-loop on herdr, I want `Interrupt` to read the state again right before it sends the key,
    so that the key matches the state the pane is in now. [H-14]
28. As ralph-loop, I want `Interrupt` to wait up to N for idle after the key and then press Ctrl+U,
    so that the cancelled prompt claude puts back in the input line never prefixes my next prompt.
    On herdr the wait accepts `done` as idle. [H-07, H-09, H-14]
29. As ralph-loop, I want `Interrupt` to return `ErrNotReady` when the agent is not idle within N, so
    that I know the interrupt did not land. [H-07, H-09]
30. As ralph-loop on herdr, I want `agent_not_found` after the key to be a failure, so that a Codex
    that quit is never read as interrupted. [H-14]
31. As a maintainer, I accept that Ctrl+U clears only the current line on Codex, so text on other
    composer lines may survive an interrupt. Codex never restored a cancelled prompt in the
    measurements. [H-13, H-14]
32. As a maintainer, I accept the residual race where the turn ends between the state read and the
    key, so one Ctrl+C reaches an idle agent. On claude it only shows a hint; no extra guard. [H-07]
33. As ralph-loop's smart-zone recovery, I want `Interrupt` then `Prompt("/compact")` to work with no
    extra wait of my own, so that `/compact` is neither garbled by a held Esc nor refused as busy.
    [H-02, H-07]
34. As ralph-loop on headless, I want `Interrupt` to keep its control-protocol `interrupt` with no key
    rule and no Ctrl+U, so that it stays exact. The busy guard on the next `Prompt` covers the time
    until claude is idle. This is a documented runner difference. [H-12]

### Interrupt callers

35. As a caller of `gx server agents interrupt`, I want `ErrNotReady` to give the `agent-not-ready`
    refusal, as `gx server agents prompt` does, so that I get a clear answer, not a server error.
    [H-08]
36. As ralph-loop's smart-zone recovery, I want an `ErrNotReady` from `Interrupt` to mean "not this
    tick", so that I retry on the next poll instead of failing the run. Any other error still fails
    the run. [H-08]
37. As ralph-loop's Codex context-exhaustion recovery, I want the state-aware `Interrupt` instead of a
    raw Ctrl+C (idle/done → no key, then `/compact`; blocked → Esc; working → Ctrl+C), so that
    recovery never quits an idle Codex. [H-14]
38. As ralph-loop's Codex context-exhaustion recovery, I want the banner-text detection kept on all
    three paths (blocked, idle/done, timeout while working), and an `ErrNotReady` to retry on the
    next poll, so that recovery does not depend on the banner's herdr state. [H-08, H-14]

### Budget stop

39. As the budget stop, I want the grace period to start at the stop decision, with the Interrupt
    wait counted inside it, so that the stop time does not grow with N. [H-11]
40. As the budget stop, I want one Interrupt per run, all in parallel, so that a slow pane cannot
    delay the others. [H-11]
41. As the budget stop, I want to use the same full `Interrupt` verb and log and ignore its
    `ErrNotReady` and other errors, so that a stuck agent is still stopped. [H-08, H-11]
42. As the budget stop, I want to `Stop` each run at the later of the grace deadline and all
    Interrupts returned, so that `Stop` never races a running Interrupt. Stop time is max(grace, N),
    about 15 s by default. [H-11]

### Deny with a reason (PTY and herdr)

43. As a user, I want `answer <addr> deny <reason>` to press Esc on the dialog and then send the
    reason as a normal prompt, on PTY and on herdr (claude and Codex), so that the reason is never
    typed into the dialog. [H-01, H-10]
44. As ralph-loop, I want the session to keep reporting `blocked` from Esc until the agent has taken
    the reason, on both runners, so that `Wait` never sees the idle gap and ends the iteration early.
    [H-01, agent-runner story 109]
45. As a caller, I want a `Prompt`, nudge, `Interrupt` or `Answer` that arrives during a deny with a
    reason to get `ErrNotReady` and send nothing, so that my text never lands between Esc and the
    reason. [H-01, H-04, H-08]
46. As a user of headless, I want deny with a reason to stay as it is: the reason travels inside the
    `control_response`, which is already exact. [H-12]

### PTY holder lock during a deny-reason

47. As a maintainer, I want the holder to refuse a `keys` op at once while its deny-reason step runs,
    with a "deny-reason in progress" error and no write, so that no key lands inside the step.
    [H-04]
48. As a caller of the PTY runner, I want that refusal reported as `agentrunner.ErrNotReady`, not as
    a dead holder, so that I retry instead of parking `needs-repair`. [H-04]
49. As a maintainer, I want the refusal to be immediate, not blocking, because the step can run
    2 × `holderDenyTimeout` (6 s), longer than the client's 5 s `holderIOTimeout`; a blocked write
    would land after the caller gave up and would read as a dead holder. [H-04]
50. As a person attached to the session, I want keys and pastes I type during the step held and then
    sent to claude in order, so that my input is not lost and does not mix with the reason. Attach
    streams have no per-request timeout, so a pause of up to about 6 s is safe. [H-04]
51. As a maintainer, I want terminal replies and the program-status probe to stay outside the lock,
    so that claude is never stalled while the step waits for it to go idle. [H-04]
52. As a maintainer, I want the bytes the holder writes to the pty during a deny-reason to be exactly
    Esc, the reason, then `\r`, followed by any held attach input, so that the order is provable.
    [H-04]

### Herdr deny with a reason

53. As a user on herdr, I want deny with a reason to send Esc, wait up to N for `idle`/`done`, then
    send the reason through the normal `Prompt` path, nudge included. [H-01]
54. As ralph-loop on herdr, I want the herdr runner to report `blocked` for the session while
    `Answer` runs, so that every waiter in the server sees no idle gap. The server's waiters and
    `Answer` share one runner value, so an in-process hold is enough. [H-01]
55. As a maintainer, I want the herdr hold to end when `StateChangeSeq` moves past the value read
    after Esc, or on any error, and never to outlast the `Answer` call, so that a failed deny never
    leaves the session stuck as `blocked`. [H-01]
56. As a user on herdr, I want a deny whose reason did not arrive (no idle within N, or the reason
    not delivered) to return `ErrNotDelivered` ("denied, but the reason was not delivered") and keep
    the session alive, so that I can resend the reason with `gx server agents prompt`. Unlike PTY,
    the session is not marked died. [H-01]
57. As a maintainer, I want herdr `Interrupt` to check the hold itself, not through status, and
    return `ErrNotReady` with no key while it is active, so that the window matches Prompt's
    refusal and PTY's locked `keys` op. [H-08]

### Nudge verb

58. As a caller of `gx server tickets nudge`, I want an agent that is not ready (busy past N,
    blocked, or in a deny with a reason) to give the `agent-not-ready` refusal, so that I get the
    same answer as from `gx server agents prompt`. [H-05]
58a. As a maintainer, I accept that the nudge can no longer steer an agent in the middle of a turn
    (today it queues the text). One rule for every caller keeps the duplicate bug closed. A person
    can still steer mid-turn by typing in the Agents tab attach (PTY) or in the pane (herdr);
    headless has no other way. [review]

### Parity and documented gaps

59. As a maintainer, I want the busy guard, the `/` rule, the Interrupt rule and deny with a reason
    to act the same on every runner that has them, checked by shared conformance cases. [map, H-09,
    H-12]
60. As a maintainer, I want herdr to get no new turn counter, since herdr has no holder and
    `StateChangeSeq` is its turn signal. A turn herdr itself does not see still moves no `Turn`.
    [H-05]
61. As a maintainer, I want the holder lock to be PTY-only: herdr has no holder and no deny-reason
    step, and the herdr hold covers herdr's reason delivery. [H-04]
62. As a maintainer, I want the herdr deny gap documented: readers outside the server process (the
    TUI reading herdr directly) and a gx restart in the middle of a deny can still see `idle`
    between Esc and the reason. [H-01]
63. As a maintainer, I want the Codex gaps documented: the context-exhaustion banner's herdr state
    is unmeasured (likely not `blocked`), and the `/model` picker and the backtrack view read
    `idle`. gx never sends `/model` to Codex and never sends Esc to an idle pane, so only a person
    can open those. [H-13, H-14]

## Implementation Decisions

**The N knob** [H-05, H-07]

- One runner config knob, default 5 s. It bounds: the busy guard's wait in `Prompt`, the `/` rule's
  idle-after-N, `Interrupt`'s wait for idle, and herdr's wait for idle in deny with a reason.
- The config key, where it is read and how it reaches each runner are the implementer's choice.
- Other timeouts stay as they are: the nudge grace (`PromptGrace` and herdr's caps), headless
  `PromptTimeout`, the holder's `holderDenyTimeout` (3 s) inside its deny-reason step, and the
  budget kill grace.

**Turn counter (claude pid watcher, PTY holder)** [H-05, H-06, H-07]

- The pid watcher that counts pid-file `status` changes as turns also counts a session-wide OSC
  7501 `working` report (per-task reports stay ignored) and a change of the pid file's `sessionId`.
  Status changes keep counting, `waiting` included.
- The holder adds a marker to its optional-op list in `ping`. The marker means "`Turn` includes the
  OSC `working` and `sessionId` bumps, and `status` carries the OSC state". Its name is the
  implementer's choice. No new op is added.
- `status` keeps its fields and gains one optional (`omitempty`) field: the last session-wide OSC
  7501 report the holder stored (cleared on OSC `clear`). Older clients ignore it, as they ignored
  `Exited` when it was added. The field's name and encoding are the implementer's choice. [review]
- The PTY runner reads the marker once per holder client. Without it, it keeps today's logic.
- The counter stays in the holder, so it survives a server restart.

**Busy guard in Prompt (all runners)** [H-05, H-12]

- `blocked` → `ErrNotReady` at once, as now. Busy → wait up to N for idle; still busy →
  `ErrNotReady`, nothing sent.
- Busy means: PTY pid `busy`; herdr's own `working` state; headless a running turn or a queued gx
  command. Background tasks are not busy for the guard: PTY `shell` is idle, the guard reads herdr's
  state before the background-task probe, and headless ignores `bgTasks`. `Status` itself is
  unchanged.
- Other states keep today's handling (for example headless `done` → `ErrNotDelivered`).
- On PTY the guard reads the OSC state from `status` first, when the holder has the marker: OSC
  `working` → busy, even if the pid file still says idle. Otherwise it reads the pid file. On a
  holder without the marker, the field is absent and the guard uses the pid file alone (today's
  ~100 ms gap remains there).
- The guard runs on every holder, old ones included; it needs only the core `status` op.

**Slash commands (all runners)** [H-05, H-07, H-12, H-14]

- `/compact` and plain text keep the existing delivery path (`claudeui.PromptWithNudge` on PTY and
  herdr, herdr's Codex compact confirmation, headless's stream-json message), with the new turn
  signals on PTY.
- Any other `/` command is sent once with Enter (headless: one message) and never nudged or retyped.
  Delivered when `Turn` moves (headless: also a lifecycle event for its uuid), or when the agent is
  still idle after N. Otherwise `ErrNotDelivered`, still with no retype. There is no input-line
  check.
- This applies on every holder, old ones included.

**Interrupt (PTY and herdr)** [H-07, H-08, H-09, H-14]

- Herdr only: while the deny hold is active → `ErrNotReady`, no key. On PTY the locked `keys` op
  gives the same result.
- Read the state: busy → Ctrl+C; `blocked` (permission or dialog) → lone Esc; any other state (idle,
  PTY `shell`, herdr `done` or `unknown`) → no key, no Ctrl+U, return nil at once. State here is the
  same raw state the busy guard reads (on PTY: the OSC state first, then the pid file), so a
  background task never makes an idle agent get Ctrl+C.
- Herdr reads the state again right before it sends the key.
- After a key: wait up to N for idle (herdr: `idle` or `done`), then Ctrl+U. Not idle within N →
  `ErrNotReady`. Herdr `agent_not_found` after the key → failure (not `ErrNotReady`).
- The result must be an empty input line on claude. Before relying on Ctrl+U alone, the implementer
  checks on claude that it clears a multi-line restored prompt; if it does not, the implementer
  picks the clearing keys that do. On herdr, the implementer also checks that the input line is
  empty after interrupting a busy claude, and adds a short settle delay before Ctrl+U only if that
  check fails. On Codex, Ctrl+U clears the current line only; that is accepted.
- These two checks are a deliberate exception to H-05's rule that unmeasured facts are settled in
  design. Either outcome keeps the same design: only the clearing keys or a short delay change, never
  the states, errors or callers. The conformance case "`Interrupt` leaves an empty input line"
  proves the result on both runners. [review]
- Where the key → wait → Ctrl+U sequence lives (a shared helper or per runner) is the implementer's
  choice; the shared conformance cases prove the runners act the same.
- Headless `Interrupt` is unchanged: control-protocol `interrupt`, returns at once.

**ralph-loop `Prompt` callers** [H-05, review]

Today every `ErrNotReady` from `Prompt` means "blocked on a dialog". With the busy guard it can also
mean "still busy after N". Each caller:

- Initial launch (`startAndPrompt`): unchanged. The 15 s blocked-pane dwell and re-check, then
  `blocked-pane` if still blocked, else a launch failure (`needs-repair`, `iteration-error`). An
  agent still busy N after launch is unexpected enough to need a person.
- Rate-limit "continue", the corrective prompt for unexecuted tool calls, and the prompt to a stalled
  adopted session: these are sent when the agent should be idle, so a busy agent has started a turn
  by itself. On `ErrNotReady` the caller re-reads `Status`: busy → skip the prompt and go back to
  waiting for the iteration to finish; blocked → the existing `blocked-pane` park (dwell, re-check,
  `needs-answer`); any other state → the error is returned as today (`iteration-error`).
- Smart-zone `/compact` and finish-up: unchanged. They log, give up for this tick, and the next poll
  tries again while usage is still over the zone.
- No new park kind; the park-kind mapping (agent-runner story 28) is unchanged.

**Interrupt callers** [H-08, H-11, H-14]

- `gx server agents interrupt` maps `ErrNotReady` to the `agent-not-ready` refusal. Other errors
  stay errors.
- Smart-zone recovery: `ErrNotReady` from `Interrupt` → skip, retry on the next poll. Other errors
  fail the run, as now. No extra wait before `Prompt("/compact")`.
- Codex context-exhaustion recovery: keep banner-text detection on all three paths; replace the raw
  Ctrl+C with `Interrupt`; `ErrNotReady` → retry on the next poll.
- Budget stop: deadline = stop decision + `BudgetKillGrace`. One goroutine per run calls
  `Interrupt`; errors are logged and ignored. `Stop` and park each run once the deadline has passed
  and every Interrupt has returned.

**Holder deny-reason lock (PTY only)** [H-04]

- While the deny-reason step runs, the holder answers `keys` with a "deny-reason in progress" error
  and writes nothing. The PTY runner maps that error to `agentrunner.ErrNotReady`, not to
  `ErrSessionDied`. How the error is told apart on the wire is the implementer's choice, within the
  `keys` op's existing response shape.
- Attach input (keys and pastes) that arrives during the step is queued and written to the pty in
  arrival order once the step ends, on success or failure.
- Terminal query replies and the OSC 7501 program-status probe are written without the lock.
- The deny-reason step itself is unchanged: Esc, wait for idle, the reason, then `\r` written apart,
  then wait for `Turn` to move, with status held at `blocked` throughout.
- Follow-on effects on PTY, with no extra code: `Interrupt` and `Answer` during the step read
  `blocked`, try to send a key, get the refusal, and return `ErrNotReady`. `Answer`'s `ErrNotReady`
  keeps its existing `not-blocked` refusal.
- An older holder has no lock. Per the agent-runner upgrade rule, a live run finishes on the holder
  that started it, so that run keeps today's behaviour. Its deny path also stays as it is (the
  holder op, or Esc then the reason as a prompt if the holder predates the op).

**Deny with a reason (herdr)** [H-01, H-08, H-10]

- `Answer` with `deny` and a reason, on claude and Codex: press Esc (never the reason as dialog
  keys), wait up to N for `idle`/`done`, then send the reason through the runner's normal `Prompt`
  path (busy guard and nudge included).
- In-process hold: set before Esc; while it is active the herdr runner's `Status` for that session
  reports `blocked`. It ends when `StateChangeSeq` moves past the value read after Esc, or on any
  error, and always before `Answer` returns.
- During the hold: `Prompt` (and so the nudge verb) sees `blocked` and returns `ErrNotReady`;
  `Interrupt` checks the hold itself and returns `ErrNotReady`; a second `Answer` returns
  `ErrNotReady` with no key, as on PTY. The deny's own reason prompt must pass its own hold; how is
  the implementer's choice.
- Failure after Esc → `ErrNotDelivered` with "denied, but the reason was not delivered". The session
  stays alive.
- `allow`, and `deny` with no reason, keep their current keys.

**Nudge verb (server)** [H-05]

- The ticket nudge maps a `Prompt` `ErrNotReady` to the `agent-not-ready` refusal, the same reason
  the `prompt` agent verb uses. Other errors stay errors.
- The nudge does not skip the busy guard. Losing mid-turn steering through the nudge is accepted
  (story 58a); the attach (PTY) and the pane (herdr) remain for a person. [review]

**Unchanged**

- The `Runner` interface, its typed errors and their park-kind mapping.
- `holder.sock`'s frozen core ops keep their request and response shapes.
- Headless deny and headless `Interrupt`.

## Testing Decisions

- **What makes a good test:** it checks what the agent receives and what the `Runner` returns
  (errors, `Status`, `Turn`), never internal fields, and no timings beyond the stated bounds.
- **Seams**, highest first. These are proposed from the map and the existing test layout:
  1. **The runner conformance suite (existing, highest seam).** New shared scenarios: busy prompt
     waits then sends; busy past N gives `ErrNotReady` with nothing sent; a background task alone
     does not trip the guard; a fast turn is delivered once (no retype); a `/` command is never
     retyped and counts as delivered when idle after N; a dialog command counts as delivered and
     leaves the session `blocked` (claude); `Interrupt` sends Ctrl+C when busy, Esc when blocked,
     nothing when idle, and leaves an empty input line; `Prompt`, `Interrupt` and `Answer` during a
     deny with a reason give `ErrNotReady`. They run on the PTY runner (TUI fake of claude) and on
     the herdr runner (`herdrfake`). The busy-guard and `/`-rule scenarios also run on headless; the
     Interrupt key scenarios skip on headless with the documented runner difference. `DenyWithReason`
     (incl. "never idle") now runs on herdr. The in-memory `runnerfake` passes the new scenarios or
     skips them with a reason.
  2. **New conformance harness hooks** (inside seam 1, not a new seam): a hook that returns the text
     and keys the fake agent received, in order (so "exactly once, unmixed" is checkable); hooks
     that make the fake's next turn fast (no polled status edge), keep it busy, or make the next
     prompt open a dialog. Herdr gets the existing `LastPrompt` hook. The PTY TUI fake gains OSC
     7501 `working` output and `sessionId` changes. Hook names and shapes are the implementer's
     choice.
  3. **Codex on herdr (inside seam 1).** The deny and Interrupt scenarios get a Codex-on-herdr case
     where `herdrfake` can act as Codex: idle/done/unknown → no key, `done` accepted after the key,
     `agent_not_found` after the key is a failure. Whether to extend the fake is the implementer's
     choice; any Codex case it cannot drive is listed as untested in the test file.
  4. **The holder over `holder.sock` (existing seam, ticket 04 proof).** One test starts a
     deny-reason, sends a `keys` op and attach input in the middle, and checks that `keys` is
     refused and the pty bytes are exactly Esc, reason, `\r`, then the held attach input. A second
     check: terminal replies still go out during the step.
  5. **The claude pid watcher (existing unit seam).** `Turn` goes up on a session-wide OSC `working`,
     on a `sessionId` change and on every status change; not on a per-task report. The `ping`
     marker is listed. `status` carries the stored session-wide OSC state, and the PTY runner reads
     a busy claude as busy while the pid file still says idle.
  6. **Herdr `Answer` and `Interrupt` unit tests (existing seam on `herdrfake`).** Deny with a reason
     sends Esc and never the reason as dialog keys; failure after Esc gives `ErrNotDelivered` and the
     session stays live; the hold ends on error; `Interrupt` during the hold gives `ErrNotReady` and
     sends no keys.
  7. **Server verb tests (existing seam on `runnerfake`).** The ticket nudge and `agents interrupt`
     turn `ErrNotReady` into `agent-not-ready`. Budget stop: several runs with slow fake Interrupts
     stop in about the grace, not the sum; a grace shorter than N waits for the Interrupts before
     `Stop`.
  8. **ralph-loop tests (existing, on the in-memory `Runner` fake).** The rate-limit "continue",
     corrective and adopted-session prompts skip and keep waiting when `Prompt` gives `ErrNotReady`
     and `Status` reads busy, and park `blocked-pane` when it reads blocked. Smart-zone and Codex
     context-exhaustion recovery retry on the next poll after `Interrupt` gives `ErrNotReady`; other
     errors still fail the run; context-exhaustion recovery no longer sends a raw Ctrl+C.
- **Prior art:** `agentrunner/runnertest` and its PTY, headless and herdr harnesses; the TUI fake in
  `testutil/agentfake`; `testutil/herdrfake`; the holder and holder-socket tests in the native
  runner package; the claude pid watcher tests in the harness package; the server nudge, agent-verb
  and budget tests on `testutil/runnerfake`; the ralph-loop Codex context recovery tests.
- **Real agents:** the measured facts behind these rules (tickets 06 and 13) are rechecked by hand;
  CI never needs claude or Codex.

## Out of Scope

- A blocking "deny + reason" op inside herdr (ticket 01 option C). It needs herdr changes. [map]
- A rewrite of runner status detection past what ticket 02 needs. The holder turn counter is in
  scope; a new status protocol is not. [map]
- Holder and session liveness and cleanup (old ticket 03, moved to `gx:server-recovery/11`). [map]
- Making PTY the default runner (a follow-up). [map]
- New follow-ups found during the map, including any serialising of different verbs sent to one
  session at the same time outside a deny with a reason. [map]
- Faster pid-file polling (rejected: the gap stays and idle CPU grows). [H-05]
- A screen-based input-line check for `/` commands (dropped). [H-07]
- gx closing a dialog that a dialog command opened. [H-07]
- A "key only" Interrupt variant for the budget stop. [H-11]
- Measuring Codex's context-exhaustion banner state. [H-14]
- A guard for the Codex `/model` picker or backtrack view. [H-14]

## Further Notes

- Claude facts were measured on Claude Code 2.1.303 with `--model haiku` and tiny conversations;
  real `/compact` time was not measured, and Esc from a real attached terminal was not tested.
  Evidence: `docs/research/pty-turn-detection.md` (branch `research/pty-turn-detection`) and
  `docs/research/claude-turn-signals.md` (branch `research/claude-turn-signals`). [H-02, H-06]
- Codex facts were measured on codex-cli 0.147.0 and herdr 0.9.3, one model, tiny conversations,
  some cases with one run. Evidence: `docs/research/codex-herdr-signals.md` (branch
  `research/codex-herdr-signals`). [H-13]
- gx-design's `--send "/clear"` goes through the Agents-tab action path, not `Pty.Prompt`; that path
  already treats a `/` step that stays idle as done. [H-02]
- Runner differences, by design: headless has no key rule and no Ctrl+U in `Interrupt`; the herdr
  deny hold lives only in the server process; the PTY holder's deny-reason step waits
  `holderDenyTimeout` per step, herdr's deny waits N; a dialog command leaves claude `blocked` but
  not Codex. [H-01, H-12, H-14]
