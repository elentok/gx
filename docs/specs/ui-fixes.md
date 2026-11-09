## Problem Statement

The web UI is hard to use on a phone, and the author writes Design notes and answers questions from an iPhone every day.

- On an iPhone, Safari offers credit-card AutoFill above the keyboard in the agent chat box and the other free-text boxes. These boxes are for chat, notes and ideas, never for payment details.
- The New Design sheet wastes the screen. With the keyboard open, the "Add image" tile and the "Paste or add images..." line take half of what is left. The only way to close the sheet is a small "×" next to the epic name, which is hard to hit. Tapping an image thumbnail also starts the design, because the thumbnail's button submits the form.
- On the design page, the keyboard leaves very little height. The question box should fill all the space between the top of the screen and the keyboard, but chrome above it (sidebar toggle, title block, attached images) and loose spacing around the footer eat that space. The keyboard detection is also fragile: it looks only at the viewport gap, so a pinch zoom or a stale resize can hide or show the chrome wrongly.
- The answered-question card on the ticket page is cluttered: the answer shows twice, the agent's question shows as a bubble next to a web bubble, "Agent's notes" is a separate toggle, and the status row wraps over several lines.

## Solution

- Every free-text box tells iOS not to treat it as a payment or contact form, and keeps its normal keyboard features. A real iPhone is the only proof; the spec asserts the attributes and the rest is a manual check.
- The New Design sheet gets a paperclip icon in its footer on phones and tablets, shows the thumbnail row and status line only when they have something to say, and has a "Close" text button at the right end of its top bar on every device. Thumbnails and every other non-submit button stop submitting the form. Autosave is untouched.
- The agent chat composer shows the same paperclip icon on tablets too (phones already have it). One rule for both composers: phone and tablet get the icon, desktop gets the labelled button or tile.
- The design page, with the keyboard open below desktop width, shows only the view tabs (with the ⋯ menu), the question box and the footer. The footer keeps today's order (Add, Note, Auto, Send) with tighter spacing and no separator line, on every device. The model and the "Open design epic" link move into the ⋯ menu on every device. Attached images become the first entry of the chat log and never show above the questions. Keyboard detection becomes reliable: gap plus a focused editable, ignoring pinch zoom, re-synced when focus changes.
- The answered-question card becomes a compact summary: a label, the agent's detail clamped to three lines with "Show more", the agent's question as plain text, one "You  <answer>" chip, an optional muted note, and a single status line with a state dot.

## User Stories

AutoFill

1. As an iPhone user typing in the agent chat box, I want Safari not to offer credit-card AutoFill above the keyboard, so that the strip stops covering the space I need and never suggests payment data in a chat.
2. As an iPhone user writing the Rough idea in the New Design sheet, I want the same protection, so that a design note never triggers card or contact suggestions.
3. As an iPhone user answering a question, adding a note, replying to a permission prompt or a recovery card, I want the same protection in those boxes, so that no free-text box in gx behaves like a payment form.
4. As an iPhone user, I want autocorrect, autocapitalize, spellcheck and the normal keyboard to keep working in those boxes, so that removing AutoFill does not make typing worse.
5. As the maintainer, I want the chosen attributes covered by unit tests, and a written manual iPhone check for the part only a device can show, so that a regression in the attributes is caught and nobody believes the suggestion strip is proven gone by a test.

New Design sheet

