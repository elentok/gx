# Web UI: answer parked tickets from a phone

Source: the map `gx:webui-unpark` and its tickets 01 to 19. Where tickets disagree, the later one
wins: 06 reverses 03 and amends 02, 04 and 05; 08 amends 07; 09 replaces the question read of 05
and wins on desktop layout; 14 settles the Host check; 15 amends 13; 16 settles plain unpark on a
gone agent; 17 gives the web Unpark a state token; 18 settles herdr and Codex. The runner facts are in
the research report named in ticket 07.

## Problem Statement

An agent asks a question and the ticket parks as `needs-answer`. The person is away from the desk.
The web UI shows the Queue read-only, so they can see the ticket is Parked but cannot answer. The
Telegram and Slack message says only a generic reason, so they do not know what was asked. They must
go to a terminal, edit the ticket in `$EDITOR` and unpark it.

Three more problems sit behind this:

- Today an agent asks by writing `## Needs Answer`, ending the turn and exiting. It frees its
  worktree, pane and slot, and the person's answer starts a cold run from a thin handoff.
- A permission prompt on a running agent blocks it with no way to approve it remotely.
- The web listener has no write route and no Host check, so adding one is unsafe until the
  listener is hardened.
- The chat message has no web hint (ADR 0022 stays unchanged), and no message is sent when an agent
  blocks on a permission prompt. A permission-blocked agent notifies nobody. The person finds it
  only by opening the Queue.

## Solution

A person opens the web UI on a phone, sees the parked ticket with the question's summary line, taps
it and reads the agent's question. They answer with a yes/no button, an option button or free text,
plus an optional note. The reply goes to the agent, which is still alive and idle in its pane. A
delivered reply sets the ticket to `claimed` at once. If the agent is gone, the answer is saved, the
ticket shows as "answered" with its queue position, and the next normal claim restarts it.

The same page approves a permission prompt on a live blocked agent ("Allow once" or "Deny") and
unparks every other `needs-answer` park (zero-commit, base-ambiguity and the rest).

Agents ask through a new verb, `gx tickets ask`. Each ticket gets an append-only event log for park
rounds, replies and permission decisions. The Telegram and Slack park message shows the question's
summary line. It has no link for now.

The layout is phone-first: one column, big tap targets. Desktop is the same column, wider.

## User Stories

