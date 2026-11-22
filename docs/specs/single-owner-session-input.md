# Single-owner session input: no false "input would not clear"

Map: `gx:single-owner-session-input` (research 01 to 03, decisions 04 to 07). Changes
`docs/specs/session-prompting.md` (first prompt ready signal, draft check stories 44 to 51).

## Problem Statement

A person sends a prompt from a chat draft, or starts a session, and the session hangs. The agent
shows `input would not clear <leader> a: <prompt>`, though the input holds no text.

- gx decides "ready" and "is there a draft" from a screen snapshot. A log from a second machine
  (Claude 2.1.330, managed corporate instance) shows the only `❯` row also holding the Claude logo
  and the "Claude Code v2.1.330" header. The check read the colored header as a draft, and no clear
  key could fix it. The cause of that torn frame is unknown and did not reproduce in about 300
  replays.
- The first prompt waits on a screen match on the input box, so one bad frame blocks it.
- Two attached clients (TUI and web UI) can type into the same session at once, and the PTY takes
  the size of whichever client last focused or typed. Different sizes are a possible source of torn
  frames, and two typists mix text.
- A session that fails this way shows nothing in the web UI beyond the card, and a quiet failure
  (typed on an unreadable screen) shows nothing at all, so the session looks hung.

## Solution

The first prompt starts on a signal from the agent's output stream, not on a screen read. The draft
check stays for later prompts but only acts on a read it can trust. One client at a time owns a
session's input. Every terminal issue shows in the web UI.

- **First prompt.** The PTY runner's holder watches the raw output for the agent's composer: for
  Claude, `❯` followed by U+00A0; for Codex, the OSC 7501 idle report gx already parses. For Claude,
  the first prompt starts on the earliest of the pid-file `idle`, an OSC 7501 idle report, that
  marker, or the first Idle. The screen match on the input box no longer counts. No draft is cleared before the first prompt: a new
  session's input is empty, because gx owns the input from launch until the first prompt's keys are sent.
- **Later prompts.** The draft check anchors on the cursor row and the composer frame. A startup
  header or logo on a `❯` row, a missing cursor row, or a PTY resize with no child output since
  make the read untrusted, and an untrusted read means "can't decide", so gx types. A trusted
  draft that still shows after the clear rounds raises "input would not clear". Text gx left itself
  (a stale `/clear`, a swallowed Enter) is cleared by the same clear.
- **Input owner.** The client (TUI or web UI) or gx that holds a session's keys. Only the owner's
  keys reach the agent. A non-owner's keys are blocked with a take-over hint. The person switches
  with an explicit control, never by focus. gx owns the input from launch until the first prompt's keys are sent. After that, gx takes the role only
  while it sends one batch of keys (ready-to-type, draft clear, text and Enter for one step) and hands it back during waits.
  Only the owner's resize changes the PTY size.
- **Issues in the web UI.** A non-blocking issue shows as a dismissible notice on the session. A
  blocking issue keeps its Actionable card and gains a Retry button.
- **Doctor.** `gx claude doctor` checks the composer marker against the installed Claude.

## User Stories

### First prompt

1. As a person starting a session, I want gx to type the first prompt once the agent's output shows
   its composer, so that a bad screen frame never blocks it.
2. As a Claude user, I want the composer recognized by `❯` followed by U+00A0 in the raw output, so
   that a startup menu (which draws `❯` then a cursor move) is not taken for the composer.
3. As a Codex user, I want the first prompt to start on the OSC 7501 idle report, so that Codex gets
   the same stream-based start.
4. As a person, I want the first prompt to start on the earliest of the pid-file idle report, an
   OSC 7501 idle report, the composer marker or the first Idle, so that a missing marker never
   hangs a session whose other signals fire.
5. As a person, I want the screen match on the input box dropped from the ready signal, so that a
   torn frame cannot delay or block the first prompt.
6. As a person, I want no draft clear before the first prompt, so that gx does not fail on a new
   session whose input is empty.
7. As a person with Claude's vim mode on, I want the vim step to still run after ready, so that the
   first prompt lands in INSERT mode.
8. As a person who starts a session with `--prefill`, I want gx to press Enter only and never type,
   so that the prefilled text is not doubled (the marker does not prove an empty input).
9. As a Codex user, I want Codex's ready-to-type to stay always true, so that nothing else changes
   for Codex.
10. As a person whose session never shows a signal, I want the existing 30s "agent still starting"
    Actionable, so that a stuck start is one key away from a retry.
