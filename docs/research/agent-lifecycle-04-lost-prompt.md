# gx:agent-lifecycle/04 - the initial skill prompt "lost" at launch

Question: why did `gx:cursor-support/01` never get its `gx-implement` prompt, and no
`launch-failed` / `prompt-nudged` event was logged?

## Verdict

The ticket's premise is wrong. The prompt was **not lost**. It reached Claude's input box and was
submitted. Claude rejected it: `Unknown command: /gx-implement`. A rejected slash command starts no
turn and writes no user message, so the transcript shows nothing. gx then treated "a slash command
that started no turn, with the agent still idle" as delivered, by design.

So it is neither `errPersonInput` (pty.go:749) nor a typed-before-ready drop. It is the **slash-command
single-send rule**, which cannot tell "ran and started no turn" from "rejected as unknown".

## Evidence (cursor-support/01 and /04)

- Runner was **herdr**: run log `paused` event has `"runner":"herdr"`, pane `w13F:p2`
  (`~/.local/share/gx/tickets/gx/cursor-support/run-log.jsonl`, line for 18:27:29). So the nativerunner
  path (pty.go:749) was not involved.
- Transcript `~/.claude/projects/-Users-david-dev-gx-cursor-support-item-01/71aa47f9-....jsonl`, first
  records (not user messages, so the ticket missed them):
  - line 3: `{"type":"system","subtype":"informational","content":"Unknown command: /gx-implement", "timestamp":"2026-10-30T15:48:51.961Z"}`
  - line 4: `"Args from unknown skill: gx:cursor-support/01"` at 15:48:51.962Z.
  - The only user message is the stall nudge at 16:27:20Z (line 6).
  - `iteration-started` is logged at 17:48:56+02:00 = 15:48:56Z, **after** the rejection. The prompt
    arrived about 5 s before gx logged the start.
- Ticket 04 of the same epic, launched 11 s later (17:49:07), has the identical pattern in
  `-Users-david-dev-gx-cursor-support-item-04/1ccb0023-....jsonl` (`Unknown command: /gx-implement`,
  nudge as first user message, `stall-nudged` at 18:27:20.9, paused zero-commit at 18:27:36). Two
  sessions in 11 s failed the same way: not a random keystroke drop.
- The retries at 18:28:38 / 18:28:57 (`b6f1c929...`, `4595c433...`) got a proper first user message
  `<command-name>/gx-implement</command-name>` with the skill body loaded from
  `/Users/david/.claude/skills/gx-implement`. Same prompt string, same worktrees. Only the moment differs.
- `~/.claude/skills/gx-implement` is a symlink (created 2026-10-30 16:04) to
  `/Users/david/dev/gx/main/skills/gx-implement`. Files inside that dir were rewritten at 17:51:29
  (LICENSE/README mtimes), three minutes after the launch. Something refreshed the skill files in that
  window. I could not find the writer in any log (gx.log and server.log do not cover Oct 30 17:4x).
- The nudge text is the generic `StallNudgeText` (ralphloop/servermerge.go:99), typed at
  servermerge.go:446 (`d.Runner.Prompt(s, StallNudgeText)`). The agent had no task, so it answered
  "no pending task" and stopped; zero commits then paused the ticket.

## Why gx saw success: every Runner.Prompt path

`startAndPrompt` (ralphloop/launch.go:326, called from ralphloop/iteration.go:80 and :1018) only
retries on `ErrNotDelivered` (launch.go:351). The prompt is `/gx-implement <addr>` (`skillPrompt`,
ralphloop/loop.go:191 -> harness/skillprefix.go:7), so every runner takes its slash branch:

| Runner | Slash branch | Returns nil when |
|---|---|---|
| herdrrunner | `promptNudged` herdrrunner.go:156 -> `promptSlash` :258 | seq moved (:270), or **no turn and agent not working after N (:276)** |
| nativerunner Pty | `PromptNudged` pty.go:741 -> `promptSlash` :773 | `started` true, or **no turn and claude not busy (pty.go:788)** |
| nativerunner Headless | `Prompt` headless.go:371 -> `awaitSlash` | not traced in depth; Headless sends JSON user messages, so a rejected skill would surface as an error event, not a silent idle. Worth a test. |
| server `deliverSlash` | server/firstprompt.go:~285 | "A step that fails after typing is only logged: the prompt was delivered." Same blind spot for server-launched sessions. |
| runnerfake / servertest ptyRunner | test doubles | n/a |

None of the paths looks at the pane/transcript for a rejection. Claude's reply to an unknown skill is
a system line on screen, which would be visible via `Screen()` (herdr: `ReadPaneRecent`).

The reason the comment gives for the single send is right: "most commands start no turn, so a retype
would run them twice" (herdrrunner.go:254, pty.go:768). A `/gx-implement` launch is not "most
commands": it must start a turn, unlike `/rename` or `/model`.

### The other two suspects, ruled out for this incident