1. As the person running gx, I want the Queue row of a parked ticket to show the question's summary line, so that I know what is asked without opening it.
2. As the person running gx, I want an "Answer →" link on an answerable row, so that I reach the ticket page in one tap.
3. As the person running gx, I want `needs-repair` rows to show the reason and a TUI hint with no link, so that I do not open a page that cannot help.
4. As the person running gx, I want a ticket page at `#/t/<address>`, so that I can bookmark it and use the browser back button.
5. As the person running gx, I want an unknown hash to show the Queue, so that a bad link never shows a blank page.
6. As the person running gx, I want the question shown as plain text under a "THE AGENT ASKS" label ("GX ASKS" when gx wrote the question), so that it never looks like a button.
7. As the person running gx, I want the agent's handoff collapsed under "Agent's notes", so that I can check context without clutter.
8. As the person running gx, I want big Yes and No buttons for a yes/no question, so that I can answer with a thumb.
9. As the person running gx, I want full-width option buttons with a one-line consequence each, so that I know what each choice does.
10. As the person running gx, I want a "Recommended" badge on the agent's suggested option and nothing preselected, so that I choose on purpose.
11. As the person running gx, I want a text area for a free-text question, so that I can write the answer.
12. As the person running gx, I want an optional note on every form, so that I can add detail to a button answer.
13. As the person running gx, I want a sticky "Send answer" button, so that it stays reachable on a long page.
14. As the person running gx, I want the buttons disabled while a request runs, so that I do not send twice.
15. As the person running gx, I want my typed note kept when a request is refused, so that I do not retype it.
16. As the person running gx, I want a stale answer refused with a clear reason and the current question shown, so that I never answer a question I have not read.
17. As the person running gx, I want my note kept when the page swaps in the new question, so that I can reuse it.
18. As the person running gx, I want the reply to reach the live agent as a prompt, so that it continues with its context, worktree and branch.
19. As the person running gx, I want the result to say "delivered" when the agent got the reply, so that I know it is working again.
20. As the person running gx, I want an "agent is working" chip after I reply, so that I see something happened.
21. As the person running gx, I want the next question to appear on the same page when the agent asks again, so that a conversation is one flow.
22. As the person running gx, I want a reply refused with "agent not ready" while the agent is blocked on a prompt, so that I approve the prompt first.
23. As the person running gx, I want an answer to a gone agent accepted and logged, so that I never lose what I typed.
24. As the person running gx, I want the ticket to show "answered" with the queue position only when the agent is gone (a delivered reply goes straight to `claimed`), so that I know it waits for a slot.
25. As the person running gx, I want the result to say "queued" in that case, so that I know the agent was not reached yet.
26. As the person running gx, I want the next claim to try `claude --resume` first and then a cold start, so that the agent keeps its context when it can.
27. As the person running gx, I want the claim's run-log entry to say which path it took, so that I can diagnose it.
28. As the person running gx, I want no undo after sending, so that the agent never sees a reply that was withdrawn.
29. As the person running gx, I want a permission card on a ticket whose agent is blocked, so that I can approve from the phone.
30. As the person running gx, I want the card to show the tool, a one-line description and the input in a code box, so that I see what I allow.
31. As the person running gx, I want only "Allow once" and "Deny", so that I never grant standing permission from a phone.
32. As the person running gx, I want a note on Deny, so that I can tell the agent what to do instead.
33. As the person running gx, I want a decision refused when the prompt changed after I looked, so that I never approve something I did not see.
34. As the person running gx, I want unsupported dialogs (plan approval, `AskUserQuestion`, unknown) to say "Approve it in the TUI" with no buttons, so that a web tap cannot pick a wrong option.
35. As the person running gx, I want the permission card to show one dialog at a time in call order, so that I always decide on the call that is on screen.
36. As the person running gx, I want a "needs approval" row in the Queue for a claimed ticket with a pending prompt, so that I find the card without a bookmark.
37. As the person running gx, I want an "Unpark" action for zero-commit, stranded-commits and background-task-gate parks, so that I can clear them from the phone. Unpark reopens the ticket and the next claim starts fresh.
38. As the person running gx, I want the result of a zero-commit unpark to say "adopted as done" or "reopened", so that I know what happened.
39. As the person running gx, I want the ambiguous-base form to list the competing blockers' branches and set the base, so that I pick the right one.
40. As the person running gx, I want a late agent finish to win over my answer (outcome "adopted"), so that nothing is written onto a ticket that already finished.
41. As the person running gx, I want the page to refetch on every event for its ticket, so that it never goes stale.
42. As the person running gx, I want the earlier park rounds collapsed at the bottom, so that I can read the history.
43. As the person running gx, I want a read-only last round when the ticket is no longer waiting, so that the page still explains what happened.
44. As the person running gx, I want "Ticket not found" on a 404 and a loading skeleton, so that the page states are clear.
45. As the person running gx, I want the Offline strip to keep the last page on screen, so that a network blip does not blank it.
46. As the person running gx, I want the Telegram and Slack park message to show the question's summary line, so that I know what is asked.
47. As the person running gx, I want other park messages to stay as they are, so that nothing else changes.
48. As the person running gx, I want a Host check on every route, so that DNS rebinding cannot reach the listener.
49. As the person running gx, I want an Origin check and a JSON content type on every write route, so that another site cannot post for me.
50. As the person running gx, I want to read a warning about plain HTTP and cookies in the docs, so that I know a sniffed cookie now controls a full-permission agent.
51. As an agent, I want `gx tickets ask` with a question, an optional block and a handoff, so that I can ask and end my turn.
52. As an agent, I want the ask refused with a reason when the body is invalid or a question is already open, so that I fix it before I exit.
53. As an agent, I want the person's reply to arrive as my next message (live Claude on a native runner) or as the start of a fresh run from my handoff (herdr and Codex), so that I continue with the reply in hand.
54. As an agent, I want a clear instruction to retry `ask` after a minute when the server is down, so that I do not end my turn into a stall.
55. As an agent, I want a single nudge if I go idle with no question and no finished report, so that a forgotten `ask` does not stall the ticket.
56. As a person using the CLI, I want `gx tickets answer` to reply to a ticket, so that scripts and the terminal have the same path.
57. As a person using the CLI or TUI, I want plain `unpark` on a live waiting self-reported park to refuse and name `answer`, so that I do not strand the agent.
57a. As a person using the CLI or TUI, I want plain `unpark` on a self-reported park whose agent is gone to discard the question and reopen the ticket, so that I can clear it without inventing an answer.
57b. As the person running gx, I want the web Unpark refused as stale when a newer park round exists, so that a stale tab never unparks a round I have not seen. This holds for a self-reported park whose agent is gone too: the web shows Unpark there only when the agent is gone, with the same state token, and writes `park.closed`.
57c. As the person running gx, I want a herdr-hosted agent's blocked prompt to say "Approve it in the TUI" with no buttons, so that I never approve a request I cannot verify.
58. As a person using the CLI, I want `gx tickets log` to print raw events and `gx tickets show` to print the latest round in full, so that I can read the conversation.
59. As the person running gx, I want waiting agents to release their slot after a grace period, so that a long wait does not block the queue.
60. As the person running gx, I want a cap on released agents, so that live-but-idle agents cannot pile up.
61. As the person running gx, I want queue verdicts "waiting on answer", "waiting (slot released)" and "waiting (slot held, release cap reached)", so that I see why a ticket waits.
62. As the person running gx, I want an answered agent to take its slot back at once, so that its reply is never delayed.
63. As the person running gx, I want waiting state to survive a server restart, so that a restart loses no question.
64. As the person running gx, I want the budget hard limit to reach released agents too, so that cost control has no gap.
65. As an owner of existing tickets, I want a one-shot migration of live `## Needs Answer` and `## Handoff` sections into the log, so that open questions still show on the web.
66. As a developer, I want a test that fails when the system prompt text and the `gx-local-tracker.md` text differ, so that agents get one wording.
67. As a developer, I want a golden-file test for pty dialogs and a `gx doctor` check, so that a Claude UI change fails loudly.

## Implementation Decisions