11. As a maintainer, I want a log line with the Claude version when the pid file, an OSC 7501 idle
    report or first Idle fired but the marker was never seen, so that a Claude release that breaks the marker is noticed.
12. As a maintainer, I want the matcher tested on the real captures from the composer research, so
    that a regression in the marker shows in CI.

### Doctor

13. As a maintainer, I want `gx claude doctor` to launch the installed Claude, feed its raw output
    to the harness matcher, and fail if the composer marker never appears (the same check runs on the
    recorded startup capture in CI), so that I can test a new Claude version on demand.
14. As a maintainer, I want the same check to fail if a startup menu (trust dialog, theme menu)
    matches the marker, so that a menu is never taken for the composer.

### Draft check for later prompts

15. As a person with a draft in the input, I want gx to still clear it before it types a later
    prompt (an Action step, a web chat send to a live session), so that the texts never mix.
16. As a person, I want the check to anchor on the cursor row and the composer frame, so that text
    elsewhere on screen is never read as my draft.
17. As a person, I want a header or logo on a `❯` row to read as undecided, so that a torn startup
    frame never raises a false draft.
18. As a person, I want a read to count as untrusted when the cursor row is missing, the header or
    logo shows on the row, or the PTY was resized and the child has not written since, and an untrusted read to
    mean undecided, so that gx types as before instead of failing.
19. As a person, I want a trusted draft that still shows after 4 rounds of 8 Ctrl+U to raise "input
    would not clear" as an Actionable with the unsent steps, so that a draft that really won't clear
    is still reported.
20. As a person, I want text gx left itself (a stale `/clear`, a prompt whose Enter was swallowed)
    cleared by the same clear before a later prompt, so that Continue is never blocked by gx's own
    text.
