# Deliver primitive: one place that types a prompt and gets it sent

Map: `gx:deliver-primitive` (decisions 01 to 11, plus the person's answers on Dismiss, undecided
exits, `/compact`, the first-prompt exits, the launch retype and the undecided read). Research:
`docs/research/deliver-primitive-*.md`. Changes the prompt rules of `docs/specs/session-prompting.md`
and `docs/specs/single-owner-session-input.md`, and the resend rule of `docs/specs/agent-lifecycle.md`.

## Problem Statement

A person sends a `/` skill command from the web chat. Claude finds a hidden character in the pasted
text, removes it, shows "Removed 1 invisible character" and swallows the first Enter. The text
stays in Claude's input box and nothing runs. The person must go to the screen and press Enter
again.

This keeps coming back because gx has four separate "type text and press Enter" implementations:

- the first prompt and the nudge path (they strip hidden characters and press a swallowed Enter again);
- the Action step and `/rename` path;
- the chat send of a `/` command;
- the PTY runner used by ralph-loop.

Each past fix landed in one path, so the others kept the bug. The chat `/` send neither strips
hidden characters nor presses a swallowed Enter. Other problems of the same split:

- The Action step path dials a second holder connection, ignores the harness rule for pasting text
  and ignores the Codex submit key.
- Several layers can each press Enter again, so a slash command could run twice.
- gx's strip set is narrower than Claude's, so a character gx keeps still makes Claude swallow the Enter.
- A plain action step whose Enter is swallowed ends in a start timeout.
- When gx cannot tell whether a `/` command landed, it says nothing. A plain send that gives up
  shows only a star with a tooltip in the web chat, and so do a first prompt that gave up, an agent
  that never started and a prompt that never reached the input.

## Solution

Every prompt gx types into a session goes through one function, `Deliver`. It strips hidden
characters, takes the input, checks the input is ready, clears a draft, pastes images, types,
presses submit and checks the result. It is the only code that retries.

From the person's side:

- A `/` command sent from the chat, a `/rename`, an Action slash step and a launch slash prompt all
  land with one Enter, even when Claude swallows the first one.
- A slash command never runs twice. gx presses Enter again only when the input box row still starts
  with gx's own text, at most 3 times. It never retypes a slash command, with one exception: a
  launch prompt that Claude answered with `Unknown command:` (Codex: `Unrecognized command`).
- When gx cannot tell whether a `/` command landed, the chat shows a yellow card, **May not have
  been sent**, with the reason, the text to copy, Show screen and Dismiss. It has no resend, because
  the command could run twice. The TUI shows a yellow info row with the same reason and text and a
  copy key.
- A plain send that gives up shows a red **Send failed** card in the web chat with the same layout.
  An `Unknown command:` on a chat send, and Codex's `Unrecognized command`, show the red card with
  Resend and Restart agent, so the person can fix a missing skill.
- A session the web created (chat or Design) shows the same red card when its first prompt gives
  up, its agent never started or its prompt never reached the input. A launch prompt that Claude
  rejects also gets Resend and Restart agent. Ralph-loop keeps its own exit for a rejected launch
  prompt: a fresh-session retry, then `agent_prompt_stalled`.
- Dismiss on any of these cards clears it for every tab and for the TUI.
- A plain Action step gets the nudge and the retype, so a swallowed Enter no longer ends in a start timeout.
- A later successful send clears an earlier card.
- Ralph-loop's PTY runner uses the same function, so unattended agents get the same fix, including
  the slow-skill-registry retype of a launch prompt.

## User Stories

Chat send

1. As a person using the web chat, I want a `/` skill command to run after one send even when Claude swallows the first Enter, so that I never have to press Enter on the screen myself.
2. As a person, I want gx to remove every hidden character Claude would remove from my text before it types, so that Claude has no reason to stop and ask me to review.
3. As a person who writes emoji, I want a joiner or variation selector inside an emoji or a word to stay, so that my text is not damaged.
4. As a person, I want a joiner or variation selector at the edge of the text or next to a space to go, so that it cannot swallow the Enter.
5. As a person, I want plain text with a hidden character to go through the same strip, so that all sends behave the same.
6. As a person, I want a `/` command that lands to run exactly once, so that `/model` or a skill never executes twice.
7. As a person sending a plain message, I want gx to nudge and retype when the Enter is swallowed, so that the turn starts without my help.
8. As a person typing in the session while gx sends, I want gx to stop its retries, so that it does not type over my text.
9. As a person sending `/compact`, with or without arguments, I want it to keep the nudge policy, so that it behaves as it does today and runs once.
10. As a person pasting images with a `/` command, I want the images pasted first and the command typed after, so that both arrive as before.

Slash settle and the undecided read