### Security first

These ship before any write route and have their own tickets.

- **Host check.** Applies to every route on the web listener, GET included. It accepts any
  IP-literal Host (IPv4 or IPv6, any port) and `localhost`. Any other name must be in a new
  `server.web.allowed-hosts` list. A failure is HTTP 403 with a short plain-text reason.
  - Why IP literals pass: DNS rebinding needs an attacker-controlled name in Host, and a browser
    cannot send an IP-literal Host from an attacker's page to a different IP. This keeps LAN,
    Tailscale-IP and `0.0.0.0` binds working.
  - **Upgrade note (docs and changelog).** Users who open the UI by a MagicDNS or other hostname
    must add that name to `server.web.allowed-hosts` before upgrading, or the UI answers 403. IP
    use is unaffected.
  - Test table, each row one request against the web listener: `127.0.0.1:port`, `[::1]:port`,
    `localhost:port`, a LAN IP, a Tailscale IP, any of them on a `0.0.0.0` bind (all pass); an
    unlisted name `evil.example` (403); a listed MagicDNS name (pass); a name with a trailing dot
    or different case; a missing Host.
- **Origin and JSON check.** Every POST needs an `Origin` whose host equals `Host` (a missing Origin
  is refused) and `Content-Type: application/json`. Failures are 403 with a short reason.
- **Web mux allowlist.** The mux stays GET-only today. It gains an explicit, narrow list of POST
  routes: the answer route, the permission route, and the unpark route. The unpark route gains a
  `base` input and a state token (see "Unpark route for the web"). The web mux requires the token;
  the CLI keeps calling the route without one.
- **Risk to state in the docs.** The listener is plain HTTP on Tailscale or LAN (TLS, token expiry
  and per-device sessions stay in `gx:webui-security`). A sniffed cookie now grants instructions to
  a full-permission agent and permission approvals.

### Ticket log (amends ADR 0026)

- Each ticket gets an append-only `<NN-slug>.jsonl` beside its `.md`. A line is `{seq, ts, type, ...}`.
  The loader ignores it. The markdown stays truth for everything it holds (ADR 0026). The log is
  the truth for the conversation, so ADR 0026 gets an amendment saying so.
- One record is one `write()` of the whole line. Readers and `validate` skip a torn last line. A
  malformed line in the middle is a `validate` error. A message text is capped at 4,000 characters.
- The server is the only writer for `ask`, `answer` and web events. It appends under the ticket
  lock and reads the last `seq` under that lock. The repair verbs (`reset`, `unpark`) may run
  without a server and write `park.closed` directly under the same lock.
- `applyMove` (move, close, sweep) carries the log along and renames it under the old ticket's lock.
- The per-epic `run-log.jsonl` stays separate. It records what happened to a ticket. The ticket log
  records the conversation.
- Event types:
  - `park.question` is `{by: agent|gx, session?, park_kind, summary, text, block?, handoff?}`.
    `summary` is at most 120 characters. `handoff` is required when `by` is `agent`.
  - `park.answer` is `{question, source: web|tui|cli, choice?, text?, note?}`. For a free-text
    question the `text` is the answer and the `note` is empty. For a button answer the `choice` is
    the answer and the `note` is the optional extra detail. A web unpark of a gx-authored
    park logs `choice: unpark`, plus the chosen base for ambiguous-base.
  - `park.permission` is `{decision: allow|deny, source, note?, shown}`. `shown` holds what the person
    saw: request ID, tool, input or dialog text, description, a `truncated` flag. It is capped at
    4,000 characters. Only decisions that reached the agent are logged.
  - `park.closed` is `{question, reason}`. Every path that leaves `needs-answer` without a person's
    answer writes it under the ticket lock, before the status write. Reasons include `reset`,
    `adopted`, `budget-killed`, `relaunch` and a new reason for an unpark of a self-reported
    park whose agent is gone, from the CLI, TUI or web (implementer picks the name, for example
    `unparked`).
- A park round is a `park.question` plus the `park.answer` or `park.closed` that ends it. The open
  question is the latest `park.question` with neither.
- `## Needs Answer` and `## Handoff` are no longer written to the `.md`. `## Comments` and
  `## Needs Repair` stay. The frontmatter keeps `status`, `park_kind` and `iteration_status`.
- Invariant: `status: needs-answer` means the latest question is open. Only `gx tickets validate`
  and the write paths that set `needs-answer` check it. `schema.Validate` does not.
- Every park writes a question first. gx writes a `by: gx` question for every park kind except
  self-reported: zero-commit, ambiguous-base, stranded-commits and background-task-gate. The
  server never parks a blocked pane, so blocked-pane is not a park kind here: a blocked iteration
  stays running.
- **gx-authored questions.** gx writes a fixed summary per kind (also the `text`; no block, no
  handoff) with `by: gx`. The UI labels them "GX ASKS". The wording:
  - zero-commit: "The agent finished with no commits. Adopt it as done, or reopen the ticket?"
  - stranded-commits: "The agent stopped with commits that never landed. Reopen the ticket to retry?"
  - background-task-gate: "The agent ended its turn with a background task still running. Reopen the ticket?"
  - ambiguous-base: "Several blockers have different branches. Pick the base branch." (the
    implementer may refine this text; the three above are fixed)
  "Unpark" for these kinds means reopen. The next claim starts fresh. The exception is the
  zero-commit unpark with a live agent (see "Unpark route for the web").