21. As a maintainer, I want the attach sizes (each client's size and the PTY size) logged next to the
    "draft would not clear" line, so that a size tear can be diagnosed from the log.

### Input owner

22. As a person with two clients attached to one session, I want only the owner's keys to reach the
    agent, so that two typists never mix text.
23. As a person on the non-owner client, I want my keys blocked with a take-over hint, not queued, so
    that nothing I type reaches the agent later by surprise.
24. As a person, I want to switch the owner with an explicit control (a key in the TUI, a button in
    the web UI), so that I choose who types.
25. As a person, I want focus never to change the owner, so that glancing at a client does not steal
    input.
26. As a person, I want the PTY size to follow the last client owner when I take over, so that the
    agent paints for the screen I type on.
27. As a person, I want gx never to resize the PTY, so that gx's typing never causes a torn frame.
28. As a person with no previous client owner, I want the PTY to keep its last size (a new session
    keeps its spawn size), so that a session's size is never undefined.
29. As a person who attaches a client while no other client is attached, I want that client to
    become the input owner and set the PTY size, so that I never need a take-over key on a lone
    session and the PTY keeps a size someone views. Two cases wait for gx. Before the first prompt's
    keys are sent, gx owns the input, and the client becomes owner when that batch ends. During any
    later gx batch, the client becomes owner when the batch ends, and the PTY takes its size then.
    The take-over control is only for taking the input from another client; only a second client
    starts as a non-owner.
30. As a person, when no client is attached or the owner detaches, I want gx to own the input, so that
    the other client does not become owner without my action.
31. As a person, I want gx to hold the input only while it sends one batch of keys and hand it back
    to the previous owner after. One batch is ready-to-type (including the vim `i` and its re-check),
    the draft clear, the typed text and its Enter for one step (an Action step, the first prompt's
    text, a web chat send). Each later nudge Enter or retype is its own batch. gx hands the input back
    after each batch, including during waits (a nudge waiting for Working, an Action waiting for
    Idle), so that my client keeps the input once gx is done and is never locked out for minutes.
    Person input during a wait still cancels the remaining nudges (session-prompting).
32. As a person, when the previous owner detached while gx typed, I want gx to keep the input, so that
    no detached client is owner.
33. As a person, I want a safety timeout, counted from gx's last key in a batch, to release the input
    if gx dies mid-batch, so that a crashed gx never locks the session.
34. As a person, I want my take-over to be ignored while gx sends a batch of keys, so that I never
    cut a prompt in half.
35. As a person, I want each client to show who owns the input ("this client", "another client" or
    "gx"), and "gx is typing" while gx types, so that I know why my keys are blocked, also with
    two TUIs or several browser tabs on one session.
36. As a person on any non-owner client, I want a take-over control beside the indicator, so that the
    way out is visible.
37. As a person using ralph-loop on the PTY runner, I want its launch wait to use the new ready
    signal and its `PromptNudged` to follow the same rules as other sessions on the holder (the
    stricter draft check, ownership per batch of keys, the size rule), so that loop launches behave
    like other sessions.
38. As a person using sessions on the herdr runner, I want the stricter `InputIsEmpty` (cursor-row
    anchor, header or logo reads undecided, in `harness/typing.go`) to reach the herdr nudge's empty
    check too, and nothing else to change (no size rule, no owner model, which are holder-only), so
    that the shared harness rule is not forked.
39. As a person with two clients attached, I want only the owner's window resize to change the PTY
    and emulator size, and a non-owner's resize to change only its own view, so that a client I am
    not typing on never tears the agent's frame.

### Issues in the web UI

40. As a person in the web UI, I want a non-blocking issue (gx typed on an untrusted screen read, the
    first-prompt marker never seen) shown as a dismissible notice
    on the session, so that a quiet problem does not look like a hang.
41. As a person, I want a notice to clear when I dismiss it or when the session's next prompt works
    without the issue, so that stale notices do not pile up.
42. As a person, I want each notice case to have one fixed message text, so that I can tell causes
    apart and the text can be searched.
43. As a maintainer, I want the log line kept beside each notice, with the attach sizes, so that the
    web text and the server log tell one story.
44. As a person, I want a blocking issue ("input would not clear", the first-prompt timeout "agent
    still starting") to keep its gx-raised Actionable card, so that the failure stays one key away.
45. As a person, I want a Retry button on those cards that runs the failed prompt again, so that I
    do not retype it. On both cards Retry does the same as `a`: the draft check (the hardened
    `ClearDraft`), then the unsent steps. It does not restart the first-prompt path or wait for the
    first Idle.
46. As a person, I want the "input would not clear" card to name the input owner and offer Take over,
    so that I can clear the draft myself when another client owns the input.
47. As a person, I want a blocked key to show as a hint at the owner indicator in the client that
    typed it, going away on take-over or after a few seconds, so that I see why my key did nothing
    without a card or notice.

### Changes to session-prompting

48. As a maintainer, I want `docs/specs/session-prompting.md` updated (ready signal, draft clear,
    stories 44 to 51, first-prompt rules), so that the two specs do not contradict each other.

## Implementation Decisions

### Vocabulary

- **Input owner** as in `GLOSSARY.md`. **Actionable**, **Harness** and **Draft history** as there.
- **Composer seen**: a flag the holder sets once its output scanner has matched the harness's
  composer marker. It is per session and stays set.
- **Trusted read**: a screen read of the input where the cursor row is found, no header or logo
  sits on the input row, and the PTY was not resized since the child's last output. A read is untrusted from a PTY resize until
  the child's first output after it (or a short settle time, the implementer's choice). Any other read is untrusted.
- **Non-blocking issue / blocking issue**: as in the Solution. Blocking issues are the ones that
  stop a prompt (input would not clear, first-prompt timeout).

### Composer marker scanner

- The harness package defines the matcher. The holder runs it over the raw output and publishes the
  composer-seen flag. The first-prompt code reads the flag.
- Claude: `❯` followed by U+00A0 in the raw output. Codex: the OSC 7501 idle report, which the
  harness already parses.
- Claude's ready for the first prompt is the earliest of the pid-file `idle`, an OSC 7501 idle
  report, the marker and the first Idle (`PidWatcher.Ready()` keeps its pid-file and OSC 7501 legs).
  Only the screen input-box match is removed from Ready. Orca's 1.5s quiet fallback is not copied.
- Claude's Ready ignores `?2031h`: it only filters out shells and is not needed to tell the composer
  from a menu.
- The vim step runs after ready, as today. No draft clear runs before the first prompt.
- `--prefill`: no typing, Enter only (as the existing rule). Codex ready-to-type stays always true.
- No signal: `FirstPromptStartTimeout` and its "agent still starting" Actionable stay. When the
  pid-file idle report, an OSC 7501 idle report or first Idle fired but the marker was never seen, the first-prompt code logs
  one line naming the Claude version.

### Doctor check

- A new check in the existing Claude doctor table (`gx claude doctor`), a Fixture-mode check. In CI it runs
  on the recorded startup capture; on real Claude it runs through `gx claude doctor`, which launches
  the installed Claude and feeds the raw output to the harness matcher. It fails if the composer
  marker never appears. It also fails if a startup menu (trust dialog, theme menu) matches. The
  negative case is a hand-made fixture (a startup-menu stream with an NBSP after `❯`), not a
  recording. The check
  name is the implementer's choice.

### Draft check hardening (later prompts)

- `InputIsEmpty` anchors on the cursor row and the composer frame, as orca's draft reader does. A
  header or logo on a `❯` row is undecided. A missing cursor row is undecided. A read taken between a PTY
  resize and the child's first output after it is undecided. Undecided means gx types.
- `ClearDraft` keeps 32 Ctrl+U presses, in 4 rounds of 8. A trusted draft that still shows after
  the 4th round raises "input would not clear" as a gx-raised Actionable with the unsent steps (the
  existing behaviour; `TestSessionRename_DraftThatWillNotClearRaisesTheCard` stays).
- The same `ClearDraft` clears text gx left itself. No separate path.
- Applies to later prompts: Action steps after the first, web chat sends to a live session. Not to
  the first prompt, and not to deny-with-reason (unchanged).
- The size rule: the holder resizes its emulator and then the PTY back to back (`holder.go`
  `setSize`), so a winsize-vs-grid compare almost never fires. It also misses the real risk, a frame
  the child painted for the old size that is read after both sizes match. So a read is untrusted
  from a PTY resize until the child's first output after it (or a short settle time, the
  implementer's choice). The rule compares no client's size.
- The harness rules (the cursor-row and frame anchor, header or logo and missing cursor row read as
  undecided) live in `harness/typing.go` and also reach the herdr runner's nudge empty check. The
  size rule and the owner model are holder-only.
- The attach sizes are logged next to the "draft would not clear" line.
- The cause of the torn frame is not chased in this spec.

### Input owner

- Applies to sessions on the PTY runner (the holder) only.
- The holder tracks one owner per session: a client attach, or gx. The client's keys reach the agent
  only while it is the owner. This covers a TUI key and the web screen view's key bar and Type field.
- A key from a non-owner client is dropped and the holder tells that client the key was blocked. The
  client shows the take-over hint. Blocked keys never take ownership and are never queued.
- An explicit control moves ownership to the requesting client: a key in the TUI (the key is the
  implementer's choice), a button in the web UI. Focus and typing never move it.
- PTY size: on take-over, the PTY and the emulator take the new owner's size (what `ownLocked` does
  today on focus or typing, now moved to take-over). gx never resizes the PTY. With no previous
  client owner, the PTY keeps its last size.
- gx owns the input from launch until the first prompt's keys are sent. A client that attaches
  earlier becomes owner when that batch ends, so it never types a draft before the first prompt
  (no draft clear, the composer is empty).
- A client that attaches while no other client is attached becomes owner, and the PTY and emulator
  take its size. If gx is sending a batch, the attach is recorded as the owner gx hands back to when
  the batch ends, and the PTY takes its size then. Only a second or later client starts as a
  non-owner.
- Only the owner's resize changes the PTY and emulator size (today `frameHub.resized` calls
  `ownLocked` for any client). A non-owner's resize changes only its own view.
