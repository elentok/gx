# Design page redesign

## Problem Statement

The Design page is where a person answers a Design session's question rounds. It is hard to use,
most of all on a phone:

- A new round opens as a docked sheet under the chat. To read a question in full, the person has to
  press ⤢ every round.
- On a phone, Progress takes about 90px above the chat: a segmented bar plus a summary button.
- The Design page has no progress ring, so it does not look like the Epic page.
- The header holds New session, "N live ▾" and Delete design as separate buttons. They crowd the
  name and are easy to press by mistake.
- The note boxes are 3 lines (per question) or a few lines (per round). They do not scroll into view
  when they open, and a long note is hard to edit.
- Moving to another question keeps the old scroll position, so the person lands in the middle of
  the next question.
- Chat text and answer choices are 14px, which is small to read on a phone.

## Solution

A web-only change to the Design page. The server and the TUI do not change.

- A question round opens in the Questions view, which fills the column with the current question.
  This replaces focus mode and its ⤢/⤡ buttons. A "Questions (n of m) | Chat" toggle above the
  column switches views. It shows only while a round is up. Without a round, the page shows the
  chat. Escape in the Questions view shows the chat.
- A progress ring sits left of the Design name on phone and desktop, as on the Epic page. It
  replaces the segmented bar. On a phone the current step line sits under the name, and tapping
  the line or the ring opens a bottom sheet with the steps. The desktop Progress sidebar stays.
- A "⋯" button in the header opens a menu with New session, the live-session list (only with 2 or
  more live sessions) and Delete design (only while it works). On a phone the menu is a bottom
  sheet. On desktop it is a popover under ⋯.
- Both note boxes (per question and per round) are 5 lines, scroll into view when they open, and
  have an expand button that opens the same text in a large modal.
- Moving to another question scrolls the sheet to that question's top.
- Chat messages and answer choices grow from 14px to 16px. Question titles and bodies grow to
  match. Small meta text stays the same.

The approved mock is `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`
(11 states).

## User Stories

### Questions and Chat views

1. As a person designing, I want a new question round to open in the Questions view, so that I can
   read and answer it without an extra tap.
2. As a person designing, I want each new round to open in the Questions view again, even if I was
   reading the chat, so that I never miss a round.
3. As a person designing, I want a "Questions (n of m) | Chat" toggle above the column while a round
   is up, so that the chat is one tap away.
4. As a person designing, I want the toggle's Questions button to show how many questions I have
   answered (n of m), so that I know how far I am without opening the sheet.
5. As a person designing, I want the toggle to be hidden when no round is up, so that the page only
   shows controls that do something.
6. As a person designing, I want the page to show the chat when no round is up, so that I can follow
   the agent's work.
7. As a person designing, I want Escape in the Questions view to show the chat, so that I can leave
   the questions from the keyboard.
8. As a person designing, I want Escape to close an open layer first (Send popover, jump menu, note
   modal, steps sheet, ⋯ menu), so that one Escape never does two things.
9. As a person designing, I want the Chat view during a round to show the docked question sheet
   under the messages, with its fold chevron, so that I can read the chat and answer at once.
10. As a person designing, I want my picks, the current question and my notes to stay when I switch
    between Questions and Chat, so that switching costs nothing.
11. As a person designing, I want Send to show the chat with my sent answers, so that I can see that
    the agent got them.
12. As a person designing on desktop, I want the Questions view to keep the question text at most
    about 820px wide, so that long lines stay readable.
13. As a person designing, I want the ⤢/⤡ focus buttons gone, so that there is one way to switch
    views.

### Progress ring and steps

14. As a person designing, I want a progress ring left of the Design name, so that the page looks
    like the Epic page and shows how far the Design is.
15. As a person designing, I want the ring to read the Design's progress as a percentage, so that I
    see the same number as on the Epic page and the Epics card.
16. As a person designing, I want the ring to be amber while the Design needs me, blue with a comet
    while a session works, grey with no live session, and green when done, so that its color tells
    me what to do.