- **Migration.** A one-shot `gx tickets migrate` step turns a live `## Needs Answer` plus
  `## Handoff` into one `park.question`, and adds a `park.answer` when the ticket was answered by
  hand and is still open. There is no dual read. Old retired Comments entries stay as history. The
  map calls the step `gx tickets migrate`; the implementer picks its final name and placement.
  - **Upgrade note (docs and changelog).** The person who upgrades runs `migrate`, or gx runs it
    once at server start. The implementer picks the trigger. Until it has run, an old ticket
    without a log shows no open question on the web, and the web cannot answer it. The docs must
    say who runs it and when.
- Agents see the log through `gx tickets show` (a generated "Park rounds" block, earlier rounds on
  one line each, the latest in full) and `gx tickets log <addr> [--type ...]`, which prints raw
  events. `show` never prints raw events.

### Question block

- An optional fenced `gx-question` YAML block in the question text. Types are `yes-no`, `options`
  and `text`. No block means free text, so old tickets need no change.
- `yes-no` has fixed labels "Yes" and "No" and optional `yes:` and `no:` one-line consequences.
  `options` has 2 to 6 entries with `id`, `label` and an optional `consequence`. Every type allows a
  free-text note.
- Optional `recommended: <id>` (or `yes` / `no`), one value. The UI shows a "Recommended" badge and
  preselects nothing.
- The summary line is plain text, no markdown, at most 120 characters. `gx tickets validate` warns
  on a longer one. Displays show it unclipped.
- Validation happens in `gx tickets ask`: bad YAML, an unknown type, duplicate ids or a
  `recommended` outside the options fail the verb with the reason. The web read treats a bad block
  as free text.
- The format is defined once in the announce-and-stop section of `gx-local-tracker.md`.

### Verbs

- `gx tickets ask <addr> --body -`. The body has `## Question` (summary line first, optional
  `gx-question` block) and a non-empty `## Handoff`. The handoff names the files touched, the
  decisions made and why, and what the agent does with each answer. The verb validates the body,
  appends one `park.question`, sets `iteration_status: needs-answer` under the ticket lock and
  returns at once. It refuses and writes nothing on any failure, including when an open question
  exists. It works for every ticket type.
- `gx tickets answer <addr> [--choice x] --body -` appends a `park.answer` and delivers it. The
  body is the note, or the text for a free-text question. It refuses on a park that is not self-reported and names `unpark`.
- `ask` and `answer` call the server and refuse with `server-not-running` when it is down (ADR 0030:
  orchestration writes go through the server).
- `gx server tickets answer` stays as the server mirror with the state token.
- `gx tickets set --status open` and `--iteration-status needs-answer` refuse when no matching open
  question exists, and name the verb to use.
- Plain `unpark` (CLI and TUI) on a live waiting `self-reported` park refuses and names
  `gx tickets answer`. It does not kill the agent or send a prompt.
- Plain `unpark` on a `self-reported` park whose agent is gone is allowed. The implementer chooses
  how "agent is gone" is detected. An agent that dies between the check and `Runner.Prompt` makes
  an answer return `queued`, not a refusal. It writes `park.closed`
  with the new reason, sets the ticket `open`, and the next normal claim cold-starts from the
  handoff. The agent never reads a fake answer, and no `park.answer` is logged. The TUI unpark
  action, `gx tickets unpark` and `gx server tickets unpark` share this rule and one refusal
  reason for the live case. The refusal string keeps the existing reason-not-delivered spelling
  (ADR 0028); the implementer picks nothing new there.
- ADR 0028: every new route needs a CLI verb in the same change; the route and CLI parity test
  covers them. The ADR gets an amendment listing the new verbs.

### TUI Tickets-tab preview

- The `.md` no longer holds `## Needs Answer` or `## Handoff`, so the preview of a parked
  `needs-answer` ticket reads the ticket log. It appends the same "Park rounds" block that
  `gx tickets show` prints (earlier rounds one line each, the latest in full). Both use one
  renderer; there is no second formatter.
- The orange highlight (`highlightParkSection`, `needsAnswerHeading` in `ui/tickets/preview.go`)
  moves to the latest round in that block. Earlier rounds stay plain.
- The hint in `ui/tickets/park_reason.go` changes from "Press m to answer." to "Answer in the web
  UI or run `gx tickets answer <addr>`." The `m` key and `ui/tickets/answer.go` are removed.
- `needs-repair` previews are unchanged: `## Needs Repair` stays in the `.md`.

### Live agent

- After `ask`, the agent ends its turn and waits idle. It keeps its worktree, pane and slot. The
  server parks the ticket (`needs-answer`, `park_kind: self-reported`) and skips worktree cleanup.
  This reverses the old announce-and-stop.
