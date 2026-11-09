## Problem Statement

When I work with an agent in gx's web UI, I cannot see how full its context window is. `gx claude statusline` warns me before a Claude session leaves the smart zone, but the Agents, Design and Triage pages give no hint. I only learn the session has gone too far when its answers get worse. The model name also sits in places that vary by page (the page header on Agents and Triage, the menu on Design), far from the box I type in.

## Solution

Every composer shows a small footer line inside its text box: the agent's model on the left, a context badge on the right. The badge is a gauge ring that fills with the session's token count, coloured by how close the count is to the 130K smart-zone ceiling, followed by the count in thousands ("70K"). Hovering it says "70K of 130K smart zone".

The colour comes from one scale shared with `gx claude statusline`: green under 75K, yellow under 100K, orange under 130K, red from 130K. The server works out the level, so the web UI keeps no thresholds. The count follows the agent's transcript, so it updates after every assistant message, for Claude and Codex, and for Queue agents on the Agents page.

The model name leaves the page header (Agents, Triage) and the Design page's menu. The footer line is its only home.

## User Stories

1. As a person chatting with an agent on the Agents page, I want a badge with the session's context size beside my text box, so that I notice before the session passes the smart zone.
2. As a person answering a design agent on the Design page, I want the same badge in the composer, so that I know when a design session is getting too long.
3. As a person running a Triage, I want the same badge in the Triage composer, so that I know when the Triage session is getting too long.
4. As a person reading the badge, I want the count in whole thousands ("70K"), so that it reads at a glance.
5. As a person reading the badge, I want a gauge ring that fills with the count, so that I see the fill level without reading the number.
6. As a person reading the badge, I want green under 75K, yellow under 100K, orange under 130K and red from 130K, so that colour tells me how near the ceiling is.
7. As a person who also uses `gx claude statusline`, I want it to use the same colour boundaries, so that the terminal and the web never disagree about the same count.
8. As a maintainer, I want one Go function to own the scale, next to the smart-zone ceiling, so that the statusline and the server cannot drift apart (`gx agent` keeps using the ceiling constant alone).
9. As a maintainer, I want the web UI to hold no thresholds and only map a level to a colour, so that changing the scale is a Go-only change.
10. As a person hovering the badge, I want a tooltip "70K of 130K smart zone", so that the number has its reference.
11. As a person with a fresh session, I want no badge before the agent's first reply, so that I never see a misleading "0K".
12. As a person with a fresh session, I want the footer line to show the model alone, so that the model is still visible.
13. As a person who sends a message, I want the badge to update when the agent replies, without reloading the page, so that it stays live.
14. As a person whose session is finished or idle, I want the last count to stay, so that I can still judge whether to continue it or start fresh.
15. As a person who ran /compact, I want the pre-compaction count to stay until the next assistant message, so that the badge never shows a number the transcript has not confirmed.
16. As a person whose session has no readable transcript, I want no badge and no error, so that a missing file does not break the composer.
17. As a person who uses Codex sessions, I want the same badge, scale and tooltip, so that the harness does not change how I read the page.
18. As a person watching a ralph-loop Queue agent on the Agents page, I want its session view's composer to show the badge too (the sidebar row gets none), so that I see when a ticket iteration nears the ceiling.
19. As a person on a desktop or tablet, I want the model to read "Claude Opus 4.5 · high" on the left of the footer line, so that I see the model with its effort.
20. As a person on a phone, I want the model to drop its effort ("Opus 4.5"), so that the footer line fits.
21. As a person using the composer, I want the Attach, Note, Auto and Send, Interrupt, Continue and Send answers buttons exactly as they are, so that my muscle memory still works.
22. As a person on the Design or Triage page with the Note box open, I want the footer line inside the box, so that it stays with the text it describes.
23. As a person on the Design or Triage page with the Note box closed, I want a dim borderless footer line directly above the button row, so that the model and badge are still visible.
24. As a person on the Agents page, I want the model gone from the page header, so that it does not show twice.
25. As a person on the Triage page, I want the model gone from the page header, so that it does not show twice.
26. As a person on the Design page, I want the model gone from the menu's Agent block, so that it does not show twice.
27. As a person with a session whose harness has no chat (NoChat), on any of the three pages, I want the footer line to show the model alone and never a badge, so that the model is still visible and no badge claims a count the server never sent.
28. As a person using dark or light theme, I want the ring and number readable in both, so that the badge works in my theme.

## Implementation Decisions