17. As a person designing on a phone, I want the current step line ("Resolve tickets · 03 Question
    navigation (3 of 5)") under the Design name, so that Progress takes no extra row.
18. As a person designing on a phone, I want a tap on the step line or the ring to open a bottom
    sheet with the eight steps and the map's tickets, so that I can see the whole plan.
19. As a person designing on a phone, I want to close the steps sheet with Close, Escape or a tap on
    the backdrop, so that it is easy to dismiss.
20. As a person designing on a phone, I want the segmented bar and the old summary button gone, so
    that the chat gets the space back.
21. As a person designing on desktop, I want the Progress sidebar to stay, so that the steps stay in
    view on a wide screen.
22. As a person designing, I want "Open implementation epic →" to show at Branch and Done on both
    phone and desktop, so that I can reach the implementation epic.

### Header menu

23. As a person designing, I want a ⋯ button in the Design header, so that rare actions stay out of
    the way.
24. As a person designing, I want New session in the ⋯ menu with the hint "Stop this session and
    start a fresh one", so that I know what it does before I press it. When the shown session has
    exited, the hint reads "Start a fresh session" instead, since nothing is running.
25. As a person designing, I want New session to keep today's behavior (ask before it stops a live
    session, then start a new one), so that I never stop a session by mistake.
26. As a person designing with 2 or more live sessions, I want the ⋯ menu to list them, with a check
    mark on the shown one and a Stop button on the others, so that I can switch to or stop one.
27. As a person designing with one live session, I want no session list in the menu, so that the
    menu stays short.
28. As a person designing, I want Delete design in the ⋯ menu only while it works (before the map is
    charted, with no live session), so that I never see an action that cannot run.
29. As a person designing, I want Delete design to ask before it deletes, with Keep and Delete, so
    that a mis-tap does not delete my Design.
30. As a person designing on a phone, I want the ⋯ menu as a bottom sheet, so that I can reach it
    with one thumb.
31. As a person designing on desktop, I want the ⋯ menu as a popover under ⋯, so that it opens where
    I clicked.
32. As a person designing, I want the New session and "N live ▾" header buttons gone, so that the
    header holds only the ring, the name and ⋯.
33. As a person who has not started a session yet, I want Start design session where it is today,
    so that starting does not move into a menu.
34. As a person designing, I want ⋯ hidden when its menu would be empty, so that it never opens an
    empty menu.

### Note boxes

35. As a person designing, I want the per-question note box to be 5 lines, so that I can see what I
    wrote.
36. As a person designing, I want the per-round note box in the composer to be 5 lines too, so that
    both notes look and work the same.
37. As a person designing, I want a note box to scroll into view when it opens, so that I see it
    without scrolling, even with the phone keyboard up.
38. As a person designing, I want an expand button at the top right of each note box, so that I can
    write a long note in a large modal.
39. As a person designing, I want the modal to edit the same text as the box, saved as I type, with
    Done to close it, so that there is no separate save step and nothing is lost.
40. As a person designing, I want the modal title to name the note ("Note on this round" or the
    question's note), so that I know which note I am editing.
41. As a person designing, I want the composer's message box (when no round is up) to get the same
    5 lines and expand button, since it is the same box, with the modal titled "Message to the
    agent", so that the composer works one way.

### Question navigation

42. As a person designing, I want a move to another question (dots, Prev/Next, swipe, a pick that
    moves to the next question, or a Send popover row) to scroll the sheet to that question's top,
    so that I always start reading at the title.
43. As a person designing, I want this in both the Questions view and the docked sheet in the Chat
    view, so that navigation works the same in both.

### Text size

44. As a person designing, I want chat messages and my own prompts at 16px, so that the chat is easy
    to read on a phone.
45. As a person designing, I want answer choices at 16px, so that I can read and tap them easily.
46. As a person designing, I want the question title at 24px and the body at 17px in the Questions
    view, and the title at 18px in the docked sheet, so that the question text matches the larger
    choices.
47. As a person designing, I want small meta text ("Earlier messages hidden", "n of m answered",
    recommended marks) to stay small, so that the main text stands out.

## Implementation Decisions

- **Web only.** The change touches the Design page modules (the page, the chat log, the question
  sheet, Progress, session controls, the composer) and the shared progress ring if it needs a hook.
  No server route, wire type, CLI verb or TUI change.
- **Views.** The page's focus-mode flag becomes the view (Questions or Chat). A new round key
  (today the effect that docks the sheet on a new round) now sets the view to Questions. A page
  load with a round up also opens Questions. With no round, the view is Chat and the toggle is
  hidden. Send sets the view to Chat, as leaving focus mode does today. The existing capture-phase
  Escape handler, which checks the open layers first, now sets the view to Chat. The note modal,
  the steps sheet and the ⋯ menu join the layer list, so Escape closes them first.
- **Toggle.** The toggle sits above the column. Its Questions button carries the answered count as
  "n of m". It replaces the sheet's ⤢/⤡ buttons. In the Questions view the sheet has no fold
  chevron. In the Chat view the docked sheet keeps it.
- **Ring.** The page uses the Epic page's `pageRing` for a map epic: label is `ring_progress` as a
  percentage, and the tone and comet come from the existing Design tone rules. The ring renders as
  the lead of the header, `md` (64px) on desktop and `sm` (52px) on a phone, like the Epic page's
  page header. The segmented bar leaves the Design page.
- **Phone Progress.** The step line is today's summary line (current step, and the current ticket
  with its position while tickets resolve). The line and the ring are buttons that open the steps
  sheet. The sheet is a bottom sheet named "Design progress steps", with a small ring, the title
  "Progress · N%", Close, and today's step tree. On a phone the implementation epic link moves to
  the top of the chat column, the same place as on desktop.