11. As a person, I want gx to press Enter again only when the input box row starts with gx's own text, so that a draft of mine never gets submitted.
12. As a person, I want at most 3 extra Enters, so that a stuck box does not turn into a loop.
13. As a person, I want gx to ignore a dialog (`/model`, `/help`) that hides the input box, so that a stray Enter does not act inside it.
14. As a person, I want gx to ignore a running `/compact`, whose input box is empty, so that no extra Enter is sent.
15. As a person, I want gx to treat "our text gone" as landed, so that a local command that redraws the screen is not reported as unsent.
16. As a person, I want gx to read the screen again after a resize settles before it decides, so that a half-redrawn screen does not fool it.
17. As a person with a coloured slash draft, I want gx to see it as my text in the box, so that the swallowed Enter is pressed again (the colour of a valid command must not make the box look empty).
18. As a person, I want a text longer than 512 bytes, which Claude shows as `[Pasted text`, to count as ours only when its line count matches mine, so that its swallowed Enter is pressed again and a pasted draft of a person is not.
19. As a person, I want a wrapped multi-row draft to be read from its first row, so that a long command is handled.
20. As a person, I want gx to still find its text after Claude cleaned a character out of it, so that the settle Enter fires and the card does not appear for a send that only needs one more Enter.

The cards

21. As a person, I want a yellow "May not have been sent" card when gx cannot tell if my command landed, so that I know to look.
22. As a person, I want the card to show the exact text I sent in a copy box with a Copy button, so that I can paste it again myself.
23. As a person, I want the card to have no resend button, so that I cannot run a command twice by mistake.
24. As a person, I want a Show screen button on every failure card, so that I can see what Claude shows.
25. As a person, I want a Dismiss button that clears the card for every tab and the TUI, and clears only these delivery-failure cards, so that a stale card does not stay and I cannot wipe a question or a draft card by mistake.
26. As a person, I want a plain send that gave up to show a red Send failed card in the chat, so that I do not have to find a star with a tooltip.
27. As a person who sent a missing skill, I want the red card to say `Unknown command` and offer Resend and Restart agent, and I want the card and its Resend to still be there after the restart, so that I can fix the skill, restart and send again.
28. As a person using Codex, I want `Unrecognized command` to show the same red card with Resend and Restart agent, so that Codex and Claude behave alike.
29. As a person, I want Restart agent to ask for a confirm, then stop and resume the session, so that I do not lose it by a misclick.
30. As a person, I want a later successful send to clear the card, so that the chat does not show a stale stop.
31. As a person using the TUI, I want a yellow info row with the reason and the text, a copy key, and "no action" on the Action key, so that the TUI has the same safety.
32. As a person using the TUI, I want the red Send failed row to keep Run action (resend), so that today's TUI flow does not change.
33. As a person, I want every delivery failure that gx did not recover to show as a card in the web UI, so that nothing fails silently or only in a log or a star.
34. As a person, I want the copy box to hold the text as I sent it, so that what I paste is what I wrote.

Action steps, rename and the first prompt

35. As a person running an Action, I want a plain step to use the nudge and the retype, so that a swallowed Enter no longer ends in a start timeout.
36. As a person running an Action with `/clear` then a skill, I want the slash step to end on a turn, a new conversation id or the settle window, as now, so that the Action stays fast.
37. As a person whose Action step ends undecided, I want the not-sent card for that step with later steps dropped and the reason "N later steps not sent", so that nothing runs out of order and nothing is offered that could run twice.
38. As a person renaming a chat, I want `/rename` to land with one Enter and show the card if gx cannot tell, so that a rename is not lost.
39. As a person whose chat is Blocked on a dialog, I want a held rename to type after the dialog closes, as now, so that the rename still works.
40. As a person starting a session with a slash first prompt, I want the swallowed Enter handled by the same code, so that a launch skill lands.
41. As a person, or a ralph-loop operator, starting a Claude or a Codex session whose skill is not loaded yet, I want gx to retype the launch prompt for about 30 seconds after it sees `Unknown command:` (Codex: `Unrecognized command`), so that a slow skill registry recovers. This holds for the server's first prompt and for the PTY runner's launch prompt.
42. As a person, I want a chat send that gets `Unknown command:` to fail at once with the red card and no retype, and a Codex chat `/` send to type once, so that I fix the cause myself and nothing runs twice.
43. As a person starting a session, I want a launch slash prompt that ends undecided to be logged and left to the launch proof, so that no card appears for a launch the proof still checks.
44. As a person, I want a draft that will not clear or an agent that is still starting to keep their existing "gx stopped" cards with Retry, so that those cards do not change.

Launch prompt exits

45. As a person who created a chat or Design session, I want a launch prompt that Claude rejected for good to show the red card with Resend and Restart agent, so that I can fix the skill and send again.
46. As a person who created a session, I want a first prompt that gave up to show the red Send failed card, so that it is not a star with a tooltip.
47. As a person who created a session, I want an agent that never started to show the red Send failed card, so that I know why nothing happens.
48. As a person who created a session, I want a prompt that never reached the input to show the red Send failed card, so that I can copy it and try again.
49. As a ralph-loop operator, I want a rejected launch prompt to keep the fresh-session retry and then `agent_prompt_stalled` with Retry, so that ADR 0042 holds and no new pause kind exists.