- A reply is a `park.answer` delivered with `Runner.Prompt`, in a fixed frame ("Reply from <person>
  to your question <seq>: ..."). The frame prints the choice first and the note under it. For a
  free-text question it prints the text and no note. The ticket goes back to `claimed`. Wording for
  the frame beyond that is the implementer's choice.
- Live waiting, the nudge and slot release apply only to Claude on a native runner (pty and
  headless). For herdr-hosted Claude and Codex agents the reply never reaches a herdr pane: herdr
  is deprecated and nothing new is built for it. On `ask`, gx closes the idle pane and frees its
  slot. The ticket parks as `needs-answer`. The answer reopens the ticket, and the next claim
  cold-starts from the handoff. herdr-hosted Claude also gets the tool ban and GX system prompt
  (see "Skills and launch edits"); Codex gets the skill wording only. Follow-up gx:follow-ups/14
  covers a live flow for them. Follow-up `gx:follow-ups/17` covers multi-harness reply and
  approval support.
- **Gone agent (crash, kill, budget kill, lost session).** `POST /v1/tickets/answer` accepts the
  answer, logs `park.answer`, reopens the ticket as `answered` and launches nothing. The next normal
  claim goes through the scheduler (ADR 0021 permit, ADR 0032 slot caps, FIFO) and tries
  `--resume <last session id>` in a worktree recreated at the same path on the kept branch, then a
  cold start from the latest round and its handoff. herdr-hosted agents and Codex always
  cold-start. A claim after a plain unpark (no answer) cold-starts. The claim's
  run-log entry gains a `path` field (`resumed` or `cold-start`).
  - `--resume` works from a recreated directory in claude 2.1.304 (tested). A pending approval does
    not survive a resume. A plain `--resume <last id>` keeps the appended system prompt, and a
    resumed session keeps the old text. `--resume` with `--session-id` fails unless
    `--fork-session` is also passed. The resume mode of `Pty.Start` therefore uses `--resume <last
    id>` alone and keeps the old session id. The implementer adds that mode. Re-passing
    `--append-system-prompt` is harmless and gives the current text if wanted.
- **Answer result.** `delivered` (the live agent got the reply) or `queued` (the agent is gone).
  `agent-gone` is not a refusal for answers. `agent-not-ready` is (the agent is blocked, approve its
  prompt first). Live delivery checks the budget first, as the nudge route does; a refusal for a hit
  hard limit has a reason name the implementer picks. That refusal logs no `park.answer`, so the
  person can retry. The check applies to live delivery only. A queued answer (agent gone) is
  accepted without it, and the next claim has its own slot caps.
- **Gone-agent detection.** How the server decides "agent is gone" is the implementer's choice. An
  agent that dies between that check and `Runner.Prompt` makes the answer return `queued`, with
  the `park.answer` already logged.
- **Slots (amends ADR 0032).** A waiting agent counts toward the slots for a grace period
  (default 10 minutes). After that it releases its slot and stays alive. At most 5 agents (default
  5, across projects) may be released at once. The 6th keeps its slot. Nothing is terminated for
  this. An answered agent takes its slot back, over the limit if needed, and new claims wait until
  the count is under the limit. Those blocked claims get the existing at-limit queue verdict if one
  fits; otherwise the implementer picks the name. Config key names are the implementer's choice (for example
  `server.waiting-slot-grace`). Queue verdicts are "waiting on answer", "waiting (slot released)"
  and "waiting (slot held, release cap reached)". A claim never launches over a live waiting agent.
- **Run state.** `waiting_since` and `slot_released` live in the server's persisted run record, not
  in the ticket. After a restart each agent keeps its saved state and the cap is recomputed.
  `reclaim` restores a waiting run as waiting and settles it only after an answer is delivered.
  Released agents stay in the registry, flagged as not counting toward slots, so the budget hard
  limit still reaches them. `parkTicket` must stop dropping the run record of a self-reported park.