- **Scale and level.** One Go function maps a token count to a level (green, yellow, orange, red) on the 130K scale. It lives beside the smart-zone ceiling, and the ceiling stays the one source of the 130K value. Its edges are: green under 75K, yellow from 75K, orange from 100K, red from 130K. The level names are an enum on the wire; the implementer picks their spelling.
- **Statusline.** `gx claude statusline` reads the same function for its colour. Red moves from 150K to 130K. The faces (🙂 🤔 🥵) keep their current steps (change at 75K and 100K) and stay a statusline-only concern. Its existing colour test moves to the new boundaries.
- **Count source.** The server reads the count from the session's transcript: the newest assistant message's context occupancy for Claude, the last context tokens for Codex. Assistant lines with zero occupancy (Claude Code's synthetic lines for API errors and interrupts, model `<synthetic>`) are skipped, and the newest real reading is kept, so the badge never drops to "0K". It is read in the same cached transcript read that gives the reported model (the cache is keyed by transcript path), so one snapshot builds no extra file read per session. Codex shares that cache or an equivalent. After /compact the newest assistant message is still the pre-compaction one, so the old count stays until the next assistant message; this needs no special case.
- **Wire.** Each session in the snapshot carries the token count and its level, and the snapshot carries the smart-zone ceiling once (one value, from `agent.SmartZoneCeiling`). A session with no reading (before its first reply, or no readable transcript) carries neither, and the web shows no badge. The count does not change when a session finishes or goes idle.
- **Queue agents.** `queueAgentRow` (`server/queue/queueagents.go`) builds its `wire.SessionInfo` by hand and reads no transcript today. It fills count and level through the same cached transcript read (keyed by the transcript path from the run's `Cwd` and `Session` for Claude, or its rollout for Codex) and the same level function. The badge shows in the Queue agent's session view composer; the sidebar row gets none.
- **Live update.** No new event is needed: a growing chat already fires `session-chat-changed`, and every event refetches the snapshot, so the field updates per assistant message. The implementation must confirm this on all three pages.
- **Display.** The badge shows the count in whole thousands. The implementer picks the rounding of the number shown, but the colour and the ring use the level and exact count from the server, never the rounded text, so a count just under 130K stays orange. The tooltip reads "<N>K of 130K smart zone" with the 130K taken from the ceiling in the snapshot. The ring fills from the count against that ceiling and is full at or above it. The web holds no 130K of its own. The implementer picks the ring's markup, its colour tokens and how it behaves in dark theme (the mock has both).
- **Footer line.** One shared footer-line component renders the model (left) and badge (right) and is used by both composers: the Agents page composer and the one Design and Triage share. Both fill the send slot of the shared composer row, but the footer line is not part of the button row, which does not change. The footer line shows on every composer, on every width. With the Design/Triage Note box open it sits inside the text box, as in the mock. With the Note box closed it is a dim borderless line directly above the button row. The Agents page composer always has its text box, so the line is always inside it. On Design and Triage the composer is not always mounted (a NoChat notice replaces it; a finished, crashed or ended session hides it). The footer line still shows whenever the session view shows, under the notice or in place of the hidden composer: the model alone for NoChat, the model and the last count for a finished, crashed or ended session.
- **Model.** The footer's model text is today's `<Model> · <effort>`; on a phone (the existing narrow-layout check, under 40em) the effort is dropped. The model leaves the page header (Agents, Triage) and the Design menu's Agent block, including the code and tests that exist only for them. Only the Model row leaves the ⋯ menu (`withModel` goes away): Tier, the "switched by hand" note and the phone Effort row stay on all three pages, Design included. Before the first reply the footer shows the model alone.
- **Harnesses.** A NoChat harness renders a disabled composer on the Agents page and a notice on Design and Triage. On all three pages the footer line shows the model alone, never a badge, because the server sends no count for it.
- **Prototype.** The approved layout is the stored mock listed under Verification; the mock's markup is a throwaway and the real component follows the repo's web conventions.

## Testing Decisions

- A good test checks what a person sees or what a command prints, not how the code is built.
- **Go unit tests on the level function** (table test at each boundary: 74,999, 75,000, 99,999, 100,000, 129,999, 130,000, and 0). Prior art: the existing statusline colour test, which moves to the new edges.
- **Server tests through the snapshot** (`SessionInfo`): a Claude transcript and a Codex rollout each give a count and level; no assistant message gives no reading; a NoChat session gives no reading; an unreadable transcript gives no reading; a /compact transcript keeps the earlier count; a synthetic zero-usage assistant line after a real reply keeps the real count; a Queue agent row carries count and level; the snapshot carries the ceiling. Prior art: the existing reported-model tests of the same read.
- **Web component tests** for the footer line (badge shown or hidden, level to colour, tooltip text, phone drops effort) and for the three pages' headers and the Design menu no longer showing the model. Prior art: `Composer.test.tsx`, `SessionControls.test.tsx`.
- **Playwright flows** (highest seam, at the browser): one new `context-window` scenario with sessions at each level, and one new named flow on it (one flow-budget slot, recorded in `webui/e2e/CATALOG.md`) carries every badge step on Agents, Design and Triage. Follow `gx-playwright-e2es`. Existing flows that assert the model in the header or in the Design menu (the session model label flow, and the Phone Design page flow that reads the menu's model row) must change to read the footer line.
- Seams: the Go level function, the snapshot (`SessionInfo`), and the browser composer. The footer's Note-closed and Note-open placement is checked in the browser, not in unit tests of layout.

## Verification

1. Story 1: flows to run: Agents session read, Agents composer send and draft. New step: a `context-window` Agents page session with a reply shows the badge in the composer footer. CLI and server checks: `gx agents list --json` (or the snapshot) shows a count and level for the session.
2. Story 2: Design page flows; new step: the Design composer shows the badge. Look check: Mock states "desktop".
3. Story 3: Triage journey flow; new step: the Triage composer shows the badge.
4. Story 4: new step: a session at 70,400 tokens reads "70K".
5. Story 5: new step: the ring is present and its fill grows between two counts (assert through the badge's accessible name or a test id, not pixels).
6. Story 6: new steps: sessions at each of green, yellow, orange and red counts show the matching level (read by test id or attribute, one per level).
7. Story 7: `gx claude statusline` with a hook payload of 120,000 tokens renders in the orange colour and 130,000 in red (existing statusline test, new boundaries; exit code 0).
8. Story 8: no browser check; the Go table test covers it.
9. Story 9: no browser check; the web unit test maps each level to a colour, and no web file holds the 75K, 100K or 130K numbers (a repository search is the check). The ring fill and the tooltip's 130K come from the snapshot's ceiling.
10. Story 10: new step: hover or focus the badge; the tooltip reads "70K of 130K smart zone".
11. Story 11: new step: a fresh session before its first reply has no badge.
12. Story 12: same step: the footer line there reads the model alone.
13. Story 13: new step: append an assistant usage line to the session's transcript in the fixture; the badge changes with no reload (the suite fails on any reload).
14. Story 14: new step: a finished session and an idle session keep their last count.
15. Story 15: new step: after a `/compact` command the badge keeps its count; a following assistant message changes it.
16. Story 16: new step: a session with no transcript file shows no badge and the page works.
17. Story 17: Codex flows (Codex design run, Codex queue rows); new step: a Codex session shows the badge and the same tooltip.
18. Story 18: Agents queue-agent flows (Codex queue rows scenario or `queue-agents`); new step: a Queue agent's session view shows the badge and its sidebar row does not.
19. Story 19: Mock states "desktop" and "tablet".
20. Story 20: Mock states "phone".
21. Story 21: Phone Design page flow (footer at 390px is one row of Add, Note, Auto, Send), Agents composer flows, Send answers flow; they pass unchanged apart from the model assertions.
22. Story 22: Mock states "desktop" (Note box open); new step on `design`.
23. Story 23: new step on `design` and `triage`: Note closed shows the dim footer line directly above the button row, with no border.
24. Story 24: new step: Agents session header has no model text; the footer has it.
25. Story 25: same for the Triage page header.
26. Story 26: Phone Design page flow, changed: the menu's Agent block has no model row, and on a phone it still shows Effort (and Tier).
27. Story 27: new step: a NoChat session on Agents, Design and Triage shows the model in the footer line and no badge. Server check: that session's snapshot row carries no count.
30. Finished, crashed or ended session on Design and Triage (story 14): new step: the composer is hidden, the footer line shows the model and the last count.
29. Zero-usage lines (stories 11 and 14): server test with a synthetic zero-usage assistant line after a real reply; the count stays.
28. Story 28: new step: with the browser in dark scheme the badge's ring and number are visible (the number's computed colour differs from the box background).

New flows or scenarios to write: a `context-window` fixture scenario with a Claude session at each of the four levels, a fresh session with no reply, a finished session (also on Design and Triage, where the composer is hidden), a NoChat session, a /compact session, a session with no transcript, a Codex session, a Queue agent, a Design session and a Triage session, so the steps above run on one scenario, in one new named flow (see Testing Decisions). Only the model-relocation assertions change in existing flows.

Mock states:

- `$(gx tickets root)/context-window/mocks/02-composer-row-layout.html`, state `desktop`, width 980, fixture scenario `context-window` (new, Design page, session at 128K), parts: header, text-box, footer-line, model, context-badge, button-row, main-button.
- `$(gx tickets root)/context-window/mocks/02-composer-row-layout.html`, state `tablet`, width 640, fixture scenario `context-window` (new), parts: header, text-box, footer-line, model, context-badge, button-row, main-button.
- `$(gx tickets root)/context-window/mocks/02-composer-row-layout.html`, state `phone`, width 380, fixture scenario `context-window` (new), parts: header, text-box, footer-line, model (without effort), context-badge, button-row, main-button.

## Out of Scope

- The TUI: this is the web composer only.
- Predicting whether the next design step fits: that stays with `gx agent context-window --fits`.
- The statusline hook payload as a data source: the transcript covers Claude and Codex.
- Changing the statusline's faces or its other segments.
- A badge for NoChat harnesses: they show the model in the footer line only.

## Further Notes

- The scale change is visible: `gx claude statusline` goes red at 130K, not 150K, so a session between 130K and 150K that showed orange now shows red.
- The mock's footer line is shown for a Design page with the Note box open. The Note-closed placement was settled in the design round and is not in the mock.
- The Design and Triage Note box is the only place where the footer line changes position; every other composer always draws it inside its text box.
