# e2e triage 2 (design, design-page, newdesign, jump, phone)

All vitest paths are under webui/src. "Merge" means: same scenario, fold the assertions into the named flow so one fixture start is shared. Flow numbers are the catalog numbers in the test titles. Sub-tests are labelled a/b/c in file order.

| file | flow | title | verdict | reason | vitest file |
|---|---|---|---|---|---|
| design.e2e.ts | 4 | Start a design session, shows the first step | MERGE-INTO 5 | Same "design" scenario and same Start click; the step/Epics-link checks are rendering and the first-message check is the first leg of 5 | design/DesignPage.test.tsx (Progress steps 245), design/ChatLog.test.tsx |
| design.e2e.ts | 5 | Answer a round (ids, answer, Continue, fresh round) | KEEP-E2E | Real prompt round trip, run-end and fresh round over SSE; target for flow 4 | n/a |
| design.e2e.ts | 47 | Send popover | TO-VITEST | Gaps list, row jump, focus, Escape/outside click, second click, Send anyway are all in Composer.test already. Only the popover-above-Send geometry and the unchanged composer height need a real engine: fold those into 49a | design/Composer.test.tsx (732-849 already cover it) |
| design.e2e.ts | 48 (x2 modes) | Fixed sheet rows while body scrolls | MERGE-INTO 74 | Same scenario, same 1280x600 viewport, same Q8-note scroll geometry as 74; real scroll/layout so it stays e2e, but one test per mode instead of two files' worth | n/a |
| design.e2e.ts | 49a | View toggle, Escape layers, round across switches | TO-VITEST | Escape per layer (popover, jump, menu, phone steps, note editor), toggle counts, round surviving switches are all in ChatLog.test. Keep only the 820px max width, sheet fills column and phone Escape geometry by folding into 74 | design/ChatLog.test.tsx (568-760, 751 "survive two view switches", 612-679 Escape layers) |
| design.e2e.ts | 49b | Round sent from Chat, next round opens in Questions | DELETE-DUPLICATE | Same wire path as flow 5 (answer, send, Continue, Q9); view assertion is in ChatLog.test 691/707 | design/ChatLog.test.tsx 691, 707 |
| design.e2e.ts | 49c | Busy turn with no round: no View group | MERGE-INTO 14 | Same "design-working" scenario and Start click; assertions are pure render, also in ChatLog.test | design/ChatLog.test.tsx 434, 736 |
| design.e2e.ts | 9a | Design progress, 1440px, each step in turn | KEEP-E2E | `tickets set --design-step` over the CLI, then the page follows live (SSE). Also absorb the ring-tone check from 64 | n/a |
| design.e2e.ts | 9b | Design progress, 375px step line | TO-VITEST | Same server push as 9a is already proven; phone step line and impl link are render logic | design/DesignPage.test.tsx 277, 302, 318, 346 (data: designDetail mock with design_step per step) |
| design.e2e.ts | 10 | Auto-advance, fresh round follows run end | MERGE-INTO 12 | Real server countdown is needed, but 12 already runs the same scenario through the countdown; extend 12 to a 3s delay and end by waiting for "A fresh round." | design/Composer.test.tsx 444 covers UI of the countdown |
| design.e2e.ts | 11a | Auto-advance, Cancel stops the send | KEEP-E2E | Needs the real server timer to prove nothing is sent after Cancel (6s wait is the cost); Cancel UI is in Composer.test | design/Composer.test.tsx 444 |
| design.e2e.ts | 11b | Note typed during countdown stops the send | TO-VITEST | Client behaviour (typing/sending a note posts cancel). Server-side "cancelled means not sent" is proven by 11a. Needs a countdown mock and a fetch spy on the cancel route. Saves a 6s sleep. Verify the Go side has a cancel test before dropping | design/Composer.test.tsx (add next to 444) |
| design.e2e.ts | 13a | Defaulted questions: one card, defaults as text | TO-VITEST | Pure render from data | design/ChatLog.test.tsx 776 (already covers it) |
| design.e2e.ts | 13b | All-defaulted round: Accept card and grilling Actionable | KEEP-E2E | The card is covered (ChatLog.test 795) but the "grilling" flag on the agent is server state read via `agents list --json` | design/ChatLog.test.tsx 795 for the card part |
| design.e2e.ts | 14 | Working row follows the step report | KEEP-E2E | `gx agent step` from the CLI with GX_AGENT_SESSION updates the row live. Target for 49c, 64b, 73b (all "design-working") | design/ChatLog.test.tsx 191, 200 for rendering |
| design.e2e.ts | 12 | The auto-advance switch | KEEP-E2E | Flip switch off/on against the real server, countdown starts. Target for 10; switch UI is in Composer.test 403 | design/Composer.test.tsx 403 |
| design.e2e.ts | 42 | Stopped session card moves to Ready to design | MERGE-INTO 43a | Same "ready" + startDesign + stopSession. Check the card first, then open the page for 43a and 44 | design/DesignPage.test.tsx (no-session states 206-237) |
| design.e2e.ts | 43a | New session on stopped Design from ⋯ menu | KEEP-E2E | New session starts a second real agent session; target for 42, 43b and 44 | design/SessionControls.test.tsx 237 |
| design.e2e.ts | 43b | New session on a live session asks first | MERGE-INTO 43a | Same page after startDesign; do live-session confirm (Cancel then OK) before stopping. Confirm/decline logic is already in SessionControls.test | design/SessionControls.test.tsx 206, 227 |
| design.e2e.ts | 44 | Failure banner New session | MERGE-INTO 43a | Same stopped session; click the banner button as another leg. Banner logic is in FailureBanner.test | design/FailureBanner.test.tsx 162 |
| design.e2e.ts | 45a | Delete design appears on stop, Delete removes it | KEEP-E2E | Menu item appears when the server sees the session stop, and the delete really removes the epic on disk (`tickets show` rejects). Keep-confirm and pending states are in DesignPage.test | design/DesignPage.test.tsx 358, 383, 392 |
| design.e2e.ts | 45b | A charted design has no Delete design | TO-VITEST | Pure data-driven visibility | design/DesignPage.test.tsx 358 (already covers "only for a deletable Design") |
| design.e2e.ts | 50 | Design epic page | KEEP-E2E | Server reads CLI-written section and claim and the page updates; ring tone and comet over the wire. Contains polling loops on hash reload (slow) | epic/EpicPage.test.tsx covers static rendering |
| design.e2e.ts | 46a | gx designs delete removes an uncharted design | KEEP-E2E | CLI/server round trip. Needs no browser; target for 40 and 46b (run on the "design" scenario) | n/a |
| design.e2e.ts | 46b | gx designs delete refuses a charted design | MERGE-INTO 46a | Pure CLI, same command; run on "design" scenario in the same flow | n/a |
| design-page.e2e.ts | 64 (390px) | Design ring tracks the session | TO-VITEST | Phone ring size/position is render; tone from data | design/DesignPage.test.tsx 167, ui/progress-ring/ProgressRing.test.tsx |
| design-page.e2e.ts | 64 (1440px) | Design ring tracks the session | MERGE-INTO 9a | Same "design" scenario and `tickets set --design-step done`; add the ring tone assertions (muted, waiting, done) to 9a. Ring-left-of-name position is layout, keep it there | design/DesignPage.test.tsx 167 |
| design-page.e2e.ts | 64b | Ring runs with comet while session works | MERGE-INTO 14 | Same "design-working" scenario and Start click | ui/progress-ring/ProgressRing.test.tsx |
| design-page.e2e.ts | 72a | One live session: New session in menu only | TO-VITEST | Menu contents from data | design/SessionControls.test.tsx 253 |
| design-page.e2e.ts | 72b | Two live sessions: list, switch, stop | KEEP-E2E | Stop from the menu sends a real state token and the server exits the session. Target for 72c and 72d | design/SessionControls.test.tsx 260, 285 |
| design-page.e2e.ts | 72c | 390px menu is a bottom sheet with grab handle | TO-VITEST | Already covered; box geometry is jsdom-testable by mocked viewport | design/SessionControls.test.tsx 311, 335 |
| design-page.e2e.ts | 72d | 1440px menu is a popover under ⋯ | MERGE-INTO 72b | Same "design-two-live" scenario; popover placement and 44px trigger are real positioner layout, so keep those assertions but in 72b via setViewportSize | design/SessionControls.test.tsx (no placement coverage) |
| design-page.e2e.ts | 73a | Note box: rows, viewport, large editor, Done, Send | TO-VITEST | Rows, expand button, modal titles, Done keeping text, send with note all covered. Move the toBeInViewport check into 74 | design/Composer.test.tsx 271, 295; design/QuestionSheet.test.tsx 39, 86; design/NoteBox.test.tsx 8; design/ChatLog.test.tsx 538 |
| design-page.e2e.ts | 73b | No round: message box 5 rows and expand button | DELETE-DUPLICATE | Same assertion as Composer.test 271 | design/Composer.test.tsx 271 |
| design-page.e2e.ts | 74 (Questions view) | A move starts at the question's title | KEEP-E2E | scrollTop and bounding box in a real engine at 1280x600. Merge the 48 assertions in; one test can switch to Chat view midway so both modes use one fixture | n/a |
| design-page.e2e.ts | 74 (docked in Chat view) | A move starts at the question's title | MERGE-INTO 74 (Questions view) | Same scenario; switch to the Chat view in the same test instead of a second server start | n/a |
| design-page.e2e.ts | 75 | Text size | TO-VITEST | Computed font-size works in jsdom (NewDesignForm.test 33 does it). Needs the QuestionSheet and chat log rendered with a round; responsive font sizes via media queries are the one risk, check at the target breakpoint | design/QuestionSheet.test.tsx, design/ChatLog.test.tsx (add getComputedStyle checks) |
| newdesign.e2e.ts | 25 | New Design from an empty page | DELETE-DUPLICATE | Same assertion as EpicsPage.test | epics/EpicsPage.test.tsx 230 |
| newdesign.e2e.ts | 26 | New Design form | KEEP-E2E | Start design creates the real epic (`tickets show` kind map, design_step chart) and navigates. Layout bits (button right of filter, idea over half the dialog) need a real engine. Target for 39 | design/NewDesignForm.test.tsx 128 |
| newdesign.e2e.ts | 34 | New Design text is 16px | DELETE-DUPLICATE | Same assertion | design/NewDesignForm.test.tsx 29 |
| newdesign.e2e.ts | 35 | Project select and starting project | TO-VITEST | Select options and starting-project rules are covered; the Epics filter wiring is a prop | design/NewDesignForm.test.tsx 39-71; epics/EpicsPage.test.tsx 241 (add filter-to-form assertion) |
| newdesign.e2e.ts | 36 | Draft keeps its project | DELETE-DUPLICATE | Same assertion | design/NewDesignForm.test.tsx 52 |
| newdesign.e2e.ts | 37 | Draft is shared and closing keeps it | TO-VITEST | Close/Escape/backdrop and restore are covered; cross-tab sync is a storage event | design/NewDesignForm.test.tsx 73, 86; drafts.test.ts 38 |
| newdesign.e2e.ts | 38 | New Design Discard | DELETE-DUPLICATE | Same assertions | design/NewDesignForm.test.tsx 105, 113 |
| newdesign.e2e.ts | 39 | Refused for an existing epic | MERGE-INTO 26 | Same "ready" scenario; submit the duplicate slug "ready-epic" first, then the fresh one. The refusal text comes from the server, so keep the real server for it | design/NewDesignForm.test.tsx 148 (dialog side) |
| newdesign.e2e.ts | 40 | gx designs open starts a New Design | MERGE-INTO 46a | Pure CLI, no browser; do open then show then the epic-exists refusal in the CLI-only flow | n/a |
| newdesign.e2e.ts | 94 | Launches at the strongest tier | KEEP-E2E | Reads launch args the server passed to the agent (opus, max). Can drive it with `fixture.cli designs open` instead of the dialog and drop the browser, if the CLI path shares the tier code | n/a (Go-level test could host it) |
| newdesign.e2e.ts | 27 | Design a not-started Design | KEEP-E2E | Card button starts a real agent session and navigates | epics/EpicCard.test.tsx covers the button |
| jump.e2e.ts | 20 | Jump menu opens and closes | TO-VITEST | Open, focus, Ctrl+K, filter reveals Ready/Finished rows, Escape all covered | nav/JumpMenu.test.tsx 70, 76, 126, 181 |
| jump.e2e.ts | 21 | Jump menu keyboard and picks | TO-VITEST | Arrow/Enter and the Design/epic/one-off route picks are covered, including URL targets | nav/JumpMenu.test.tsx 146, 157; epic/EpicPage.test.tsx for the one-off Epics tab with no count |
| jump.e2e.ts | 22 | Back link count | KEEP-E2E | Count follows the server with no reload after `tickets set`, and depends on the scenario settling (9 cards). Target for 23 and 53a. Drop the computed-colour assertion or keep it, it is cheap here | design/DesignPage.test.tsx 116 covers counting from a loaded list |
| phone.e2e.ts | 23 | Phone, Epics page | MERGE-INTO 22 | Same "united" scenario; single-column grid, time on one line and Epics tab on one line are real layout, so keep them in 22 using setViewportSize(375) | n/a |
| phone.e2e.ts | 53a | Phone epic page, buttons below stats line | MERGE-INTO 22 | Same "united" scenario, real layout (bounding boxes); run after the 375px switch | n/a |
| phone.e2e.ts | 53b | Phone epic page, card meta below its title | MERGE-INTO 53c | Same "mixed" scenario; real layout (wrap, padding). Add the long-title ticket in 53c, config recovery false should not hurt | n/a |
| phone.e2e.ts | 53c | Phone epic page, answer and fix full-width | KEEP-E2E | Needs real layout and the mixed scenario's queue reaching 4/7 (30s). Target for 53b | n/a |
| phone.e2e.ts | 41 | Phone, New Design sheet | DELETE-DUPLICATE | 88% height, grab handle, 1:2 footer, overlay on visual viewport are all asserted | design/NewDesignForm.test.tsx 189, 199, 211, 221 |
| phone.e2e.ts | 24 | Phone, Design page | TO-VITEST | Steps sheet open/close paths, View toggle, Note icon-only, Auto names, jump sheet are all covered. Move the two geometry checks (popover inside 375px, jump sheet at 0,0,375x812) into 74 after a setViewportSize(375) | design/DesignPage.test.tsx 302, 318, 346; design/ChatLog.test.tsx 568, 662; design/Composer.test.tsx 339; nav/JumpMenu.test.tsx 247 |

