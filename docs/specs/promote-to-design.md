# Promote to Design: turn a plain chat into a Design

Source: wayfinder map `gx:promote-to-design` (ticket 01). `[01]` marks a point ticket 01 settled;
`[map]` marks a point the map itself set (Notes, Out of scope); `[default]` marks a point the map
decided by default. Terms (Design, Design session, Session ticket, Design agent, Queue agent,
Promote to Design) are in `GLOSSARY.md`.

## Problem Statement

A person starts a plain agent chat and talks about something. At some point the topic grows too big
for a chat and needs a proper Design. Today the person has to leave the chat, pick a slug, write the
idea by hand and run `gx-design` or New Design. The context built up in the chat (the goal, the
decisions already made, the questions still open) is lost or has to be retyped.

## Solution

Promote to Design. The person types the `gx-promote-to-design` skill in the chat, or clicks a
secondary "Promote to Design" button in the web session header (which sends the same skill
command to the chat). The chat's agent proposes a slug, a project and a brief. The person approves
or corrects them in the chat. On approval the agent runs `gx designs open`, which creates the map
and starts a new Design session. The brief becomes the map's `## Initial prompt`. Chart treats the
brief's decisions as settled. The old chat stays a plain session. [01]

## User Stories

1. As a person in a plain chat, I want to type one skill to turn the current topic into a Design, so that I do not choose a slug and retype the idea myself.
2. As a person in the web UI, I want a "Promote to Design" button in the session header, so that I can start the same flow without remembering the skill name.
3. As a person, I want the agent to draft a brief from the chat, so that the goal, the decisions made and the open questions carry over into the Design.
4. As a person, I want the agent to propose the slug and the project, so that I only confirm or correct them.
5. As a person, I want the proposal to use the chat's own project by default, so that the usual case needs no change.
6. As a person, I want to approve the proposal on a question card, so that I answer with one click.
7. As a person, I want to answer the card with a note and get a redrafted proposal, so that I can correct the brief, slug or project without starting over.
8. As a person, I want the Design to run in a new Design session in its own Design worktree, so that it gets a clean context and the strongest model for Chart.
9. As a person, I want the old chat to stay open as a plain session, so that I can keep using it or close it myself.
10. As a Design agent, I want the brief to name the chat's branch and changed files, so that I can find code my trunk-based worktree cannot see.
11. As a Design agent, I want the brief to end with the chat's harness, conversation id and transcript path, so that I can read the transcript when a detail is missing.
12. As a Design agent, I want the brief to make sense without the transcript, so that I read the transcript only to check a detail or fill a gap.
13. As a Design agent running Chart, I want the brief's decisions marked settled, so that I do not grill the person again on them.
14. As a Design agent running Chart, I want to still confirm the Destination and to raise a decision again when it conflicts with the code or with another decision, so that a bad decision does not slip through.
15. As an agent, I want `gx agent transcript` to print the transcript path of my own session for Claude and for Codex, so that the skill can put it in the brief.
16. As a person, I want the button hidden on Design agents and Queue agents, so that I cannot promote a session that gx already drives.
17. As a person, I want the button to follow the composer's send rules and stay disabled until the chat has a message, so that it never sends into a session that cannot take input or has nothing to promote.
18. As a person, I want the button to send the skill command with the right prefix for the harness (Claude `/`, Codex `$`), so that it works in both.
19. As a person, I want a click to send at once with no confirm dialog, so that starting the flow is cheap. The approval card is the confirmation.
20. As a person, I want a draft I typed in the composer to stay as it is after the click, so that I lose no typing.
21. As a person, I want to stay on the chat after approval, with the agent's last message linking to the new Design, so that I can open the Design when I choose.
22. As a person, I want the agent to shorten a brief that would pass the 32 KB idea limit, so that `gx designs open` never rejects it.
23. As a person, I want the agent to propose another slug when `gx designs open` refuses one, so that I do not retype it.
24. As a person, I want the agent to report the error and offer the Design's address when the map was created but its session failed to start, so that I can start the session myself and no second map appears.
25. As a person in a TUI session, I want to type the skill, so that Promote to Design works there too (no TUI key yet). [map]

## Implementation Decisions

### Skill `gx-promote-to-design`