6. As a phone or tablet user in the New Design sheet, I want a paperclip icon in the footer, to the left of Discard, so that I can add an image without a big tile above the keyboard.
7. As a phone or tablet user, I want the 72px "Add image" tile and the "Paste or add images..." line gone, so that the idea box keeps the space.
8. As a phone or tablet user, I want the thumbnail row to appear only when the draft has images, so that an empty draft shows no empty strip.
9. As a phone or tablet user, I want the status line under the images to appear only when it has something to say (an image count, an upload in progress or an error), so that the sheet is not cluttered with limits text.
10. As a desktop user, I want the "Add image" tile and the limits line to stay as they are, so that nothing changes where the change was not asked for.
11. As a user, I want pasting an image into the idea box and picking files through the paperclip to both still work and show the same count, upload and error states, so that the icon is a full replacement for the tile.
12. As a user on any device, I want a "Close" text button at the right end of the top bar, after the "Saved" status, so that I can close the sheet with one easy tap.
13. As a user, I want the old "×" gone, so that there are not two close controls.
14. As a user, I want closing to keep the draft and save it first, so that a note I was writing is never lost (the existing behaviour; Escape and a backdrop tap still close the same way).
15. As a user, I want tapping a thumbnail to open the image viewer and closing the viewer to return to the sheet, without starting the design, so that I can look at an image safely.
16. As a user, I want every other button in the sheet (remove image, retry upload, banner actions, discard confirmation, draft switcher and rename) to never submit the form, so that only "Start design" starts a design.
17. As a user, I want autosave to behave exactly as before (saves 800 ms after the last keystroke, retries every 5 s while failing, saves on close), so that this fix does not change when my notes are saved.

Agent chat composer

18. As a tablet user, I want the same paperclip icon in the agent chat composer that phones have, so that both composers follow one rule: phone and tablet get the icon, desktop gets the labelled button.
19. As a phone user, I want the composer to keep working as it does today, so that the shared rule changes nothing for phones.

Design page

20. As a phone user on the design page with the keyboard open, I want the sidebar toggle row, the title block (name, step line, ring) and the attached images hidden, so that the question box fills the space between the top of the screen and the keyboard.
21. As a phone user with the keyboard open and a round open, I want the view tabs (Questions, Chat) to stay with the ⋯ menu beside them, so that I can still switch views and reach the menu. With no round open there are no view tabs, so the ⋯ hides together with the header while the keyboard is open and returns when it closes.
22. As a phone user, I want the layout to come back when I dismiss the keyboard, so that the header and images return. When I open the ⋯ menu from the tabs row during the keyboard-open layout, the page keeps that layout while the menu is open and the menu stays anchored to the tabs-row button, so that tapping ⋯ (which blurs the field) does not move or close the menu. Opening ⋯ from the title row with the keyboard closed does not change the layout.
23. As a user on any device, I want the footer to keep today's order (Add, Note, Auto, then Send on the right) with tighter spacing above and below, and no separator line above it, so that it takes less height.
24. As a user, I want the Add image paperclip to stay in the design page footer, so that the footer change removes no capability.
25. As a user on any device, I want the model and effort as the first row of the menu's Agent block, and an "Open design epic" item, in the ⋯ menu, and no model/epic line under the title, so that the header is shorter and the information is still one tap away.
26. As a user, I want "Open design epic" to open the Design's epic page, so that the link I lose from the meta line is not lost.
27. As a user, I want attached images shown as a thumbnail strip at the top of the chat log, so that they are with the conversation and never above the questions.
28. As a user viewing the Questions view, I want no images above the question box, so that the questions get the room.
29. As a desktop user, I want the images strip to show no scrollbar beside it, so that there is no useless control.
30. As a user, I want the keyboard to count as open only when the visual viewport is at least 150px shorter than the window AND a text field is focused, so that a short window or a zoomed page does not hide the header.
31. As a user pinching to zoom, I want the header to stay put, so that zoom is not mistaken for a keyboard.
32. As a user moving focus from one field to another, I want the layout not to flash back and forth, and as a user who leaves a field I want the chrome back as soon as the keyboard is gone, even if the browser reports the resize late.
33. As a developer, I want a check at iPhone size that keyboard detection hides and restores the chrome, so that this part is not left to a hope.

Answered-question card