## Summary

Counts per verdict (every row above, a/b/c and viewport variants counted as separate rows, 57 rows):

- KEEP-E2E: 17 (5, 9a, 11a, 13b, 14, 12, 43a, 45a, 50, 46a, 72b, 74, 26, 94, 27, 22, 53c)
- TO-VITEST: 16 (47, 49a, 9b, 11b, 13a, 45b, 64@390, 72a, 72c, 73a, 75, 35, 37, 20, 21, 24)
- MERGE-INTO: 17 (4, 48, 49c, 10, 42, 43b, 44, 46b, 64@1440, 64b, 72d, 74 docked, 39, 40, 23, 53a, 53b)
- DELETE-DUPLICATE: 7 (49b, 73b, 25, 34, 36, 38, 41)

Flows that keep a browser and server start after merging (about 17 fixture starts instead of about 56): this file set drops from ~57 tests to ~17 tests.

Five largest wins:

1. Fold 42, 43a, 43b, 44 into one stopped-session flow (4 server starts -> 1, all "ready" + startDesign + stopSession).
2. Delete the whole New Design dialog block as e2e (34, 36, 38, 41, 25, 37, 35): NewDesignForm.test and EpicsPage.test already assert each one. Merge 39 into 26 and 40 into 46a (CLI only).
3. Move 20, 21 (Jump menu) to JumpMenu.test: fully covered; saves two "united" starts (heavy scenario).
4. Merge the four "design-working" starts (14, 49c, 64b, 73b) into 14, and the layout flows on "united" (23, 53a) into 22 (also the 60s settle wait is paid once).
5. Move 47, 49a, 49b, 73a, 73b, 75, 24 to vitest (Composer.test, ChatLog.test, QuestionSheet.test); all these are "design" scenario starts with long UI scripts and 1h auto-advance delays. Also drop the 6s sleep in 11b.