PTY runner and Codex

50. As a person running ralph-loop, I want its PTY runner to use the same function, so that unattended agents get the same fix.
51. As a ralph-loop operator, I want an undecided slash send on the PTY runner to be reported as not delivered, so that the supervisor retries in a fresh session and then raises `agent_prompt_stalled`.
52. As a person using Codex, I want a `/` chat command typed once with one Enter and no settle Enter, so that Codex, which has no input-box reader, is not guessed at.
53. As a person using Codex, I want its `Unrecognized command` pane text to fail the send, so that I see the red card.
54. As a person using Codex, I want plain, action, first-prompt and nudge sends to use the nudge policy with Codex's own submit key (Tab while a turn runs), so that queued sends still work.
55. As a person using Codex, I want the same hidden-character strip, so that all harnesses are the same.
56. As a person pasting images into a Codex send, I want the harness paste rule used, so that images arrive as before.

Guards, tests and docs

57. As a maintainer, I want a table test over every entry point against a fake holder that swallows the first Enter, so that no path can lose the fix again.
58. As a maintainer, I want an AST guard that fails when code outside an allowlist types text, pastes or presses submit, so that a fifth implementation cannot appear.
59. As a maintainer, I want `applySGR` fixed on `38;2;R;G;B` and `38;5;N`, so that a coloured slash draft stops reading as empty everywhere (including the person-input relay).
60. As a maintainer, I want the stacked retry layers gone (`PressEnterIfStillTyped`, the `waitForSlash` Enter, LaunchProof's `Resend`, `TypeSessionPrompt`), so that Enter cannot be pressed 3 times 3 times.
61. As a maintainer, I want `server/sessions/prompt` to shrink under its source-line limit, so that the size guard passes with room.
62. As a maintainer, I want `docs/specs/session-prompting.md`, `docs/specs/single-owner-session-input.md`, `docs/specs/agent-lifecycle.md` and the glossary updated, so that the docs match the code.
63. As a user reading the changelog, I want a line about the fix, so that I know the double-Enter bug is gone.

## Implementation Decisions

Module and ownership

- A new leaf package `agentrunner/deliver` holds `Deliver(target, text, Policy)`. The invisible-character package and the nudge engine (`PromptWithNudge`, `NudgeCaps`, `Prompter`) move into it; `claudeui` keeps only the screen readers. `claudeui` never imports `deliver`. The old prompt-package `invisible` is deleted. `server/sessions/prompt` and `nativerunner` both call it. `nativerunner` imports `agentrunner/...` and never `server/...`.
- `Deliver` takes a small `Target` interface: screen read, type, paste, keys and transcript. The method list is the implementer's choice. There is exactly one production implementation of `Target`, written over the holder client that both runners already use, and it lives in `agentrunner/deliver` (so the guard's allowlist covers it). The server's holder prompter and the PTY prompter are deleted as typing code: they keep only what is not typing (person-input detection, the start signal, cancel), supplied to `Deliver` as callbacks or a thin wrapper that calls none of the flagged methods. Tests use a fake `Target` or the fake holder.
- `Deliver` owns, in order: strip, lock and batch (one holder connection, one input owner), ready check, clear draft, paste images, type, submit, verify. Only `Deliver` retries.
- `Policy` is `{TurnStarting, Settle, Nudges, Retypes}`, with the launch retype window and its caps (today's `UnknownGrace` and `ResendEvery`) as launch-policy fields. The tests keep a way to shorten them, as the server and PTY test harness does today. Three shapes exist: the turn-starting policy (nudge and retype, start signal is the holder's turn), the slash policy (type once, settle Enter only on a trusted read) and the launch policy (the slash or turn-starting shape plus the `Unknown command` retype window). The implementer picks the field types and constants. The slash policy keeps today's timings: 1 second settle, 1 second between Enters, at most 3 extra Enters.
- A text whose first character is `/` gets the slash policy, except `/compact` with or without arguments, which keeps the turn-starting policy with no special case. The wire layer already refuses a `!` prefix, so `Deliver` has no `!` case.

Strip

- `Deliver` strips Claude's full hidden-character set for every send, slash or plain, on every harness. The set is pinned to Claude 2.1.330, and a unit test pins exactly this list:
  - C0 controls except TAB and LF (U+0000-U+001F minus U+0009 and U+000A), DEL and C1 (U+007F-U+009F);
  - U+2028, U+2029;
  - Claude's table: U+00AD, U+034F, U+061C, U+115F-U+1160, U+17B4-U+17B5, U+180B-U+180F, U+200B-U+200F, U+202A-U+202E, U+2060-U+206F, U+3164, U+FE00-U+FE0F, U+FEFF, U+FFA0, U+FFF0-U+FFFB, U+1D173-U+1D17A, U+E0000-U+E0FFF, U+E0100-U+E01EF;
  - lone surrogates.
- ZWJ (U+200D), ZWNJ (U+200C) and variation selectors (U+FE00-U+FE0F, U+E0100-U+E01EF) stay next to a letter or an emoji. They go at the edge of the text or next to a space.
- The set will drift when Claude changes it. The settle Enter is the guard for that, so the strip is not trusted as the only fix.