- **Header menu.** One ⋯ button ("More actions") opens a menu named "Design actions". Items, top to
  bottom: New session with its hint, then "Live sessions · N" with one row per live session (only
  with 2 or more), then Delete design (only when the map is deletable and no session is live).
  New session keeps today's confirm-then-stop flow and state token. Delete design keeps a confirm
  step with Keep and Delete; the confirm's form (dialog, or an inline row in the menu) is the
  implementer's choice. A start failure message stays visible under the header. A phone gets a
  bottom sheet with a grab handle; desktop gets a popover anchored to ⋯.
- **Note box.** One note box component serves the per-question note and the composer's box. It is
  5 rows, scrolls itself into view on open, and has an expand button ("Open the note in a large
  editor"). The modal edits the same value through the same change handler, so the round draft
  keeps saving it. The modal shows "Saved as you type" and a Done button. On a phone it fills the
  screen under the top bar; on desktop it is a centered modal about 860px wide.
- **Scroll to top.** Every index change in the question sheet (a dot, Prev/Next, a swipe that
  settles, a pick, or a Send popover jump) scrolls the sheet's vertical body to the top of the new
  question.
- **Text sizes.** Chat messages and prompts go from `sm` to `md` (16px). Choices go to 16px.
  Questions view: title 24px, body 17px. Docked sheet: title 18px. Meta text keeps its size. Use
  theme tokens; add a token if a size has none.
- **Send button.** It stays the green main action and keeps the label "Send". The mock's "Send
  answers" label is not a decision.
- **Implementer's choice.** Component names and file split, the state names for the view, the
  `data-testid`s, how the ⋯ menu is built on Chakra, the Delete confirm's form, and the token names
  for the new text sizes.

## Testing Decisions

- A good test checks what the person sees and does: roles, names, visible text, and computed sizes.
  It does not check component state or internal props.
- Seams, highest first:
  - The web e2e flows against a `gxfixture` server, for the views, the menu, the ring tones and the
    phone sheets.
  - `DesignPage` rendered with injected `DesignDeps` and a fake connection, for view switching on a
    new round, Escape order, menu contents per session count, and Delete design visibility.
  - The note box and the question sheet as components, for rows, the expand modal and scroll to top.
- Prior art: `DesignPage.test.tsx`, `QuestionSheet.test.tsx`, `SessionControls.test.tsx` (the
  live-session menu and Stop), `Composer.test.tsx`, and the e2e files `design.e2e.ts` and
  `phone.e2e.ts`. The Epic page's ring tests are prior art for ring tones.
- `SessionControls.test.tsx` tests for "2 live" and the header New session move to the ⋯ menu.

## Verification

Flows that need rewriting, because they assume the old layout:

- Flow 4: the first message ("Two questions before the spec.") arrives with round one, so the
  Questions view hides it. Check it after tapping Chat.
- Flow 13: the defaulted questions' plain text is in the chat. Check it in the Chat view.
- Flow 24 (Phone, Design page): replace the Progress summary steps with the step line and the ring,
  and ⤢/⤡ with the toggle.
- Flow 43: New session is in the ⋯ menu.
- Flow 45: Delete design is in the ⋯ menu.
- Flow 47: fold the sheet in the Chat view. "Click the chat" becomes a click outside the popover in
  the Questions view.
- Flow 48: the note's `rows` is 5. "Enter with ⤢" becomes the Questions view.
- Flow 49 (Focus mode): rewrite as the Questions and Chat views flow (story 1 to 13).

New scenario to write: `design-two-live`, the `design` scenario plus a second live session linked
to `design-epic` (for example `gx agents create` then `gx agents link demo:design-epic`; the flow
ticket confirms the route). It backs stories 26 and 27 and two Mock states.

Story entries:

1. Rewritten flow 49 on `design`: after start, the toggle reads Questions pressed and the sheet
   fills the column. Mock states phone-focus and desktop-focus.
2. Rewritten flow 49: tap Chat, Send round one, Continue; the `Q9` round opens with Questions
   pressed. Flow 5 still passes.
3. Rewritten flow 49: Chat then Questions switch the view. Mock state phone-chat-round-docked.
4. Rewritten flow 49: the Questions button reads "0 of 2", then "1 of 2" after picking SQLite.
5. Rewritten flow 49 on `design-working`: no "View" group. Mock state phone-chat-no-round.
6. Flow 14 on `design-working`: the working row shows with no toggle. Mock state
   desktop-chat-no-round.
7. Rewritten flow 49: Escape in the Questions view shows the chat.
8. Rewritten flow 49: with the Send popover, the jump menu, the note modal and the ⋯ menu each
   open, and with the steps sheet open at 375px, the first Escape closes only the layer and the
   second shows the chat. Flow 47 step 4.
9. Rewritten flow 49: in the Chat view the docked sheet shows under the messages, and its fold
   chevron folds and opens it. Mock state phone-chat-round-docked.
10. Rewritten flow 49: SQLite, `Q8b` as the current dot and a composer note survive two switches.
11. Flows 5 and 10: after Send the Chat view shows the answers as the person's message.
12. New check in rewritten flow 49 at 1440px: the question title is at most 820px wide. Mock state
    desktop-focus.
13. Rewritten flow 49: no "Enter focus mode" or "Leave focus mode" button exists.
14. New flow "Design ring" on `design`: a ring with a Design badge sits left of the name at 390px
    and 1440px. Mock states phone-focus and desktop-focus.
15. New flow "Design ring": the ring label matches the `ring_progress` percentage that the Epic
    page shows for `demo:design-epic` (flow 50).
16. New flow "Design ring": before start `data-tone="muted"`; after start `running` with the comet;
    with round one up `waiting`. Flow 9 at `done`: `done`.
17. Rewritten flow 24 at 375px: the step line under the name reads "Resolve tickets · …". Mock
    state phone-focus.
18. Rewritten flow 24: the step line opens "Design progress steps" with Resolve tickets current;
    the ring opens it too. Mock state phone-progress-sheet.
19. Rewritten flow 24: Close, Escape and a backdrop tap each close the sheet.
20. Rewritten flow 24: no "Design progress" segmented bar on the page.
21. Flow 9 at 1440px: the Progress region still shows each step in turn.
22. Flow 9 at 375px and 1440px: "Open implementation epic →" shows only at Branch and Done, at the
    top of the chat column.
23. New flow "Design actions menu" on `design`: a "More actions" button sits in the header. Mock
    states phone-overflow-menu and desktop-chat-round-docked-menu.
24. New flow "Design actions menu": the menu has "New session" with the hint "Stop this session and
    start a fresh one".
25. Flow 43 through the ⋯ menu. On a live session, New session asks first; Cancel starts nothing;
    OK makes `gx agents list` show a new session and the old one exited. On the stopped Design,
    the ⋯ item reads "New session" with the hint "Start a fresh session".
26. New flow "Design actions menu" on `design-two-live`: "Live sessions · 2", a check mark on the
    shown one, Stop on the other. Picking the other row shows it; Stop makes `gx agents list` show
    it exited. Mock states phone-overflow-menu and desktop-chat-round-docked-menu.
27. New flow "Design actions menu" on `design`: with one live session, no "Live sessions" heading.
28. Flow 45 through the ⋯ menu: Delete design appears with no reload after the stop; `design-epic`
    (charted) lists no Delete design.
29. Flow 45: Keep returns with Delete design still listed; Delete is disabled while the call runs.
30. New flow "Design actions menu" at 375px: the menu is a bottom sheet with a grab handle. Mock
    state phone-overflow-menu.
31. New flow "Design actions menu" at 1440px: the menu is a popover under ⋯. Mock state
    desktop-chat-round-docked-menu.
32. New flow "Design actions menu": no "New session" or "N live" button outside the menu.
33. Flow 4: before start, "Start design session" shows in the header and is not in the menu.
34. New flow "Design actions menu" on `design` before start (charted, no session): no "More
    actions" button.
35. Flow 48: the per-question note's `rows` is 5. Mock state phone-note-box-open.
36. New flow "Note box" on `design`: the composer note's `rows` is 5.
37. New flow "Note box" at 375px: opening the `Q8` note puts the textarea inside the viewport.
    Mock state phone-note-box-open.
38. New flow "Note box": each note box has "Open the note in a large editor". Mock states
    phone-note-modal and desktop-note-modal.
39. New flow "Note box": text typed in the modal shows in the box after Done, and Send includes it.
    Flow 11 step 2 still passes.
40. New flow "Note box": the round note's modal is titled "Note on this round".
41. New flow "Note box" on `design-working`: the composer's message box has 5 rows and the expand
    button, and its modal is titled "Message to the agent".
42. New flow "Question top" on `design` at 1280x600: scroll `Q8` to its end, then move to `Q8b` by
    dot, Next, a pick and a Send popover row; each time the `Q8b` title is at the top of the sheet.
43. New flow "Question top": the same checks in the Chat view's docked sheet.
44. New flow "Text size" on `design`: a chat message and a prompt have a computed font size of
    16px.
45. New flow "Text size": a choice has a computed font size of 16px.
46. New flow "Text size": in the Questions view the title is 24px and the body 17px; in the docked
    sheet the title is 18px.
47. New flow "Text size": "Earlier messages hidden" and "n of m answered" keep today's size.

No CLI or server verb checks: the epic changes no CLI or server behavior.

**Mock states**

- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-focus, 390px,
  `design`, parts: header, ring, progress-row, overflow-button, view-toggle, question-sheet,
  note-button, composer.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`,
  phone-chat-round-docked, 390px, `design`, parts: header, ring, progress-row, overflow-button,
  view-toggle, chat, question-sheet, note-button, composer.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-chat-no-round,
  390px, `design-working`, parts: header, ring, progress-row, overflow-button, chat, composer.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-progress-sheet,
  390px, `design`, parts: header, ring, progress-row, overflow-button, view-toggle,
  question-sheet, note-button, composer, progress-sheet.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-overflow-menu,
  390px, `design-two-live` (new), parts: header, ring, progress-row, overflow-button, view-toggle,
  question-sheet, note-button, composer, overflow-menu, live-session-list.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-note-box-open,
  390px, `design`, parts: header, ring, progress-row, overflow-button, view-toggle,
  question-sheet, note-button, note-box, composer.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, phone-note-modal,
  390px, `design`, parts: header, ring, progress-row, overflow-button, view-toggle,
  question-sheet, note-button, composer, note-modal.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, desktop-focus, 1440px,
  `design`, parts: header, ring, overflow-button, view-toggle, question-sheet, note-button,
  composer, progress-sidebar.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, desktop-chat-no-round,
  1440px, `design-working`, parts: header, ring, overflow-button, chat, composer,
  progress-sidebar.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`,
  desktop-chat-round-docked-menu, 1440px, `design-two-live` (new), parts: header, ring,
  overflow-button, view-toggle, chat, question-sheet, composer, progress-sidebar, overflow-menu,
  live-session-list.
- `$(gx tickets root)/design-page-redesign/mocks/01-design-page-mock.html`, desktop-note-modal,
  1440px, `design`, parts: header, ring, overflow-button, view-toggle, question-sheet,
  note-button, composer, progress-sidebar, note-modal.

## Out of Scope

- Server, wire type, CLI and TUI changes.
- The Epic page, the Epics page, the New Design form and every other page.
- How New session behaves while a session is live (it keeps today's confirm and stop).

## Further Notes

- The mock's text (questions, chat lines, note text) is sample content, not decisions.
- The map: `gx:design-page-redesign`. Decision 01 approved the mock.
- Two calls this spec makes by default, flag them in review if wrong: the phone's implementation
  epic link moves to the top of the chat column (parity with desktop), and ⋯ hides when its menu
  would be empty.