- A new gx skill, installed and discovered like the other `gx-*` skills. It takes no argument. `gx-design` is unchanged for this flow. [01]
- Steps, in order:
  1. Draft the proposal: slug, project (the session's project by default), and the brief.
  2. Run `gx agent actionable` so the session shows Actionable while it waits. gx raises Actionable from a round only in Design sessions, so a plain session needs this call. [default]
  3. Ask for approval on a question card (yes-no). Yes creates the Design. A note leads to a redraft and a new card. [default]
  4. On Yes, pipe the brief into `gx designs open <slug> --idea - --project <project>`. Passing a multi-paragraph brief with backticks and quotes as a shell argument is fragile. [default]
  5. Tell the person the Design's address in the last message, as a link to the Design page. The skill gets the web base URL from `gx server web-url --json` and adds the Design page path. With no web UI it prints `project:slug` only. [default]
- Failure handling: [default]
  - A refused slug: propose another and ask again.
  - Map created but session not started: report the error, give the Design's address, create no second map.
  - A brief over 32 KB: shorten it before the call; never let the call reject it.
- The brief is the whole idea text and lands in the map's `## Initial prompt`. It holds: the goal, the decisions made, the open questions, and the chat's branch and changed files. It ends with the chat's harness, conversation id and transcript path. [01]
- "Changed files" means the working-tree changes plus the diff against the merge-base with trunk. On trunk with a clean tree, the brief says there is no unmerged code. [default]
- The brief's first line is a fixed sentence that marks it as promoted. The skill writes it and the `gx-wayfinder` Chart rule quotes it. A skills-package check fails when the two differ. [default]
- Why the brief names the branch and files: the Design branch starts from trunk, so the Design agent cannot see the chat's uncommitted or unmerged code. No code moves. [01]
- The skill writes prose for a cold reader. The brief must make sense without the transcript. [01]
- Old chat: stays open and plain, not linked to the map. The person closes it. The skill does not stop or link the old session. [01]
- Words in the skill text follow `GLOSSARY.md` (Promote to Design, Design, Design session).

### Chart rule in `gx-wayfinder`

- Add a short rule to the "Chart the map" step: when the map's `## Initial prompt` comes from Promote to Design, its decisions are settled and go into Decisions so far. Chart still confirms the Destination, and may raise a decision again when it conflicts with the code or with another decision. [01]
- The rule recognises a promoted brief by its fixed first-line sentence (see the skill section). The implementer picks the wording. The skill and the rule share it from one place, and a skills-package check fails when they differ. [default]

### CLI: `gx agent transcript`

- A new verb under `gx agent`, next to `actionable` and `link`. It resolves the calling session from the same environment variable those verbs use.
- It prints the path of the session's conversation file. For Claude it reuses the server's `Chat.ClaudeTranscript` lookup (`server/sessions/chat/transcripts.go`) (honours `CLAUDE_CONFIG_DIR`, tries the expected project dir first). For Codex it uses the server's Codex rollout lookup. The route calls the `server/sessions/chat` lookup the web chat uses (it wraps `ClaudeTranscript` and `codexRollout` behind the transcript miss cache), so the verb and the chat cannot disagree. Several matches count as "not found". `transcript.FindByID` is not used: it ignores `CLAUDE_CONFIG_DIR` and can return several matches. [default]
- A new verb is still needed: `gx claude session-path <id>` needs the id and covers Claude only, while this verb finds the calling session itself and covers Codex.
- `--json` exits 0 and always returns `harness`, `conversation_id`, `path` and `found`. Harness and id are empty only outside a gx session. The skill uses `--json`, so it gets all three values in one call, and it still has the harness and id when no file is found. Text mode prints the path. [default]
- Outside a gx session, or when no file is found, text mode exits 1, prints nothing on stdout and gives the reason on stderr. The skill then leaves the transcript line out of the brief. The brief stays valid without it. [default]
- A verb that reads a session goes through the server like the other `gx agent` verbs; a matching server route and the parity table in `cmd/server_parity_test.go` follow the existing pattern.

### CLI: `gx designs open`

- Today, in text mode, `printSessionResult` (`cmd/agent/agents.go`) returns nil when the session is nil, so a created map with a failed session start exits 0 and prints nothing. The skill cannot detect the half-made Design. [person, Q1]
- Fix: when `EpicCreated` is set and `StartError` is not empty, the command prints `project:slug` and the start error, and exits non-zero. This is the cause fix, so the skill does not need `--json` to work around it. The condition is not "Session is nil": `printSessionResult` is shared with `gx designs delete`, the `gx agents` verbs and `agentsfix`, and a successful delete has no session. `--json` keeps exit 0 and already carries `epic_created` and `start_error`. [default]
- A CLI test checks that the text output and exit code of `gx designs delete` do not change.
- Add `--idea -`: read the idea text from stdin. The 32 KB limit applies as before. [default]
- Tests: a CLI test for the failed-start output and exit code, and one for `--idea -`.

### Web UI: button

- A secondary "Promote to Design" button in the web session header (`SessionHeader` in the Agents session view), in both the wide and the phone layout. [01]
- Shown on plain sessions only. Hidden on Design agents and Queue agents. [default]
- Disabled when the composer cannot send (offline, non-chat harness, exited, busy) and while the chat has no message. When disabled, the reason is shown the same way the composer shows its reason. The implementer picks the exact text and whether the header reads the chat's message count from props or a hook. [default]
- A click posts a new `promote` field on `SessionPromptRequest` (`server/wire/sessions.go`), next to the `Continue` field. The server expands it to one text, `harness.SkillCommand(meta.Harness, "gx-promote-to-design", "")`: `/gx-promote-to-design` for Claude, `$gx-promote-to-design` for Codex. The web UI keeps no prefix table. The send keeps the composer's refusals and the same `last_message` check. [person, Q2]
- Unlike `continue`, `promote` sends that one text through the plain send path and never calls `startContinue`, because that path types `/clear` first and would wipe the chat being promoted. Like `continue`, it replaces `Text`. The event log records `Action: "promote"`. [default]
- The server refuses `promote` on a Design session and on a Queue agent with a new reason (the implementer names it, for example `not-plain-session`). A request with both `continue` and `promote` set is a bad request. The web shows the reason like other refusals. [person, Q3]
- No confirm dialog. A draft in the composer stays untouched; the click neither reads nor clears it. [default]
- After a click the page stays on the chat. A refused send shows the same refusal the composer shows.

### Not changed

- The 32 KB idea limit of `gx designs open`. The skill adapts to it.
- The Design session start (Design worktree, strongest model, auto-advance, Chart handoff) is whatever `gx designs open` already does.
- No change to the Session ticket of the old chat.

## Testing Decisions

- A good test checks external behaviour: what the person sees in the web UI, what the CLI prints and what the server returns. It does not check component internals or skill wording.
- Seams, highest first:
  - Web e2e flows through the fixture server (`webui/e2e`), for the button.
  - CLI and server verb tests for `gx agent transcript`, including the parity table, and for the `gx designs open` changes.
  - A server test for the `promote` field of `SessionPromptRequest`: Claude gets `/gx-promote-to-design`, Codex gets `$gx-promote-to-design`, the `last_message` stale check applies, and the server refuses on a Design session, on a Queue agent, and when `continue` is also set.
  - The skills package checks (bundle, rule and duplicate checks) for the new skill and the edited gx-wayfinder step, plus a check that fails when the skill's fixed first-line sentence and the `gx-wayfinder` rule differ.
- The web unit tests cover the button's visibility and disabled rules with a session fixture, the way the composer tests cover Send.
- The skill's conversation (the card, the redraft, the failure paths) cannot run in CI, since the agent is a model. Verification runs it by hand against a temporary server.
- Prior art: the composer tests (`AgentComposer.test.tsx`, `AgentsPage.composer.test.tsx`), the `gx agent actionable` and `link` tests, `harness.SkillCommand`'s test, the `gx designs open` tests, and the flow catalog's Agents flows.

## Verification

No stored mock exists, so there is no Mock states list. A verify ticket runs these against a
temporary gx server. Flow numbers below are new; the implementer takes the next free numbers in
`webui/e2e/CATALOG.md`.

- **Story 1, 3, 4, 5, 6, 7:** manual run in a fixture plain chat. Type `/gx-promote-to-design`. Expect a proposal with a slug, the chat's project and a brief; a yes-no card; a redraft after a note.
  - New flows or scenarios: none (needs a real agent).
- **Story 2, 17, 18, 19, 20:** new flow "Promote to Design button". Scenario: a plain Claude session with one message.
  - Steps: open the session at wide width and at phone width; the button is enabled in both headers. Type a draft; click; the fake agent receives `/gx-promote-to-design`; the draft is still in the composer; no dialog appeared.
  - Same flow on a Codex session: the fake agent receives `$gx-promote-to-design`.
  - Same flow on a session with no messages: the button is disabled. On an exited and on a busy session: disabled with a reason.
  - New scenario if none fits: a plain session with a Codex harness and one with no messages.
- **Story 16:** server test (see Testing Decisions): `promote` on a Design session or a Queue agent is refused. Plus new flow "No Promote to Design on driven agents". A Design agent and a Queue agent: no "Promote to Design" button in the header (wide and phone width).
- **Story 8, 9, 21, 22, 23, 24, 25:** manual run with a real or fixture agent. After Yes: `gx agents list` shows a new Design session for the slug and the old chat still plain; the page stays on the chat and the last message links to the Design. Repeat with a taken slug (expect a new proposal) and with a brief over 32 KB (expect it shortened, no refusal). Repeat in a TUI session with no button.
  - CLI check: `gx designs open <slug> --idea - --project <project>` with a 33 KB text on stdin refuses; confirms the skill must shorten.
  - Story 24 CLI check: make the session start fail after the map is written (test or fixture). Expect the map address and the start error in the output, and a non-zero exit. Then run the skill against that case: one map, no second map, the address offered to the person.
  - Story 21: the link in the agent's last message opens the Design page.
- **Story 10, 11, 12:** manual: read the new map's `## Initial prompt` with `gx tickets show <project:epic>`. It names the chat's branch and changed files and ends with harness, conversation id and transcript path.
- **Story 13, 14:** manual: the new map's `## Initial prompt` starts with the fixed first-line sentence. In the new Design session, Chart treats the brief's decisions as settled (they appear in Decisions so far), still confirms the Destination, and a conflicting decision is raised again.
- **Story 15:** CLI checks. In a Claude session and in a Codex session (fixture), `gx agent transcript` prints an existing path whose file name holds the conversation id. Outside a gx session, it exits 1 with nothing on stdout and a reason on stderr (`--json`: `found: false`); the skill's transcript line is then left out.
  - New tests: a verb test per harness and one for the outside-session case, plus the server route test and the parity table entry.

## Out of Scope

- A TUI key for Promote to Design. The first version adds a web button only; the skill already works in TUI sessions by typing it. A key can come later on top of the same skill. [map]
- A web dialog for slug, project or brief. The approval happens in the chat. [01]
- Moving code, linking the old chat to the map, or turning the chat session into a Design session. [01]
- Stopping or closing the old chat for the person. [01]
- Promoting a Design agent or a Queue agent. [default]

## Further Notes

- Tickets for the build use the epic `promote-to-design-impl`. Suggested slices: the `gx agent transcript` verb with its server route; the skill plus the gx-wayfinder Chart rule; the `gx designs open` fix and `--idea -`; the web button with the `promote` field and its e2e flows and the CATALOG entry.
- Implementer's choices (the map did not decide them): the slug-proposal wording and how the agent derives a slug; the wording of the fixed first-line sentence of the brief; the flag names of `gx agent transcript`; the name of the server's refusal reason for `promote`; the exact text of the button's disabled reason; how the header learns that the chat has a message; the e2e flow numbers and scenario names.

## Package layout (after the server split)

The server is split into packages and `sizeguard` fails a package over 50 files, 4,500 source lines, 5,000 test lines, or a file over 1,000 lines (see `.ai/index.md`, "File size"). `server/sessions` is near its limits (49 files, 4,325 test lines), so do not add files there.

- The `gx agent transcript` route goes in `server/sessions/chat`, next to `Chat.ClaudeTranscript`. The verb goes in `cmd/agent`.
- The `promote` field and its refusal go in `server/wire/sessions.go` and `server/sessions/prompt/sessionprompt.go`.
- The `gx designs open` change goes in `server/designs/designs.go` and `cmd/agent/agents.go`.