Slash policy and the settle read

- The slash policy types once and never retypes. It presses Enter again only on a trusted read where the input box holds our text, up to 3 times.
- The one read it trusts: `claudeui.InputBox` on plain rows is the gate (a dialog or a bare rule gives no box), then the box row, compared after removing Claude's whole hidden-character class set (joiners and selectors included) from both sides, must start with our first 20 characters, or hold `[Pasted text` together with a `+M lines` count equal to our text's line count. No colour parsing, no whole-screen read. A person's own draft does not match and gets no Enter. The paste rule rests on the draft clear that runs first, so a person's older paste is already gone.
- `holdsText`, `draftIsOurs` and `screenShowsPrompt` are deleted. `nudgeEnter` moves into `Deliver`. `PressEnterIfStillTyped` and the `waitForSlash` Enter press are deleted.
- The undecided read (a resized read, a torn frame, no box) is not silent. Definitions:
  - "Our text is visible": the input-box row holds it when a box exists. When there is no box, any screen row holds it except the echo row of the command (the `❯ /cmd` line Claude draws for a running or open command) and the suggestion-menu rows.
  - The "screen changed" baseline is the frame right after gx typed.
  - The window is the slash settle plus the resize cap.
  - Steps: (1) re-read after the resize settle: poll every 25 ms until the read is no longer resized, cap 1 second (log the poll count on an undecided exit); (2) at the end of the window, our text gone means landed, with the screen changed or not, and gx presses nothing; (3) our text visible, with the screen unchanged or changed, ends the send as "may not have been sent".
- The one slash retype: a launch prompt retypes after a seen `Unknown command:` on the pane (Codex: `Unrecognized command`), for about 30 seconds, logging `prompt-nudged` each time. The command did not run, so the retype cannot run it twice. Only the launch policy sets this window, and it applies to the server's first prompt and to the PTY runner's launch prompt. A chat `/` send that sees `Unknown command:` fails at once. A Codex chat `/` send still types once. LaunchProof stays a landed check with no `Resend`. The old "transcript has no user record" trigger is dropped, because Claude writes a local command's record only after the command ends.
- Codex `/` chat send: type once, one Enter, no settle Enter. `checkCodexKnownCommand` moves into `Deliver` as the failure signal. Every other Codex send uses the turn-starting policy with Codex's own submit key from the harness typing object, and the harness paste rule for text and images.

Exits and cards

- Every `Deliver` failure that is not recovered raises a card the web UI shows. Exits, by runner:
  - web-created sessions (chat, Design), the server:
    - plain send gives up: red Send failed;
    - slash undecided: "May not have been sent";
    - `Unknown command` on a chat send, or Codex `Unrecognized command`: red card with Resend and Restart agent;
    - first prompt gives up, agent never started, prompt never reached the input: red Send failed;
    - launch prompt rejected for good: red card with Resend and Restart agent (Resend sends the launch prompt again);
    - draft would not clear or agent still starting: existing "gx stopped" card with Retry;
    - person typed, or the server is stopping: no card (as today);
  - ralph-loop (PTY runner): a rejected or undecided launch prompt is returned as not delivered, so the supervisor retries in a fresh session and then raises `agent_prompt_stalled` with Retry. No pause kind is added (ADR 0042).