34. As a person who just answered an agent's question on the ticket page, I want one compact card instead of bubbles and repeated text, so that I can see what was asked and what I said at a glance.
35. As that person, I want the label "THE AGENT ASKED" (or "GX ASKED" for a gx question), so that the card reads as a record.
36. As that person, I want the agent's detail clamped to three lines with a "Show more" toggle (and "Show less" when open) in place of the "Agent's notes" toggle, so that a long detail does not push everything down; the toggle shows when the text is cut off or the round has a handoff.
37. As that person, I want the agent's question as plain text, so that it is not a bubble.
38. As that person, I want my answer as one inline chip, "You  <answer>", showing the picked option, the free text, the fixed "Huh?" message or "no option picked", and the answer shown once only, so that nothing repeats.
39. As that person, I want an optional note as one muted line under the chip, so that the note is clearly mine.
40. As that person, I want one status line with a state dot: "Delivered · Agent is working", "Queued · #N in queue", or "Sent · The agent had already finished", on one line at phone width, so that I know what happened to my answer.
41. As that person, I want no "SENT" or "Answered" badge and no AGENT or WEB bubbles, so that the card is calm.
42. As a user reloading the page or opening an answered ticket later, I want the same compact card with the label, detail, question, chip and note, and no status line (the delivery outcome is only known right after sending), so that old answers are not shown as cluttered bubbles.
43. As a user, I want the Action buttons above the card and an unanswered question's form to look and work as they do now, so that only the answered card changes.

## Implementation Decisions