- No client attached: gx owns. The owner detaches: gx owns, not the other client. A person takes it
  with the control.
- gx holds ownership only while it sends one batch of keys and hands it back to the previous owner when the batch
  ends. One batch is ready-to-type (including the vim `i` and its re-check), the draft clear, the
  typed text and its Enter for one step (an Action step, the first prompt's text, a web chat send).
  Each later nudge Enter or retype is its own batch. During waits (a nudge waiting for Working, an Action waiting for Idle) gx holds nothing, so
  person keys reach the agent, and person input during a wait still cancels the remaining nudges as
  `session-prompting.md` says. If the previous owner detached meanwhile, gx keeps it. While gx sends
  a batch, person keys are blocked and the take-over control is ignored.
- A safety timeout, counted from gx's last key in a batch, releases ownership if gx dies mid-batch. Its value is the
  implementer's choice.
- Each client shows the owner ("this client", "another client" or "gx") and "gx is typing". Every
  non-owner also shows the take-over control. The owner state reaches the clients through the existing attach
  and session streams. Names and wire shape are the implementer's choice.

### Terminal issues in the web UI

- The holder and the prompt code report each issue to the server. The server puts it on the
  session's wire data through the existing session events. No new holder event type. The field name
  is the implementer's choice.
- Notices (non-blocking): gx typed on an untrusted read, marker never seen. A torn startup frame is
  not its own case: the first prompt reads no screen, and a torn frame at a later prompt is an
  untrusted read. Shown as a dismissible notice on the session. Cleared on dismiss or when the
  session's next prompt works without the issue. One fixed message text per case (the wording is
  the implementer's choice).
- Blocking: "input would not clear" and "agent still starting" keep the gx-raised Actionable card,
  and the card gains a Retry button. On both cards, Retry does the same as `a`: the draft check (the hardened
  `ClearDraft`), then the unsent steps. It does not restart the first-prompt path, has no fresh
  `FirstPromptStartTimeout` and does not wait for the first Idle. The draft card also names the input owner and offers Take over.
- Blocked keys show as a hint at the owner indicator in the client that typed. It goes away on
  take-over or after a few seconds (the duration is the implementer's choice). Not a notice or a
  card.
- The log line stays beside each notice, with the attach sizes (see the draft check).

### Changes to `docs/specs/session-prompting.md`

Made in the same change as this spec:

- Solution, "Ready signal": Claude's ready is the earliest of the pid file `idle`, an OSC 7501 idle
  report, the composer marker in the raw output, or the first Idle. The screen match on the input box is gone.
- Solution, "Draft clear": the first prompt has no draft clear. Later prompts use the hardened
  check; an untrusted read types. Only a trusted draft that won't clear raises the Actionable. The
  "clears a person's draft" line says the draft clear no longer covers the first prompt.
- Draft check stories 44, 46, 50 and 51 and the first-prompt, Action and testing text that repeat
  these rules are reworded to match. Stories 45, 47, 48 and 49 stand.
- A note under the header points to this spec.

## Testing Decisions

- A good test checks external behaviour only: which keys reach the session, what the server API and
  SSE report (session issues, owner), what the clients show. Not the scanner's internals.
- **Proposed seams** (for the caller to confirm):
  1. **Server interactive-session API** (highest, existing: `server/servertest` with the
     `agentfake` program fake). Covers: first prompt on the composer marker, on the pid-file idle
     report and on the first Idle; no screen leg; no draft clear before the first prompt; the
     timeout Actionable; the version log line when the marker never shows; later prompts typing on
     an untrusted read and raising "input would not clear" only on a trusted draft; owner rules
     (blocked non-owner keys, take-over, gx taking and returning ownership, ignored take-over while
     gx types, detach, safety timeout); issues on the session wire data and their clearing; Retry.
     The fake grows knobs for: drawing a header on a `❯` row, a missing marker, and a second and
     third attached client. A test hook in the holder triggers a PTY resize just before the read.
  2. **Harness typing rules** (existing table tests). The composer matcher on the real captures
     from the composer research (Claude marker, trust dialog and theme menu not matching, Codex
     OSC 7501), and `InputIsEmpty` on recorded screens: header on a `❯` row, cursor row missing,
     trusted draft, placeholder.
  3. **Web UI** (vitest and, only where a real round trip is needed, the existing Agents flows).
     Covers the owner indicator, take-over button, blocked-key hint, notice with dismiss, and Retry
     and Take over on the card.
  4. **TUI** (existing Bubble Tea tests and the real-binary TUI suite). Covers the owner indicator,
     the take-over key and the blocked-key hint.
  5. **Claude doctor** (existing table, a Fixture-mode check run against the recorded startup capture
   in CI, plus one hand-made negative fixture).
- **Prior art:** `server/sessions_test.go` and the actionable tests (seam 1);
  `TestSessionRename_DraftThatWillNotClearRaisesTheCard`; `harness/typing_test.go` with its screen
  fixtures and `harness/osc7501_test.go` (seam 2); `claudedoctor` fixture tests (seam 5); the holder
  attach tests for owner rules (seam 1).
- Tests follow `gx-go-tests` (readiness waits, process limits) and `gx-playwright-tests`.

## Verification

"Mock states" does not apply: the map stores no mocks. Entries use flows from `webui/e2e/CATALOG.md`
by catalog number. The scenarios below are named by role; each "new scenario" becomes an
implementation ticket, and the implementer picks its name. Stories 1, 3 to 11 and 15 to 39 (except
16) run against a fixture server on the PTY runner with the agent fake; stories 2, 12 to 14 and 16
run as `go test` or doctor runs; stories 40 to 47 also open the web UI.

Marker and first prompt (stories 1 to 12)

1. Run flow 85 (Create session). New scenario: an agent fake that draws a header on its `❯` row.
   Expected: the session still gets its first prompt; the input holds no `input would not clear`
   text.
2. Replay the real Claude captures through the matcher: `go test` on the harness package. The marker
   matches the composer and not the trust dialog or theme menu.
3. Codex fake that emits the OSC 7501 idle report: the first prompt is typed on that report. Run
   flow 85 on the Codex chat.
4. New scenario: four runs where the marker, the pid-file idle report, the OSC 7501 idle report and
   the first Idle fire first in turn; expected: the prompt is typed once each time, on the earliest.
5. New scenario: a fake whose input-box screen matches but no marker, pid report or Idle fires;
   expected: no typing before the 30s timeout (the test shortens it) and the "agent still starting"
   Actionable.
6. New scenario: a fake with a client attached before the first prompt that types a draft; expected:
   the client is not owner until the first prompt's batch ends (its keys are blocked), the first
   prompt arrives with no Ctrl+U sent first and no mixed text (assert the keys the fake received).
7. New scenario: vim mode on; expected: `i` is pressed once, then the prompt arrives intact.
8. `--prefill`: expected: only Enter reaches the fake, no typed text (extend the vim and prefill
   fake knobs).
9. Codex chat start (flow 85 on Codex): ready-to-type sends no keys.
10. A fake that never shows a signal: flow 85 with the shortened timeout shows the Actionable and
    `gx server logs` has no marker line.
11. New scenario: pid-file idle fires (and, in a second run, the OSC 7501 idle report), the marker
    never does; expected: `gx server logs` has one
    line with the Claude version.
12. `go test ./harness` on the capture fixtures (as 2).

Doctor (stories 13 and 14)

13. `go run . claude doctor` against the installed Claude: exit 0 and the new check PASS. In CI the
    same Fixture-mode check runs on the recorded startup capture.
14. `go test ./claudedoctor` with a hand-made fixture as its own canned session (a trust-dialog
    stream with an NBSP after `❯`): the check FAILs on it. The fixture is written by hand, not
    recorded. The live run of 13 stays positive only.

Draft check (stories 15 to 21)

15. Run flow 76 (Send and draft) on a live session with a seeded draft; expected: gx clears it and
    types, and the sent text arrives alone.
16. Harness table test: text elsewhere on screen, cursor on an empty composer; expected: empty.
17. New scenario: header on a `❯` row at a later prompt; expected: the prompt is typed, the session
    shows the "typed on an unreadable screen" notice (see 40).
18. New scenarios: cursor row missing, and a PTY resize triggered just before the read, with no
    child output since (driven by a test hook in the holder, not a fake knob); expected: each types
    and shows the notice. After the child's first output post-resize, the read is trusted again.
19. New scenario: a seeded draft that never clears; expected: after 32 presses in 4 rounds the
    session is a gx-raised Actionable "input would not clear" with the unsent steps. Run
    `TestSessionRename_DraftThatWillNotClearRaisesTheCard`.
20. Flow 153 (Send a command, typed once or nudged, and /clear) with a stale `/clear` in the input:
    expected: it is cleared and the next prompt lands.
21. `gx server logs` after 19 shows the attach sizes beside the "draft would not clear" line.

Input owner (stories 22 to 37)

22. New scenario: two clients attached to one session; keys from the non-owner never reach the fake.
    Three-client case: two TUIs or tabs plus gx typing; the indicator on each names "this client",
    "another client" or "gx", every non-owner shows Take over, and a take-over from the third client
    moves ownership from the second.
23. Same scenario: expected: the blocked client receives the blocked report; no queued text arrives
    after take-over.
24. Same scenario: the take-over control (TUI key, web button) moves ownership; the other client's
    keys are then blocked.
25. Same scenario: focusing the other client and typing a key leave the owner unchanged.
26. Same scenario: after take-over the fake's PTY size equals the new owner's size.
27. Same scenario: during a gx-typed prompt, the PTY size never changes (assert size reads).
28. Same scenario: a new session with no client owner keeps its spawn size.
29. New scenario: a client attaches to a session with no other client, after the first prompt;
    expected: it is owner with no take-over, its first key reaches the fake, and the PTY takes its
    size. A second client attaching starts as a non-owner. Also: a client that attaches while gx
    types (first prompt or a later batch) is owner when the batch ends, and the PTY takes its size
    then, not before.
30. Same scenario: the owner detaches; expected: the owner indicator shows gx, and the remaining
    client's keys are blocked until take-over. With no client attached, gx owns.
31. Run flow 76 with a client attached as owner: gx takes the input for each batch of keys, and the
    client is owner again after each batch and during the nudge and Action waits (the indicator shows
    "gx is typing", then the client). A person key during a nudge wait reaches the fake and cancels
    the remaining nudges.
32. Same as 31, but the previous owner detaches mid-batch; expected: gx keeps the input.
33. New scenario: gx's batch ends abnormally (test hook); expected: ownership releases after the
    safety timeout, counted from gx's last key in the batch (the test shortens it). A nudge wait
    longer than the timeout neither releases nor holds ownership mid-wait.
34. New scenario: a take-over request while gx sends a batch is ignored; the prompt arrives whole.
35. Run flow 156 (Key bar and Type field in the screen view) with two clients, then a third;
    expected: every indicator names the owner ("this client", "another client" or "gx"); "gx is typing" shows during a gx send.
36. Same: every non-owner shows the take-over control (web button, TUI key hint), also in the
    three-client case of 22.
37. A ralph-loop ticket launch on the PTY runner: the launch wait ends on the new ready signal,
    `PromptNudged` takes ownership for each batch of keys and hands it back (assert the owner
    reports), and its draft check uses the stricter `InputIsEmpty` and the size rule.
38. A session on the herdr runner: no owner indicator and keys behave as today, except the herdr
    nudge's empty check uses the stricter `InputIsEmpty` (harness table test with the header-on-a-`❯`
    row screen through the herdr path; existing herdr tests stay green).
39. Same two-client scenario as 22: the non-owner resizes its window; the PTY and emulator size do
    not change. The owner's resize changes both.

Issues in the web UI (stories 40 to 47)

40. New scenario with fixture sessions for each non-blocking issue (typed on an untrusted read,
    marker never seen); expected: a dismissible notice with its fixed text. Run flow 75
    (Read a session) to open it.
41. Dismiss a notice: it is gone; a notice left undismissed clears after the next prompt works.
42. Each case's notice text is distinct and fixed (vitest on the notice component).
43. `gx server logs` shows the log line beside each notice, with the attach sizes (as 21).
44. Cases 19 and 5: the Actionable card shows on the session and in the Agents tab.
45. On the cards of 44, press Retry; expected: for both, the hardened `ClearDraft` runs and then
    the unsent steps, as `a` does (the test asserts no first-prompt restart and no wait for first
    Idle, and that a draft typed meanwhile is cleared first). The card clears when the prompt lands.
46. Card of 19 with two clients attached: the card names the owner and Take over moves ownership.
47. The non-owner client shows the blocked-key hint at the owner indicator; it clears on take-over
    or after the timeout (shortened in the test), and no notice or card appears.

Spec changes (story 48)

48. `rg "screen match on its input box|clear it before it types its first prompt"
    docs/specs/session-prompting.md` finds nothing; `rg "single-owner-session-input"
    docs/specs/session-prompting.md` finds the pointer. Check by search.

## Out of Scope

- The size rule and the owner model on runners other than the PTY runner (the holder), including
  the herdr runner. The shared harness rules (story 38) and ralph-loop on the PTY runner (story 37)
  are in scope.
- Finding the cause of the torn startup frame. The spec only logs attach sizes.
- Stashing and restoring a person's draft (as in `session-prompting.md`).
- Orca's behaviour of typing anyway after one retry: gx keeps raising "input would not clear" on a
  trusted draft.
- Orca's 1.5s quiet fallback after `?2004h`.
- A prototype of the owner switch. The spec describes the indicator and the control.
- Queueing a non-owner's keys.
- A dismiss key for a gx-raised Actionable.

## Further Notes

- Research lives on local branches `research/torn-startup-frame`, `research/composer-stream-signals`
  and `research/orca-drafts` (the push to origin failed with an SSH key error).
- The `█` logo quick fix from the source chat is not carried over: this spec fixes the cause of the
  first prompt hanging, and the hardened check handles the header case.
- Orca's Codex screen-structure paths broke on a Codex release, which is why a stream signal is
  preferred. gx's Codex ready already relies on the OSC 7501 idle report.