- `errPersonInput` swallow (pty.go:749, pty_personinput.go:17,83): not on this path (herdr runner).
  It is still a latent hole: it returns nil after gx typed and a person then typed, without checking a
  turn started. Acceptable for a person at the keyboard, but it should report "delivery unconfirmed"
  to the caller rather than success. Low priority.
- herdr `errTurnPassedUnseen` (herdrrunner.go:187/205/214): returns nil when a poll timeout coincides
  with `StateChangeSeq > turn`. Only on the non-slash branch. A seq bump from any state change (for
  example idle -> blocked -> idle during startup) would pass for "turn ran". Also an unconfirmed
  success; same fix covers it.
- herdr `agent.StateChangeSeq <= turn` (herdrrunner.go:196) does return `ErrNotDelivered`, correctly.

## Why was the skill unknown?

Not proven. Two candidates, both consistent with the evidence:

1. Claude 2.1.314 registers skills asynchronously; the first idle comes before the registry is
   loaded. Against this: the same race would then hit most launches, and I saw no other lost launch in
   this epic's run log (not checked beyond it).
2. The skill directory behind the symlink was being rewritten at that moment (the 17:51:29 mtimes show
   a rewrite right after). A window where `SKILL.md` was missing at Claude's scan fits "two launches
   at 17:48:56 and 17:49:07 failed, retries 40 min later worked". The `9152a275` commit (18:27:45,
   "install dev skills through owned directory links") is the same area and would remove copy
   windows, but I cannot tell if it fixes this one.

Either way the right fix does not depend on the answer: the launch must verify the skill ran.

## Shared with gx:agent-lifecycle/03

- Same function: `PromptWithNudge` -> `clearWhole` -> `ErrInputNotEmpty` (agentrunner/claudeui/nudge.go:134)
  and `awaitEmptyInput` (nativerunner/pty_personinput.go:98). 03's "submission never reached the pane:
  input not empty after clearing" is a **different** failure from 04: it is a loud `ErrNotDelivered`,
  retried once by `startAndPrompt`, then `agent_prompt_stalled`. 04 is a silent success.
- Both end in the same bad place: zero-commit pause plus an investigate ticket whose only result is
  Retry. A single "launch verified" step (below) and "auto-retry once on ErrInputNotEmpty" (03) belong
  in the same change to `startAndPrompt`.
- 03's suggestion to log the caller of the `i` press helps here too: 04 shows no log of what the pane
  looked like at launch.

## Recommended fix

1. **Confirm the turn started for a skill launch.** In `startAndPrompt` (or inside each runner's slash
   branch) treat a launch prompt as a "must start a turn" prompt. Add a runner option (for example
   `PromptOptions.ExpectTurn`) so the slash branch uses the normal `PromptWithNudge`-style wait for a
   turn instead of "idle after N is fine". Keep the single-send rule for `/compact`-like commands
   that really start no turn.
2. **Detect the rejection explicitly.** After the send, read the pane (herdr `ReadPaneRecent`, pty
   `Screen()`) and match `Unknown command: /...`. Also confirm via the transcript: the session's first
   user record must be a `<command-name>/gx-implement` message (`ralphloop/deps.go:374`
   `transcriptPath`). Neither signal alone is enough (a user message can arrive late; the pane scrolls).
3. **On a miss, retry, then fail loudly.** Resend the same prompt in the same session after a short
   delay (a registry that loads late works on the second try, and a rejected command typed twice
   runs zero times, so the double-run worry does not apply). Emit `prompt-nudged` with decision
   `skill-unknown` (logPromptNudged, iteration.go:92). After the cap return `ErrNotDelivered`, so the
   existing fresh-session retry and `launch-failed` / `agent_prompt_stalled` pause (launch.go:355-360)
   fire. A fresh session is cheap here.
4. **Stall nudge on a session with no user message.** In `nudgeIfStalled` (servermerge.go:430) check
   the transcript: if the session has no user record at all, resend the original skill prompt
   (`skillPrompt(...)`, same string as iteration.go:79) instead of `StallNudgeText`, and log
   `stall-nudged` with that text. The generic nudge is wrong for a session that never got its task.
   This is the backstop; (1)-(3) should make it rare.
5. **Server path.** `deliverSlash` (server/firstprompt.go) needs the same post-send check, or a
   rejected first prompt for a server session stays silent too.
6. Tests: a runnerfake/agentfake whose skill registry is empty for the first N seconds; assert
   `prompt-nudged`, then success; and an always-unknown skill that ends in `agent_prompt_stalled`.

## Open design questions for the person

- Is "a launch prompt must start a turn" the right rule, or should unknown-skill be a hard pause
  immediately (the skill truly does not exist), with retry only for a short grace window? I recommend
  retry for about 30 s, then pause.
- Should the delivery check read the transcript (accurate, depends on Claude's file layout and the
  Codex equivalent) or the pane (cheap, depends on Claude's wording of the error)? I recommend both:
  transcript as the success proof, pane match only to fail fast.
- Should the skills refresh (17:51 rewrite) be made atomic (build in a temp dir, rename), so Claude
  never sees a half-written skill directory? This needs a person to confirm the cause first by logging
  skill refresh times.