- AutoFill: the shared free-text field (ExpandableTextField) takes a stable, non-financial field name and always turns the browser's autocomplete off. The agent chat box is named `chat-message` and the Rough idea box `design-idea`. The other users of the shared field (question answers, design notes, permission and recovery cards) each get their own stable non-financial name. The names of those other users, and whether the field takes a required prop, are the implementer's choice, but no two purposes share a name that is card-like or contact-like. No other attribute is added: no one-time-code, new-password or invalid-value tricks, and autocorrect, autocapitalize, spellcheck and inputmode are left as they are. Aria labels and placeholders stay unchanged and avoid card-like words. The New Design sheet already renders a real form through the bottom sheet, and it keeps doing so; no form-level attribute is added. The research found no source that proves any attribute stops iOS card AutoFill on a textarea, so the work is the attributes plus a manual check; if the suggestion persists on a device, the outcome is a Safari bug report and acceptance, not more tricks.
- Image attach rule: one shared rule decides the image-attach control from the device (phone, tablet or desktop, as the device hook defines them: phone below 640px, tablet 640 to 991px, desktop from 992px). Phone and tablet get an icon-only paperclip button of at least 44px with the label "Add image" as its accessible name and title; desktop keeps the labelled control. Both the agent composer and the New Design sheet use it, instead of each deciding on its own.
- New Design sheet: on phone and tablet the paperclip is the first control of the footer, left of Discard, and a hidden file input serves it. The thumbnail row renders only when the draft has images. The status line renders only when there is a count, an upload in progress or an error. The existing "reason" text above the footer on a phone stays. On desktop the tile and the limits line stay. The footer in its discard-confirmation state is unchanged and does not show the paperclip.
- Close: the "Close" text button is added at the right end of the top bar after the save status, replaces the "×" next to the epic name on every device, keeps the accessible name "Close" and the "keeps the draft" hint, and runs the same close path (save, then close). The slug row loses the "×".
- Submit bug: the cause is a button without an explicit type inside the form. The fix goes where the button lives, so the shared image-strip control (open, retry, remove) is typed as a plain button everywhere it appears, including the composers and the read-only strip, not only in the sheet. Every other non-submit button in the sheet is also typed as a plain button; "Start design" is the only submit.
- Autosave: no change.
- Design page keyboard layout: below desktop width (the same devices that hide the title block today), while the keyboard is open, the page hides the sidebar toggle row, the title block and the attached images. The view tabs stay and carry the ⋯ menu, so the menu stays reachable. When no round is open there are no view tabs: the ⋯ hides with the header while the keyboard is open and returns when it closes. While the ⋯ menu is open, the page keeps the keyboard-open layout and the menu stays anchored to the tabs-row button (opening it blurs the field and closes the keyboard); the normal keyboard signal takes over again when the menu closes. The mock's `app-top-bar` is the mascot toggle ("Show sidebar") that `PageHeader` renders; it already hides when `keyboardOpen && !sideColumn`. Connection and disk strips are alerts and stay as they are. The question box and footer take all remaining height.
- Design page header: the model line and the epic link leave the meta line on every device. The model and effort text the meta line showed becomes the first row of the menu's existing Agent block, on every device (the phone-only Effort row is removed, since the header no longer shows it on any device). Tier and the switched note follow it. An "Open design epic" item that opens the epic page goes ahead of the other items (New session, live sessions, Copy design ID, Delete design, card actions, as they are today). With no session, the Agent block is omitted rather than empty. The ⋯ renders in the tabs row only while the keyboard-open layout shows, and in the title row otherwise. The page renders the dialogs the menu opens (Move, Remove, Hold) outside the menu component, so they survive the ⋯ moving between rows. The keyboard-open layout is held only when the menu was opened from the tabs row during that layout.
- Design page footer: one row in today's order: Add, Note (icon-only on a phone), Auto, Send on the right. The mock's Auto, Note, Send order is incidental, because it shows spacing only. Vertical spacing is reduced to the approved mock's footer padding (6px above, 8px below), and the separator line above the footer is removed. Existing behaviour of the footer controls (Send disabled until an answer exists, the Auto menu, the Queuing countdown strip and its Cancel) is unchanged. The tighter spacing and the missing separator apply on every device, because there is one footer component. The approved mock leaves the paperclip out only because it shows spacing and layout; the decision for option A was "today's labels, tighter spacing only".
- Design page images: the attached-image strip becomes the first entry of the chat log (Chat view only) and is removed from the top of the Chat region, so it never appears above the Questions view. This holds on every device. The strip no longer shows a scrollbar beside it at desktop width.
- Keyboard detection: the keyboard-open signal is true only when the visual viewport is at least 150px shorter than the window, a focused element accepts text (text-like inputs, textareas, editable regions; not buttons, checkboxes or radios), and the page is not pinch-zoomed (visual viewport scale above 1 counts as not open). It re-syncs on focus changes (focus in and out) as well as on viewport resize, because the resize can arrive late on iOS. Moving focus between two fields must not flip the signal through "closed" in a way that flashes the chrome. It stays false where the visual viewport does not exist. No hide-keyboard button is added.
- Answered card: when the latest round has an answer, the ticket page shows the compact card in place of the current reading view, chat bubbles and result block, in this order: label, detail (clamped, with the toggle), question, answer chip, note, status. "Show more" expands the detail and also reveals the agent's notes (the handoff), so it replaces the old "Agent's notes" toggle; there is no other toggle. The toggle shows when the detail is cut off or the round has a handoff. The question text is the round's summary; the detail is the round's body text. The answer is the last web message in `detail.chat`, and the chip shows only that one. The round's `prompt` is dropped when it repeats the question and shown as plain text otherwise. `closed_reason` stays as a muted line under the status. The status line comes from the send outcome that the page already knows (delivered, queued with its place in the queue, adopted) and is shown only while that outcome is known for the displayed question, as today; after a reload the card shows no status line. The state dot has a distinct tone per outcome (running, waiting, done) and the status stays on one line at phone width. The answer chip text rules are unchanged from today. An unanswered question, permission, recovery, task and error cards, and the "Earlier rounds" list, are not changed. "Earlier rounds" stays below as today.
- Fixtures: the delivered and note states use an existing scenario; the queued and adopted states need new fixture scenarios; the attached-images design page needs a new scenario (names are the implementer's choice).

## Testing Decisions

- A good test checks what a person sees or can do (roles, names, text, layout at a width, whether a request was sent), not how it is built. Attribute assertions are the exception for the AutoFill work, because the attribute is the behaviour.
- Vitest (jsdom) is the default seam, as the e2e rules require. It covers: the field attributes (autocomplete off, stable name, label and placeholder unchanged, keyboard attributes not turned off); the device rule for the attach control in both composers (using the device and pointer context overrides); the New Design sheet (paperclip on phone and tablet, tile and line on desktop, row and line only when there is something to show, Close button placement and behaviour, no submit from a thumbnail or viewer Close or any other non-submit button, Start still submits); the keyboard hook with a fake visual viewport (gap alone, focus alone, both, pinch zoom, focus moves, late resize); the design page ⋯ menu content (including staying open and anchored through a focusout and viewport restore, and hiding with the header when no round is open), footer order, and images placement in the chat log versus the Questions view; the answered card for each outcome (including `prompt`, `closed_reason`, several chat exchanges and a handoff with a short detail), with and without a note, with and without a long detail, the "GX" variant, and the no-status reload state.
- Playwright flows are allowed only for what vitest cannot see: real layout at a width, the real server round trip and the real fake agents. The suite has a flow budget, so no new flow is added where an existing flow on the same scenario can take another step. The keyboard check at iPhone size is a step in the existing phone Design page flow that replaces the visual viewport with a short one while a field is focused. Chromium cannot open a real iOS keyboard, so this step proves the page reacts to the signal; only a real iPhone proves iOS reports it.
- Prior art: the existing component tests next to each file (New Design form, composer images, question sheet, answer result, ticket reading and action card), the device and media-context overrides used by the gallery and tests, the phone flows for the New Design sheet and the Design page, and the epics flow that answers questions on the ticket page.
- Manual checks on a real iPhone (Safari, and the home-screen app): no card or contact strip in the chat box, the idea box and the answer, note and permission boxes (with a saved card on the device); the chat box in the large editor; the design page layout with the keyboard open and closed, including pinch zoom.
- The existing flows that assert the old shapes are updated in the same work (the phone New Design sheet flow for "Close above the idea box", the phone Design page flow for the footer spacing and separator (the order and 44px sizes stay), the desktop Design page flow in `design-page.e2e.ts` for the footer order, spacing and separator at 1440px, the Design page text-size flow for the meta line, the Design page ⋯ menu flows for the new items, and the epics flow that reads the "Answer result" status).

## Verification

Flows run against a temporary gx server with the fixture (see the gx-ui-check procedure). Flow numbers are from the e2e flow catalog. Unit tests are the named vitest files' areas, not flows. "Mock states" lines are at the end; a story entry that names a mock state uses its look check and does not repeat it.

1. Run: none (Safari behaviour is not testable in a flow). New: vitest assertion that the chat composer's text box has autocomplete off and the chat name; manual iPhone check (chat box, saved card).
2. New: vitest assertion on the New Design idea box (autocomplete off, the idea name, aria-label "Rough idea" and the placeholder unchanged). Manual iPhone check.
3. New: vitest assertions for each other use of the shared field (question answer, design note, permission card, recovery card): autocomplete off and a stable non-financial name, each different from card-like words. Manual iPhone check for answer and note boxes.
4. New: vitest assertion in the same tests that autocorrect, autocapitalize, spellcheck and inputmode are not turned off by the change (they equal their previous values).
5. New: the vitest tests from stories 1 to 4 are the check; the manual iPhone checklist is written into the epic's verify ticket result. No CLI check.
6. Run: 41 (Phone, New Design sheet, `ready`, 375px) extended: the paperclip is left of Discard; add a 744px step with the same assertion. New: vitest for the device rule.
7. Run: 41 extended: no "Add image" tile and no limits line at 375px and 744px. New: vitest.
8. Run: 41 extended: with an empty draft there is no thumbnail row; after adding an image the row appears. New: vitest.
9. Run: 26 (image journey, "2 of 10 images", held and failed uploads) at desktop width, and 41 extended at 375px: the status line shows only for a count, an upload or an error, and not before. New: vitest for each case.
10. Run: 26 at 1440px: the tile and the limits line are present. New: vitest at desktop.
11. Run: 26 image journey (paste and file pick) and 41 extended (file pick through the paperclip adds a thumbnail with the same count). New: vitest.
12. Run: 41 extended: at 375px and 744px "Close" is a text button at the right end of the top bar, after "Saved"; and at 1440px in 26. New: vitest for placement and name.
13. Run: 41 and 26: no "×" control remains in the sheet. New: vitest.
14. Run: 26 (close and reopen on the same draft). New: vitest that Close saves, then closes, and Escape and a backdrop tap still do the same.
15. Run: 41 extended and 26 (viewer step): tapping a thumbnail opens the viewer, Close in the viewer returns to the sheet, the sheet is still open and no design started (no new epic, no Start request). New: vitest that clicking the thumbnail does not submit.
16. New: vitest that every button in the sheet other than "Start design" has the plain button type (remove, retry, banner actions, discard confirmation, switcher, rename); Run: 26 for discard with an image, Start refused with images kept, and a good Start.
17. Run: 26 (autosave: "Saved" and reload, close and reopen) unchanged. New: none; the existing autosave unit tests must pass unchanged.
18. New: vitest for the agent composer at tablet width (paperclip icon, no labelled button); Run: the agents composer flows (`agents-composer.e2e.ts`) at 744px as one added step on the flow that opens the composer.
19. Run: the same agents composer flows at 390px: unchanged behaviour (sizes 44px, upload still works), and 189 (Send images) and 191 for the grid. New: none.
20. Run: 24 (Phone, Design page, `design`, 390px) extended with the keyboard step; see Mock state `questions-keyboard-a`. New: step that fakes a 300px-shorter visual viewport and focuses the question box.
21. Run: the same step: view tabs and the ⋯ menu are visible with the keyboard open (Mock state `questions-keyboard-a`). New: vitest for the menu staying reachable; vitest that with no round open the ⋯ hides with the header while the keyboard is open and returns after.
22. Run: the same step: after the viewport is restored and focus leaves, the sidebar toggle row, title block and images return. New: vitest; vitest that opens the ⋯ menu from the tabs row, fires focusout and a viewport restore, and expects the menu to stay open and anchored, then return to the normal layout after it closes; vitest that opens Hold from the tabs-row ⋯ and checks the dialog stays when the layout returns; vitest that opening ⋯ from the title row with the keyboard closed does not change the layout.
23. Run: 24 extended; Mock state `chat-closed-a`, `questions-keyboard-a`. New: keep the existing Add, Note, Auto, Send order and size assertions in 24; assert no separator line above the footer and the 6px/8px padding; at 1440px assert only the order, the missing separator and the padding, as a step in the existing desktop Design page flow (the 44px checks stay phone only). Both mock states show an incidental order (Auto, Note, Send): the look check covers spacing and layout only.
24. Run: 24 extended: the footer shows the paperclip at the left of the footer row. New: vitest.
25. Run: 128 (Copy ID, Design), "two live sessions: popover under ⋯", the Design page text-size flow, and the "Design page" header shape flow at 375px and 1440px; Mock state `menu-open`. New: update those flows for the missing meta line and the new menu rows.
26. Run: the Design page ⋯ menu step; click "Open design epic" and expect the epic page URL. New: add this click to the added menu step in 24 and assert the model row text in the Agent block.
27. Run: Mock state `chat-closed-a` on a Design with attached images. New: scenario with a Design that has attached images; flow 26's final "read-only strip in the Design chat" is extended to assert the strip is the first entry of the chat log.
28. Run: Mock state `questions-keyboard-a` and a Design page with a round open and images attached: no strip above the question box. New: vitest that the Questions view renders no strip; the same new scenario.
29. Run: the Design page at 1440px on the new images scenario: the strip has no scrollbar (`overflow` check after an expect on the strip). New: add to the added step.
30. New: vitest for the keyboard hook (gap with focus, gap without focus, small gap with focus).
31. New: vitest with a fake visual viewport whose scale is above 1: the signal stays false.
32. New: vitest for focusin and focusout re-sync, a late resize after focusout, and focus moved between two fields (no closed flash).
33. Run: 24 extended step (above). No CLI check. Manual iPhone check recorded in the verify ticket result.
34. Run: 2 (Epic looks right), steps "agent question: note-only answer" and "agent question: Huh? sends the fixed message", updated to read the new card; Mock states `delivered-phone`, `delivered-desktop`.
35. Run: the same steps; label text. New: vitest for the "GX ASKED" variant.
36. Run: the same steps on a long detail; Mock states `delivered-phone`, `delivered-desktop`. New: vitest that the toggle shows for a cut-off detail or a handoff (also with a short detail) and expands the notes.
37. Run: the same steps; Mock states `delivered-phone`, `delivered-desktop`. New: none.
38. Run: the Huh? step ("Answer result" shows the fixed message once) and the note-only step ("no option picked"); Mock states `note-phone`, `note-desktop`. New: vitest that the answer text appears once.
39. Run: the note-only step; Mock states `note-phone`, `note-desktop`. New: none.
40. Run: Mock states `delivered-*`, `queued-*`, `finished-*`, `note-*`. New: fixture scenarios for the queued and the "agent had already finished" outcomes and an added step in a ticket-page flow for each.
41. Run: the same steps: no SENT or Answered badge, no AGENT or WEB bubble in the card. New: vitest.
42. Run: none. New: vitest only, with a round that has an answer and no sent outcome. No reload in a flow.
43. Run: 2 steps "options, Huh? and Note at both widths" (unanswered form unchanged) and the permission and recovery flows. New: none.

CLI and server verb checks: none. This epic changes only the web UI and the fixture scenarios. The one server-side check: `gx tickets` / answer verbs are unchanged, so no CLI output is compared.

New flows or scenarios to write (these become implementation tickets):

- A fixture scenario for a Design with attached images (stories 27 to 29).
- Fixture scenarios for a ticket answered while its agent is gone (queued outcome with a queue place) and for a ticket whose agent had already finished (adopted outcome) (story 40).
- A keyboard step in flow 24 using a faked visual viewport (stories 20 to 22, 33).
- An added tablet (744px) step in flow 41 and in the agents composer flow (stories 6, 7, 18).
- No new standalone flow is needed. The flow budget stays; every flow above is an added step in an existing flow.

**Mock states**

- `$(gx tickets root)/ui-fixes/mocks/04-design-page-phone-mock.html`, `chat-closed-a`, width 390, scenario: the new Design-with-images scenario, parts: app-top-bar, title-block, more-menu, chat-log, log-images, footer.
- `$(gx tickets root)/ui-fixes/mocks/04-design-page-phone-mock.html`, `questions-keyboard-a`, width 390, scenario `design` (round one up, keyboard faked), parts: view-tabs, more-menu, question-box, footer. The app top bar, title block and images are absent.
- `$(gx tickets root)/ui-fixes/mocks/04-design-page-phone-mock.html`, `menu-open`, width 390, scenario `design`, parts: app-top-bar, title-block, more-menu, menu, menu-model (the Agent block), menu-epic-link.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `delivered-phone`, width 390, scenario `mixed` (ticket demo:mixed-epic/06, answered while the agent is live), parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `delivered-desktop`, width 720, scenario `mixed`, parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `queued-phone`, width 390, scenario: the new queued-outcome scenario, parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `queued-desktop`, width 720, scenario: the new queued-outcome scenario, parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `finished-phone`, width 390, scenario: the new adopted-outcome scenario, parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `finished-desktop`, width 720, scenario: the new adopted-outcome scenario, parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `note-phone`, width 390, scenario `mixed` (answer with a note), parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, note, status.
- `$(gx tickets root)/ui-fixes/mocks/05-answered-card-mock.html`, `note-desktop`, width 720, scenario `mixed` (answer with a note), parts: action-buttons, agent-label, agent-detail, show-more, agent-question, answer, note, status.

## Out of Scope

- New Design autosave timing: it already debounces (800 ms), retries every 5 s and saves on close, and stays as it is.
- The answered state of a Design's questions sheet: it already looks good and was only the reference for the card mocks.
- A hide-keyboard control and any attempt to hide the iOS row above the keyboard: a page cannot do it.
- A story or test for the row above the iPhone keyboard (arrows, Done bar, QuickType): a page cannot hide it and gx adds no control for it.
- Proof that card AutoFill is gone on iOS: a vitest cannot show it, and no attribute is promised to work; a Safari bug report is the fallback.
- The look of an unanswered question, permission, recovery, task and error cards on the ticket page, and the "Earlier rounds" list.
- Desktop changes to the New Design sheet and the composers beyond the Close button.
- Desktop changes to the design page beyond the meta line moving into ⋯, the images placement and scrollbar, and the footer spacing and separator.
- The design UI not rendering a `gx tickets ask` question (filed separately as a follow-up).

## Further Notes

- Sources: the map's decisions 01 to 05, the research notes on iOS AutoFill and the iPhone keyboard row, and the two stored mocks. Where the map said the New Design sheet has no form, it is wrong: it renders a form through the bottom sheet.
- The approved design-page mock does not draw the Add image paperclip; the footer decision (option A) kept today's labels and order and only tightened spacing, so the paperclip stays and the order is unchanged.
- The ticket page card mock shows a "Screen" and "Goto agent" action row above the card; it is unchanged by this work.