- **Stalled agent.** An idle agent with no open question and no finished iteration gets one fixed
  nudge after 2 minutes (delay is a config key, name the implementer's): "If you are waiting for a
  person's reply, run `gx tickets ask` now. Otherwise continue." If the condition still holds, the
  existing stalled path applies. The nudge goes to the run log. After an answer is delivered, the
  timer starts when `Runner.Prompt` returns. This adds up to 2 minutes before the land path for an
  idle agent with commits and no `finished` report. No Haiku checkup agent.
- **Ending the wait.** The server wait must not treat an idle agent with an open question as
  finished. How each runner tells "idle, waiting" from "idle, finished" is the implementer's choice;
  the ticket's open question is the signal on both.
- An agent whose `ask` fails with `server-not-running` retries after a minute and does not end its
  turn.

### Answer and detail routes

- `GET /v1/tickets/detail?address=` (replaces the earlier `question` route). It returns title,
  status, `park_kind`, `answered`, the park rounds (earlier ones as one line each, the latest in
  full with question, block, handoff and answer), the pending permission card, the chat for the open
  round, the state token and the list of applicable actions (`answer`, `approve`, `unpark`). The
  server decides the actions; the UI only renders them. Field names are the implementer's choice.
- `POST /v1/tickets/answer` with the address, the state token, the choice (or the text for a
  free-text question) and the optional note. The state
  token covers the latest question's `seq` and `status`, recomputed under the ticket write lock; its
  encoding is the implementer's choice. Validation: the choice exists in the block,
  a `text` answer is non-empty, the note is at most 4,000 characters.
- Refusals are HTTP 200 `{refused: true, reason, message}`. Reasons: `stale` (carries the current
  question), `not-parked`, `not-answerable`, `bad-choice`, `empty-answer`, `too-long`,
  `agent-not-ready`, plus the budget reason above. Success is `{outcome: ...}`: `delivered` or
  `queued`. The server runs `adoptFirst` before writing, so a race with a late finish returns
  `{outcome: adopted}` and writes no answer; the UI says the agent had already finished.
- `answer` on a non-self-reported park refuses with `not-answerable` and a message naming the unpark
  action. `needs-repair` is `not-answerable` for the web.
- **Answered flag.** Derived, never stored: `status` is `open` and the ticket's latest question has
  an answer. It exists only on the gone-agent path: a delivered reply goes straight to `claimed`.
  It clears when gx claims the ticket. CLI and TUI answers count too. The snapshot's
  parked entries gain `summary`, `answerable` and `answered`.
- **Permission route.** The existing `POST /v1/agents/answer` joins the allowlist.
  - headless: `Status` gains `Pending{ID, Tool, Input, Description}` and `Answer` gains `RequestID`
    (the wire `request_id`). `Answer` refuses a request ID that is not the open one. The runner
    saves each answered `request_id` in its meta file so a replay of `out.jsonl` skips it.
  - pty: the runner reads the screen through the holder and shows the dialog text as is. Web
    approval works only when the header is `Bash command`, `Edit file` or `Create file` and option 1
    reads exactly "Yes". The request ID is a hash of the header plus body, without the cwd line, tip
    line and footer. Verified on claude 2.1.304 (spike): dialogs come one at a time in call order;
    the stable markers are a title line, `Do you want to ...?`, a numbered list with `1. Yes` first,
    and a last line `Esc to cancel`; the option count varies (Bash 4, Write, Edit and Read 3). The
    transcript does not hold the pending `tool_use` while a dialog is open, so the screen is the
    only source. Before sending "Yes", the server re-reads the screen and refuses if the dialog
    differs from what the person saw. If the screen parser proves too fragile, pty shows
    "permission prompt" with no detail and approval stays in the terminal.
  - The TUI and CLI may omit the request ID and keep today's behaviour.
  - Not approvable from the web: plan approval, `AskUserQuestion`, and any unknown dialog (on
    headless, by tool name). These show "Approve it in the TUI" with no buttons.
  - herdr-hosted agents get no approval card at all. The ticket page and the "needs approval" Queue
    row say "Approve it in the TUI" with no buttons, because herdr's `Answer` has no request-id
    guard and a stale approval would grant a call the person never saw.
  - Refusals: `not-blocked`, `request-changed`, `not-approvable`, `agent-gone`, `not-delivered`.
    `agent-gone` is a refusal here because a lost prompt cannot be resumed.
  - "Allow once" and "Deny" cover every tool, `Bash` included. The note is sent only on Deny, capped
    at 4,000 characters. Deny on pty uses Esc on the dialog on screen. Dialogs are sequential, so
    the next dialog, if any, is a new card with a new request ID.
- **Unpark route for the web.** The existing `POST /v1/tickets/unpark` gains a `base` input and the
  same state token as the answer (the park round's `seq` and `status`; every non-self-reported park
  has a `by: gx` question, so the round is in the log). The web mux requires the token and refuses
  `stale` when it no longer matches; the check runs before any state change. The CLI and
  `gx server tickets unpark` keep working without a token. The web offers Unpark on every
  non-self-reported park. On a self-reported park it offers Unpark only when the agent is gone,
  with the same state token, and it writes `park.closed` (ticket 16). With a live agent the page
  offers the answer only; a gone-agent answer is `queued`. For ambiguous-base the form lists the competing blockers' branches and `base` must be one of them.
  For zero-commit the result says "adopted as done" or "reopened". When a live agent still exists
  after a zero-commit finish, the unpark sends it a fixed prompt ("A person reviewed your finish
  with no commits and wants you to continue. Finish the work, or report `--iteration-status finished
  --commitless true`.") and reopens for a claim only when the agent is gone.
- **Blocked iteration entry.** The snapshot marks a `claimed` ticket that has a pending permission
  prompt. The Queue shows a "needs approval" row linking to its ticket page. No chat message is sent
  for a blocked agent (ADR 0022 and the mute gate stay unchanged).

### Web app

- React Router in hash mode (`createHashRouter`). Routes are `#/` (Queue) and
  `#/t/<project:epic/NN>`. The Queue row's "Answer →" is a plain link to the ticket route.
- The one global `/v1/events` stream stays. There is no per-ticket stream. The page refetches its
  detail on any event for its address, on load and after each POST. Events stay payloadless. There
  is no transcript streaming: after a reply the page shows the sent reply and an "agent is working"
  chip, and the next question appears when the agent runs `ask`.
- Page states: loading skeleton; "Ticket not found" on a 404; read-only last round when the ticket
  is no longer waiting; the existing Offline strip with the last page kept.
- After a POST there is no optimistic update. The page refetches and shows the result line. A
  refusal shows its message inline and keeps the typed note. `stale` swaps in the current question
  from the refusal and keeps the note.
- Page order on phone, one column: back link, badge, title; permission card (only for a blocked live
  iteration); the latest question as plain text, then the handoff collapsed under "Agent's notes";
  the chat thread for the round; the sticky answer area (buttons or text, optional note, "Send
  answer"); earlier rounds collapsed at the bottom. Desktop is the same column, wider (ticket 09
  wins over the side-by-side queue in the mocks and over ticket 15).
- The in-between state is called "answered" (amber). It exists only on the gone-agent path: a
  delivered reply goes straight to `claimed`. The page shows the answer sent, the queue
  position and no form.
- The button reads "Send answer". The system term stays `unpark`.
- Mocks: branch `prototype/webui-unpark-mocks` (the mocks page)
  (throwaway, for look only).

### Chat

- Only `self-reported` needs-answer parks change. The detail line is the question's summary line
  (max 120 characters) instead of the generic reason. The counts line and the `[gx] project/epic/NN`
  line stay.
- `ticketNeedsHumanText` gains the park kind, and the sink method `TicketNeedsHuman` gains it. Telegram
  and Slack both go through that function.
- ADR 0022 is unchanged: one event per park from one site, no second message, no new event kind, no
  change to the mute gate. No link, and no hint that the web can answer. No replies from chat.

### Skills and launch edits

- Eight skill files change: gx-implement, gx-one-off, gx-evolve, gx-code-review, gx-tdd, gx-merge,
  `gx.md` (the overview of the park) and gx-investigate (its `## Needs Answer` and "Answer…" menu
  advice). They switch from `gx tickets section` to `gx tickets ask` and link to the wording and
  full syntax in `gx-local-tracker.md`.
- `gx-follow-up` is unchanged. Its unattended mode never asks.
- Wording in the announce-and-stop section of `gx-local-tracker.md`: "After `gx tickets ask`, end
  your turn. The person's reply reaches you as a new message, in this session or in a fresh one
  that starts from your handoff. If the reply leaves anything unclear, or you
  need more, run `gx tickets ask` again. Never end a turn with a question in plain text and no `ask`
  call. No one is told, and the ticket stalls." Plus: "Never use the AskUserQuestion tool. To ask a
  person, run `gx tickets ask`." Not added to gx-design.
- One shared launch constant bans `AskUserQuestion`, `EnterPlanMode` and `ExitPlanMode` on pty and
  headless, and on herdr-hosted Claude through the shared Claude launch path. The ban on the two
  plan tools is inferred, not tested. Codex gets neither the ban nor the system prompt, only the
  skill wording.
- The GX system prompt (GLOSSARY.md) is one Go constant in `harness`, passed with
  `--append-system-prompt` on every Claude launch and resume of a loop (ticket) agent, herdr-hosted
  Claude included. For herdr, `ask` then closes the pane, parks the ticket and the answer
  cold-starts. It holds the
  full `gx tickets ask` syntax. The same text sits in `gx-local-tracker.md`, and a test fails when
  the two copies differ.
- The ban and the system prompt apply to loop launches and their resume only. Agents-tab and
  gx-design sessions keep the tools.

### ADR amendments

- 0032: waiting agents release their slot after a grace period, with a release cap.
- 0026: the conversation lives in `<NN-slug>.jsonl`; markdown stays truth for what it holds.
- 0028: new verbs `ask`, `answer`, `log`; the parity test covers the new routes.
- Respected without change: 0021 (the answer call launches nothing, so the permit is untouched),
  0022, 0030 (`ask` and `answer` are server writes; `reset` and `unpark` stay direct repair verbs),
  0031 (both native runners get the feature; gaps only where a runner cannot do it: pty approves
  only three dialog kinds, headless is structured; herdr gets no approval card), 0032 (no launch
  outside the scheduler).

### Left to the implementer

Names and layouts the map did not fix: config key names (the defaults are fixed: 10 minutes
waiting-slot grace, release cap 5, stall-nudge delay 2 minutes; `allowed-hosts` is already named),
JSON field names of the detail read and snapshot additions, the state token encoding, the reply
frame wording, the hash function for pty request IDs, the budget refusal reason, the `base` field
name, the name of the `park.closed` reason for an unpark of a gone agent, the final name and
placement of the migrate step and its trigger, the queue verdict name for new claims blocked while
an answered agent retakes its slot (only if no existing verdict fits), how the server tells a
herdr-hosted agent from a native one, how "agent is gone" is detected for the unpark and answer
rules, CLI flag spellings beyond those above, and internal
markers for waiting state.

## Testing Decisions

A good test checks external behaviour: the HTTP result, the ticket files and log on disk, the text a
fake agent receives, the Queue snapshot, the rendered page. It does not check internal calls or
private state. The person approved these seams; each reuses an existing one.

1. **Server harness with a fake runner** (`server/servertest.Harness` plus `runnerfake.Runner`). Covers
   `ask`, answer delivery, `queued` for a gone agent, claim-time `--resume` then cold start and the
   `path` run-log field, refusals, slot release and cap, reclaim of a waiting run, the stalled
   nudge, adopt races, unpark with `base` and with a stale token, plain `unpark` refusal on a live
   waiting agent and plain `unpark` allowed on a gone one (`park.closed`, reopen, cold start), no
   approval card for a herdr-hosted agent, `ask` on herdr closing the pane and freeing the slot,
   budget gating (a refusal logs no `park.answer`; a queued answer skips the check), the fixed
   summaries of gx-authored parks, migration and the `park.closed` writers. Prior art: `server/agentverbs_test.go`, `nudge_test.go`,
   `finish_park_test.go`, `reclaim_*_test.go`, `budget_limits_test.go`.
2. **Web listener over HTTP** (`server/web_test.go` `startWeb`). Covers the Host check table
   (including `0.0.0.0` binds), the Origin and JSON checks, the POST allowlist, and the detail and
   answer routes end to end. Prior art: `server/web_test.go`, `web_devproxy_test.go`,
   `webbind_test.go`.
3. **Native runner tests** (`nativerunner`). pty golden files of captured dialogs for the three
   approvable kinds and the not-approvable ones, with the request ID hash and the re-read before the
   key; headless stream fixtures for `Pending`, `RequestID` mismatch and replay skipping answered
   requests. Prior art: `nativerunner/pty_test.go`, the headless event tests. A `gx doctor` check
   covers live Claude UI changes (`claudedoctor/headless.go`).
4. **Route and CLI parity test** (`cmd/server_parity_test.go`). Covers the new routes and verbs.
5. **Chat contract test** (the reflection test over `EventSink` from ADR 0022) plus the chat text
   function. Covers the summary line for self-reported parks and unchanged text for the other kinds.
6. **Web UI Vitest with the canned snapshot** (`webui/src/**/*.test.tsx`, `canned-snapshot.test.ts`).
   Covers routing, the page order, answer forms for each question type, refusal handling that keeps
   the note, the answered state and the permission card. Prior art: `QueueScreen.test.tsx`,
   `view.test.ts`, `connection.test.ts`.
7. **Ticket store and log tests** (`tickets` package). Torn last line, malformed middle line, `seq`
   under the lock, `applyMove` carrying the log on move, close and sweep.
8. **Skill text test.** The system prompt constant equals the `gx-local-tracker.md` text.

Seams for stories that no seam above names:

- Story 10 (Recommended badge, nothing preselected): seam 6. Render the canned detail with a
  `recommended` option and assert the badge shows, no button is selected, and "Send answer" sends
  nothing until the person picks one.
- Story 28 (no undo): seam 2 asserts the POST allowlist has no withdraw route (any such path
  answers 404 or 405), and seam 6 asserts the page has no undo control after a send.
- Story 41 (refetch on every event): seam 6, in the style of `connection.test.ts`. A fake
  `/v1/events` stream emits an event for the page's address and the test asserts one detail
  refetch; an event for another address causes none.
- Story 50 (docs warning): a Go test over the web UI docs file (path picked by the implementer,
  same style as seam 8) asserts the plain-HTTP and cookie warning and the hostname upgrade note
  are present.

## Out of Scope

- `needs-repair` answering from the web (its own effort).
- Replying to the chat message from Telegram or Slack.
- TLS, token expiry and per-device sessions (`gx:webui-security`).
- A web UI link in the park message (needs `server.web.public-url` and a `chatmarkup` link helper).
- Investigate tickets that use the ask flow to unstick tickets (belongs with `needs-repair`; filed as
  a follow-up).
- A grace period or Haiku checkup agent for stalled agents. (The 10-minute waiting-slot grace is a
  separate thing and is in scope.)
- New herdr and Codex support, including a live flow for them (gx:follow-ups/14). herdr and Codex
  agents get only what the shared launch path and the skill wording already give them.
- Multi-harness reply and approval support. Filed as follow-up `gx:follow-ups/17`.
- Banning `AskUserQuestion` in Agents-tab sessions.
- A TUI answer action. The `$EDITOR` heading flow is removed with no replacement; the owner decides
  after trying the web form.
- A Telegram or Slack message when an agent blocks on a permission prompt.
- "Always allow" on permission prompts, undo of a sent answer, and transcript streaming to the page.
- The ask-back chat session (reversed by ticket 06).

## Further Notes

- Runner facts, verified by the spike (claude 2.1.304, report `docs/research/webui-unpark-spike-results.md`):
  - Plain `--resume` keeps the appended system prompt.
  - `--resume` plus `--session-id` fails unless `--fork-session` is passed.
  - A raw LF in a pty reply makes a newline and does not submit; only `\r` submits. Bracketed
    paste also works and is the fallback if a release changes this. A reply should have any `\r`
    normalized.
  - Pty permission dialogs come one at a time in call order, with stable markers (title line,
    `Do you want to ...?`, numbered list with `1. Yes` first, `Esc to cancel`). The option count
    varies (Bash 4, Write, Edit and Read 3).
  - The transcript does not hold the pending `tool_use` while a dialog is open, so the screen is
    the only source. Before sending "Yes", gx re-reads the screen and refuses if the dialog
    differs from what the person saw. If the screen parser is too fragile, pty shows
    "permission prompt" with no detail and approval stays in the terminal.
  - Headless stream-json multi-turn over stdin works on default auth, with `command_lifecycle`
    events.
  - Bedrock is untested, so keep a `gx doctor` check for the stream-json flags and event shapes.
  - A `gx doctor` check and the pty golden files also catch dialog text changes in Claude releases.
- The upgrade notes (hostnames, and the migrate step) go in the changelog and the web UI docs.