- The three delivery-failure cards carry a structured `Kind` on the session's Actionable: "may not have been sent", "send failed" and "unknown command". The web, the TUI and the clear-on-success rule switch on `Kind`; matching on the reason text (the old "Send failed: " prefix) is dropped. The implementer picks the three values. A "may not have been sent" card has the reason, the text to copy (as sent, not stripped) and no Action. The TUI Action key reports "no action" for it. The red cards carry the same copyable text. Resend and Restart agent appear only on the Unknown command kind.
- A Dismiss session verb clears the Actionable on the server for every tab and the TUI. It clears only the three delivery-failure kinds. Any other Actionable (a blocking draft or starting card, an agent-raised one, a derived Design or grilling one) gets a distinct refusal reason, and so does a session with no card. It has a mirrored CLI verb (ADR 0028); the implementer picks both names. The clear-on-success rule that drops an earlier Send failed card also drops the other two kinds.
- Restart agent is Stop then Resume behind a confirm, built on the existing verbs. No new restart verb. An Unknown command card survives it (the exit that normally replaces an open Actionable does not drop this kind when the restart is the person's own), so Resend works after the restart. Dismiss or a successful send clears it.
- Show screen reuses the existing screen modal. TUI: the yellow row holds the reason and the text; a new copy key copies the text (the mock shows `<leader> y`; the implementer picks the key if it is taken). The red Send failed row keeps Run action.
- The web red Send failed card has Copy, Show screen and Dismiss and no Resend (as in the mock). The old star-with-tooltip display is replaced by the card.
- Wording says "see the screen", never "check the terminal".

Entry points and what changes

- Chat send: plain goes to the turn-starting policy, slash to the slash policy. Chat slash send and `/rename` stop using the Action-step typing path.
- Action steps: `typeStepWith` calls `Deliver`. Plain steps use the turn-starting policy. A slash step still ends on the first of a full turn, a new conversation id or the settle window; the settle Enter is `Deliver`'s. An undecided step raises the not-sent card for that step, drops later steps and says "N later steps not sent" in the reason. No remaining-steps Action is offered.
- `/rename`: unchanged hold logic for a Blocked chat; the typing and Codex check go through `Deliver`; failure goes through the same card exits as a chat send.
- First prompt (server): the holder prompter, its submit key, clear and slash delivery move onto `Deliver`. An undecided slash exit is logged and left to LaunchProof, with no card. The first prompt keeps its rule of no draft clear. Its give-up, never-started and never-reached-input exits raise the red card instead of today's non-blocking one-step Actionable.
- `TypeSessionPrompt` is deleted (it ignored the harness paste rule and the working submit key, and dialled a second holder connection).
- PTY runner (commit 2): the prompter and `promptSlash` move onto `Deliver`. The PTY runner's person-input relay moves to its own file, so the guard's file-level entry covers only the relay. An undecided slash exit and a rejected launch are returned as not delivered. The launch proof and its Codex twin lose `Resend`; their retype is the launch policy's.
- The headless runner and herdr are out of scope: headless sends stream-json and herdr has no typing call.

Input-box read fix

- `applySGR` skips the arguments of `38;...` and `48;...` colour codes. It is its own small ticket with unit tests on `38;2;R;G;B` and `38;5;N`. It repairs the person-input relay on the PTY runner, which stays and shares the bug.

Guard

- A new top-level package `deliverguard` holds an AST test, in the style of the existing state and env guards. It parses non-test, non-`testutil` Go files and uses type information to resolve constants. It fails on a selector call to `TypeText`, `TypeTextBy`, `TypePersonTextBy` or `Paste`; a `SendKeys` call whose argument is any constant (a literal or a named constant) equal to `"\r"`; and calls to `SubmitKey(...)` or `ClearKey()`.
- Allowlist by file or directory, each with a reason in the test, and a stale-entry check (an entry that no longer has a flagged call fails): the holder client and its answer helpers; `agentrunner/deliver` (including the one `Target` implementation); the doctor's image-paste diagnostic; the person-input relay in the prompt package (`TypePersonTextBy`); the web attach relay and the TUI attach relay (their `Paste`); the PTY person-input relay file; the emulator's own `Paste` (not a holder call); and the harness typing object, which only delegates `ClearKey` and sends nothing. The vim probe sends only `i` and a backspace, which are not flagged, so it has no entry.
- No entry for the old prompters, typing, session-watch, send, first-prompt or the rest of the PTY prompt files. They move onto `Deliver`, so the guard fails if one stays.
- The guard lands in commit 2, after the PTY move.

Docs and size

- Update `docs/specs/session-prompting.md` (one retry owner, the slash settle, the not-sent card), `docs/specs/single-owner-session-input.md` (the batch is `Deliver`'s), and `docs/specs/agent-lifecycle.md` (the launch resend is `Deliver`'s `Unknown command` retype). Add `Deliver` and the not-sent card to the glossary. Add a CHANGELOG line.
- `server/sessions/prompt` is at 4,469 of 4,500 source lines. Moving the nudge engine, strip, clear, settle and typing code out to `agentrunner/deliver` must shrink it. The new Dismiss route goes in a different package or the net must still shrink the package. Its test files are at 4,382 of 5,000 lines and 167 of 180 test functions, so new tests go elsewhere (see Testing Decisions).

Rollout, one PR, two commits

- Commit 1 (prompt package): the table test with the chat-slash row first (it lands with the fix so CI stays green), `Deliver`, `typeStepWith` and chat sends onto it (chat slash, `/rename`, actions, first-prompt slash), the cards and their `Kind`, the Dismiss verb, the `applySGR` ticket, and deletion of `TypeSessionPrompt`, `PressEnterIfStillTyped`, the `waitForSlash` press and LaunchProof `Resend`. Because the engine and `Resend` leave `claudeui` here, commit 1 also makes the smallest `nativerunner` edits needed to compile: import paths and dropping `Resend`. The real PTY prompter move is commit 2. Between the two commits the PTY launch has no retype; the PR is one unit, and commit 2 restores it through the launch policy.
- Commit 2 (nativerunner and PTY): the PTY prompter and `promptSlash` onto `Deliver`, the launch policy for the PTY and Codex launch prompts (the caps and their test override move into it), the AST guard, the three spec updates, the glossary entry and the CHANGELOG line.

Implementer's choices (no further decision needed): names of the `Target` methods, the `Policy` field types, the three `Kind` values, the Dismiss verb and CLI names, the TUI copy key, the fake holder (extend the existing Claude TUI fake), and what `Deliver` logs (poll count, settle time, our-text-visible flag). No Verification entry reads a log line the implementer names, except the existing `prompt-nudged` event.

## Testing Decisions

A good test checks what the session and the person see: the bytes the fake received, how many times a command ran, and which card shows. It does not check `Deliver`'s internals, its call order or its private helpers. Wait on readiness, never sleep (see the Go test rules).

Seams

- One seam for delivery: the holder protocol. `Deliver` is tested through every real entry point against a fake holder, so a new entry point that bypasses it fails.
- One seam for the card: the session's Actionable (its `Kind`) as the server and the clients render it.

Table test over every entry point (commit 1, first)

- Package: `promptflows` (3,076 test lines and 101 test functions, so it has room); the prompt package is near its test limits. The PTY rows are added in commit 2 in the PTY runner's own tests, against the same fake holder.
- Entry points: chat plain send, chat slash send, `/compact` send, `/rename`, Action plain step, Action slash step, first-prompt plain, first-prompt slash, Codex plain, Codex slash, and (commit 2) the PTY runner plain, slash and launch.
- Fake holder: extend the existing Claude TUI fake. It must model what `docs/research/deliver-primitive-input-box-read.md` lists:
  1. a real input box (rule, draft row, rule) on every frame where Claude has one, with the draft in that row;
  2. a highlighted draft in truecolor for a known slash command, so the `applySGR` bug is visible and the test fails on a broken reader;
  3. a suggestion menu under the box for a `/` draft;
  4. the swallow: for text starting with `/` or `!` holding a character the fake treats as hidden, the first Enter strips the character (and rewrites the box to the cleaned text), keeps the text, shows the notice and does not submit; the second Enter submits;
  5. dialog frames for `/model` and `/help` (styled echo row, one rule, body, no box), Esc closes, and Enter in the `/model` dialog saves and closes it so a stray Enter is visible;
  6. a running `/compact` (echo, spinner, empty box) that ends later, with a counter of compactions run;
  7. a landed local command left in scrollback with an empty box below;
  8. the truecolor environment, as the holder forces it.
- Rows from ticket 07: (a) U+200B in the prompt: stripped, one Enter lands, a slash command ran once; (b) a character outside gx's strip set, which the fake swallows by its own rule and sits inside the first 20 characters: the box no longer starts with the typed text, the comparison after removing the class set still matches, the settle Enter lands once and the command ran once. U+2028 is a strip case only.
- Strip rows: an emoji ZWJ sequence inside a `/` command stays and runs once; a ZWJ at the edge of the text is dropped.
- Row from the person's answer: `/compact` where the fake swallows the first Enter: the nudge policy lands it and one compaction runs.
- Input-box read screens from ticket 09, each a row: placeholder text in the box; a paste over 512 bytes shown as `[Pasted text #N +M lines]` (ours, line count equal: Enter) and a person's own paste with a different line count (no Enter); a wrapped multi-row draft; a person's own draft in the box (no Enter); a torn frame during resize; a permission dialog (`❯ 1. Yes` inside rules, no Enter); plus the 14 recorded screens (idle, typed with menu, `/model` and `/help` open and landed, after a chat turn, `/compact` running and done, paste unsent, swallowed, after Esc, cleared). The spec does not call the box-row read verified for the uncovered screens; these rows are what covers them.
- Undecided rows: our text visible and screen unchanged, and our text visible and screen changed, each give "may not have been sent" and no resend; our text gone, with the screen changed or not, gives landed with no Enter; a no-box screen whose only match is the echo row or a menu row does not count as visible; each entry point's mapping of the undecided exit (chat and rename: card; Action step: card with later steps dropped; first prompt: logged; PTY runner: not delivered).
- Exit rows, one per exit in the exit list: each raises its card kind (or, for the PTY runner, returns not delivered); this is how "every failure shows" is checked.
- Unknown command rows: chat slash fails at once with the red card; a launch prompt retypes for the window, on Claude and on Codex, then ends in its exit (the card on a web-created session, not delivered on the PTY runner). The PTY test harness shortens the window as it does today.
- Codex rows: `/` sends once with one Enter and no settle Enter (the received keys are asserted here, because the Codex fixture fake records no input); `Unrecognized command` fails; a plain send uses Tab as the submit key while a turn runs.
- No row may press a stacked Enter: assert the total Enters the fake received.

Other tests

- Unit tests for the strip (the pinned set, edge and in-word joiners, emoji intact) and for the fixed `applySGR` (`38;2;R;G;B`, `38;5;N`, `48;...`).
- Server tests for the Dismiss verb and its CLI mirror (the three kinds clear, every other Actionable and an empty session are refused with distinct reasons), for clear-on-success of the three kinds, and for the Unknown command card surviving Restart agent.
- The AST guard (ticket 11, commit 2) is a Go test in `deliverguard`: it fails on an unlisted call, on a `SendKeys` of a named constant equal to `"\r"`, and on a stale allowlist entry; each entry carries a reason. Its planted-call case is the proof that it catches a bypass.
- Web component tests for the card kinds and their buttons, with Playwright flows for the user-visible behaviour (see Verification).
- Prior art: the existing prompt-package tests (`sessionprompt_test.go`, `retry_test.go`, `rename_test.go`, `promptflows`), `nativerunner/pty_personinput_test.go`, the Claude TUI fake and `testutil/agentfake`, and the existing AST guards for state and env.

## Verification

Entries use flows from `webui/e2e/CATALOG.md` by number. No entry reads a log line the implementer names. Stories 1 to 49 run against a fixture server with the agent fake; stories 50 to 63 are `go test`, ralph-loop fixture and source checks.

Chat send, strip and slash settle (stories 1 to 20)

- Stories 1, 2, 5, 6, 7, 8, 10: run flow 76 (Send and draft), flow 153 (Send a command, typed once or nudged, and /clear) and flow 189 (Send images, including `/compact foo`), with the swallowing fake mode (new scenario S1); expected: the command ran once (the fake's count) and `gx agents chat` shows one message.
- Stories 3 and 4: the strip unit tests and the table rows with an emoji ZWJ sequence inside a `/` command (kept, one run) and a ZWJ at the edge (dropped). The catalog flows send no emoji.
- Story 9: run flow 153 with S1 on `/compact`; expected: one compaction.
- Stories 11 to 15, 17 to 20: `go test` of the table test (the screens above).
- Story 16: the table rows for a torn frame and a resize; expected: the send ends landed or "may not have been sent" by the rules above, never silently.

The cards (stories 21 to 34)

- Stories 21, 22, 24 and 34: see Mock states line `web-may-not-be-sent`. Run the new flow; expected: Copy puts the text as sent on the clipboard; Show screen opens the screen modal.
- Story 23: the same flow; expected: the card has no Resend button.
- Story 25: new flow step: click Dismiss, then reload and open a second tab; the card is gone in both. CLI: the new dismiss verb on a session with a card prints the cleared session and exits 0; on a session with a blocking draft card or an agent-raised Actionable it exits with the other-Actionable refusal; on a session with none it exits with the no-card refusal. `gx agents list` shows no card after the first.
- Story 26: see Mock states line `web-send-failed`.
- Stories 27 to 29: see Mock states line `web-unknown-command`. After Restart agent (confirm, then `gx agents list` shows the session stopped and resumed) the card and its Resend are still there, and Resend lands the command. Codex variant: the new Codex `Unrecognized command` scenario (S5); expected: the same card.
- Story 30: run flow 76 after a failed send; expected: the card is gone after the next good send, for each of the three kinds.
- Stories 31 and 32: see Mock states line `tui-not-sent`. TUI check: the Action key says "no action"; the copy key puts the text on the clipboard; the red row keeps Run action.
- Story 33: the table test exit rows (each exit raises its card kind) and the exit flows under "Launch prompt exits".

Action steps, rename and the first prompt (stories 35 to 44)

- Story 35: run flow 153 and flow 154 (Commands in a design session) with S1 on a plain action step; expected: the turn starts, no start timeout.
- Story 36: run flow 154 (`/clear` then a skill); expected: the step ends on the new conversation id, not the timeout.
- Story 37: new scenario S2: a two-step Action whose first slash step stays undecided; expected: the not-sent card with the reason "N later steps not sent" (here N is 1) and the second step never typed (assert the fake's received input).
- Story 38: run flow 75 (rename of the Working chat) and flow 153 (Codex rename: no Send failed card); `gx agents list` shows the new name.
- Story 39: run flow 158 (held rename of a Blocked chat); expected: the rename types once after the sheet answer.
- Story 40: run flow 85 (Create session) with S1 on a slash first prompt; expected: the prompt lands once.
- Story 41: `go test` of the launch retype on the server and on the PTY runner, for Claude (a fake whose skill registry fills after 10 s: `prompt-nudged` events, then a started turn; the window shortened by the test caps) and for Codex (`Unrecognized command`, then a started turn).
- Story 42: see Mock states line `web-unknown-command` for the chat send; the table Codex row for the single typing.
- Story 43: new scenario S3: a launch slash prompt that stays undecided; expected: no card on the session and the launch proof decides.
- Story 44: run `go test ./server/sessions/prompt -run 'DraftThatWillNotClear|Retry'` (`rename_test.go`, `retry_test.go`) and the web `BlockingCard` test; expected: unchanged.

Launch prompt exits (stories 45 to 49)

- Story 45: new scenario S4: a web-created session whose skill the fake rejects for good; expected: the red card with Resend and Restart agent (see Mock states line `web-unknown-command`); Resend sends the launch prompt.
- Story 46: new scenario S4: a first prompt that gives up; expected: the red Send failed card.
- Story 47: new scenario S4: an agent that never starts; expected: the red Send failed card.
- Story 48: new scenario S4: a prompt that never reaches the input; expected: the red Send failed card.
- Story 49: new ralph-loop fixture S6 (a ticket whose launch skill is rejected for good on the PTY runner); expected: a fresh-session retry, then `agent_prompt_stalled` with Retry in `gx tickets log`.

PTY runner and Codex (stories 50 to 56)

- Stories 50 and 51: `go test ./nativerunner` and the table test PTY rows; plus new ralph-loop fixture S7 (a fake that stays undecided on a slash prompt); expected: fresh-session retry, then `agent_prompt_stalled`.
- Stories 52, 54: the table test Codex rows only (flow 153 and the Codex fake record no input, so received keys cannot be checked in a flow).
- Story 53: the new Codex `Unrecognized command` scenario S5 (see story 28).
- Stories 55 and 56: the strip unit tests; run flow 189 on the Codex session for images.

Guards, tests and docs (stories 57 to 63)

- Stories 57 and 58: `go test ./deliverguard` and the table test, including the guard's planted-call case.
- Story 59: `go test ./harness` on the `applySGR` cases.
- Story 60: `rg -n 'TypeSessionPrompt|PressEnterIfStillTyped'` over non-test Go code finds nothing; `rg -n 'Resend' agentrunner/claudeui nativerunner --glob '!*_test.go'` finds no launch-proof `Resend` field or use; the `waitForSlash` body has no Enter press (a code read). The ralph-loop classifier resend names and the web "Resend" button are unrelated and not searched.
- Story 61: `go test ./sizeguard`; expected: `server/sessions/prompt` is under 4,500 with margin.
- Stories 62 and 63: read the three updated specs, the glossary and CHANGELOG for the `Deliver` text.

New flows or scenarios to write (each becomes an implementation ticket; the implementer picks names)

- S1: a swallowing Claude fake mode (first Enter swallowed for `/` text holding a hidden character, and for a plain action step) on a fixture scenario, for flows 76, 153, 154, 85 and 189.
- S2: an undecided two-step Action (the first slash step's text stays in the box).
- S3: an undecided launch slash prompt.
- S4: first-prompt exits on a web-created session: launch rejection for good, give-up, agent never started, prompt never reached the input.
- S5: a Codex `Unrecognized command` fake mode, with a Codex chat.
- S6: a ralph-loop fixture with a launch skill rejected for good on the PTY runner.
- S7: a ralph-loop fixture with an undecided slash prompt on the PTY runner.
- The three card scenarios of the Mock states list (below) and a TUI frame check for `tui-not-sent`. Web steps go into existing agents flows where the flow budget in `CATALOG.md` and `maxFlows` requires it.

Mock states

- `/Users/david/.local/share/gx/tickets/gx/deliver-primitive/mocks/06-not-sent-card.html`, state `web-may-not-be-sent`, width 640, fixture scenario: new (a chat whose fake leaves a `/model opus` draft in its box with an undecided read), parts: `card`, `reason`, `copy`, `buttons` (Show screen, Dismiss).
- `/Users/david/.local/share/gx/tickets/gx/deliver-primitive/mocks/06-not-sent-card.html`, state `web-send-failed`, width 640, fixture scenario: new (a chat whose plain send never starts a turn), parts: `card`, `reason`, `copy`, `buttons` (Show screen, Dismiss).
- `/Users/david/.local/share/gx/tickets/gx/deliver-primitive/mocks/06-not-sent-card.html`, state `web-unknown-command`, width 640, fixture scenario: new (a chat sending `/my-skill` that the fake answers with `Unknown command: /my-skill`), parts: `card`, `reason`, `copy`, `buttons` (Resend, Restart agent, Show screen, Dismiss).
- `/Users/david/.local/share/gx/tickets/gx/deliver-primitive/mocks/06-not-sent-card.html`, state `tui-not-sent`, width 640, fixture scenario: the same as `web-may-not-be-sent`, opened in the TUI, parts: `row`.

## Out of Scope

- The doctor's image-paste diagnostic: a diagnostic, not a prompt send.
- An interim two-line patch to the chat-slash path: the merge comes first.
- The headless runner and herdr, which never type into a terminal.
- A Resend button on the "may not have been sent" card: the command could run twice.
- A new restart verb: Restart agent is built on Stop and Resume.
- New pause kinds (ADR 0042): ralph-loop keeps `agent_prompt_stalled`.
- A Codex input-box reader or a Codex settle Enter: there is no evidence Codex swallows an Enter.
- Matching Claude's strip set automatically as Claude changes: the settle Enter is the guard.

## Further Notes

- Why one retry owner: stacked layers (nudge, settle Enter, launch resend) could press Enter 3 times 3 times and run a slash command twice.
- Claude strips at Enter and swallows that Enter only for text starting with `/` or `!`, typed or pasted. The character in the original failing send is unknown, because gx logs no typed bytes.
- Local commands record in the transcript only after they end, so the transcript is never the first landed signal.
- The resize settle has no log data; the 25 ms poll with a 1 second cap comes from the code (the read turns trusted on the child's next output).
- An open card on a session that exits is replaced by the exit, as for any Actionable. The one exception is the Unknown command card across the person's own Restart agent.
- Source plan: `docs/plans/deliver-primitive.md` in the `main` checkout.
