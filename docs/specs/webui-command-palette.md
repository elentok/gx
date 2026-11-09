## Problem Statement

A person using the web UI has to find the right page and the right button to start common actions. Starting a Design is only possible from the Epics page, starting an agent only from the Agents page, and there is no quick way to reach the next card waiting on them. The Cmd+K list only jumps to existing epics and sessions. Its filter is plain substring, so a person has to type exact fragments.

## Solution

A **Command palette** on every page of the web UI: a centered, filterable list of actions, opened from the keyboard with Cmd+E on a Mac and Ctrl+E on every other platform. It looks and behaves like the Cmd+K list and reuses it. Typing narrows the list with a fuzzy match. Running a command closes the palette and does the action. Two of the commands, New design and New chat, open their form over whatever page the person is on, and the app goes to the new item only once it exists. A third, Next question, jumps to the next card in the Needs you section. The palette runs actions and never searches epics or sessions; Cmd+K stays for that. The fuzzy matcher is shared, so the Cmd+K list becomes fuzzy too.

## User Stories

1. As a person using the web UI, I want to press Cmd+E (Mac) or Ctrl+E (elsewhere) on any page, so that I can reach an action without hunting for the page that holds its button.
2. As a Mac user, I want only Cmd+E to open the palette, so that Ctrl+E keeps moving to the end of the line in text boxes.
3. As a Windows or Linux user, I want only Ctrl+E to open the palette, so that the shortcut matches my platform.
4. As a person typing in a text box, I want the shortcut to work there too, so that I do not have to leave the box first.
5. As a person with the palette open, I want to press the shortcut again to close it, so that I can dismiss it the way I opened it.
6. As a person with another modal open, I want the shortcut to do nothing, so that the palette does not stack over my work, same as Cmd+K.
7. As a person, I want the palette centered over the page with a filter box focused, so that I can type at once.
8. As a person, I want the list to hold New design, New chat, Next question, Go to Epics, Go to Agents, Go to Stats, Go to Settings and Toggle AFK, so that the common actions are in one place.
9. As a person, I want the rows in a fixed order while the filter is empty, so that I learn where each command sits.
10. As a person, I want typing to narrow the list with a fuzzy match, so that "nd" or "newdes" finds New design.
11. As a person, I want best matches first once I type, so that Enter picks what I most likely mean.
12. As a person, I want Up, Down and Enter to move and run a row exactly as in the Cmd+K list, so that I need to learn nothing new.
13. As a person, I want Escape or a click outside the palette to close it without running anything, so that I can back out.
14. As a person, I want to choose New design and get the New design form over my current page, so that I can start a Design without going to the Epics page.
15. As a person, I want New design to pre-select the project of the epic, Design or session page I am on when it opens a new blank draft, so that I start in the project I am working in. A restored draft keeps its own project.
16. As a person on any other page, I want New design to keep today's project rule, so that behaviour there does not change. While the Epics page is open, every opener (page button, "+ New", palette) uses the page's single-project filter.
17. As a person, I want the app to go to the new Design only after it is created, so that closing or discarding the form leaves me where I was.
18. As a person, I want to choose New chat and get the new-chat form over my current page, so that I can start a session from anywhere.
19. As a person, I want New chat to pre-select the project of the page I am on in the same way (blank drafts only), so that both forms behave alike.
20. As a person, I want the app to go to the new session only after it is created, so that cancelling leaves me where I was.
21. As a person, I want Next question to take me to a card in the Needs you section of the active workspace, so that I can answer what is waiting without scanning the Epics page.
22. As a person, I want Needs you cards ordered newest first for this command, so that the question that just arrived comes first.
23. As a person on the page a Needs you card opens, I want Next question to go to the card after it and wrap at the end, so that repeated use walks through every waiting card. The card on screen is found by the page each card's link opens, for every card kind, including the triage page of a Follow-ups card.
24. As a person on any other page, I want Next question to go to the newest card, so that I always start from the freshest.
25. As a person, I want Next question to open the right place for the card (the Answer link for a question, the card's own page otherwise), so that I land where I can act.
26. As a person, I want Next question to cover every kind of Needs you card (permission prompts, paused tickets, blocked merges, epics awaiting merge, deadlocks, Design questions, Follow-ups reviews and one-off cards), so that nothing waiting on me is skipped.
27. As a person, I want typing "needs you" to find Next question, so that I can find it by the section's name.
28. As a person, I want the Next question row to show the target card and how many are waiting, so that I know where Enter will take me.
29. As a person with nothing in Needs you, I want the Next question row to stay and say "Nothing needs you", with Enter doing nothing, so that the list does not shift and I get a clear answer.
30. As a person, I want Go to Epics, Go to Agents, Go to Stats and Go to Settings to navigate to those pages, so that I can move around by keyboard.
31. As a person, I want Toggle AFK to flip AFK mode as the AFK switch in the gear menu does, so that I can switch it without reaching for the mouse.
32. As a person on a narrow window or phone with a keyboard, I want the shortcut to work at any width, with the palette in the full-screen layout the Cmd+K list uses there, so that it is usable on a small screen.
33. As a person, I want the Cmd+K list to use the same fuzzy filter as the palette, so that the lists in the app filter the same way.
34. As a person, I want Cmd+K to keep searching epics and sessions only, so that the two lists stay distinct.

## Implementation Decisions

- The palette is a second use of the shared filterable row list that already powers the Cmd+K list. It gets its own command rows but not its own list component. Rows, arrow and Enter handling, centered popover and full-screen narrow layout are shared. The palette is a sibling of the Cmd+K palette in the app shell, with its own open state. Escape and click-outside handling is extracted once from the Cmd+K palette and used by both, and both use the same layer guard, so Cmd+E and Cmd+K never stack in either order.
- One fuzzy matcher replaces the plain substring filter in the shared list, so every user of the list (Cmd+K list, palette) becomes fuzzy. Matching is case-insensitive; with a filter, sections stay and rows within a section are ordered best match first; with an empty filter, order is the fixed order given by the command list. The algorithm and scoring are the implementer's choice.
- There is one command list in code. No command is page-specific. Each command may match on extra words (Next question also matches "needs you"); the implementer picks how those aliases are stored.
- The shortcut is registered once at the app level so it works on every page. Cmd on Mac and Ctrl elsewhere, using the existing platform check; the other modifier is ignored. It works while a text box has focus, a second press closes, and it does nothing while another modal is open, the same rule as Cmd+K.
- The sidebar footer already renders both forms from its "+ New" menu and navigates only after create. Its dialog state is lifted into one shared app-level store. The palette, "+ New", the Epics page button and the Agents page button all open it. The page-level copies of the two forms on the Epics and Agents pages are deleted, so there is one instance of each. The Epics page's project filter becomes an input to the shared opener. The new-session form is labelled "New chat" in the palette, matching "+ New"; the Agents page button keeps its own label. The palette row has no aliases. Form behaviours, such as drafts, are unchanged.
- After either form creates its item, the app navigates to it. Closing or discarding the form leaves the current page.
- Project prefill: on an epic, Design or session page a new blank draft starts with that page's project, through the forms' existing prefill and filter inputs (no new props). A restored draft keeps its own project. On any other page the form keeps its current rule.
- Next question reads the Needs you cards of the active workspace. Epic and Design cards count when their workspace is the active one; Follow-ups cards count through the same workspace check the Epics page uses; One-off cards always count, because they belong to no workspace. Follow-ups cards have no waiting-since time and sort last (the server's order reversed). It sorts them by the card's existing waiting-since time, newest first (the server's own order, oldest first, is unchanged; no server change). It takes the card after the one on screen and wraps, or the newest when the current page is not a Needs you card's page. The card on screen is the card whose link opens the current page, for every card kind (epic, Design, one-off, and the triage page for Follow-ups). It reuses the existing function that picks the right link for a card, including the Answer link. Its subtitle names the target card and the active workspace's waiting count. Cards of other workspaces are not walked, and the count can differ from the sidebar footer badge, which counts every workspace. The empty case is described in the stories.
- Go to commands use the app's normal navigation. Toggle AFK and the AFK switch in the gear menu share one toggle path (the unused toggle helper is either used by the gear menu or deleted).
- Names of the rows are fixed as written in the stories. Internal ids and the visible placeholder text of the filter box are the implementer's choice.
- Nothing on screen shows the shortcut in this version: no top bar button, no hint text, no phone entry point.

## Testing Decisions

- A good test drives the palette as a person does (press the shortcut, type, press Enter) and asserts what appears and where the app goes, not the internal structure of the matcher or list.
- Vitest component tests are the main seam. The existing tests for the Cmd+K list and for the New session form on the Agents page are the prior art. They cover: the shortcut per platform and its guards (text box, second press, another modal open); the command rows and their fixed order; fuzzy filtering and ranking, including the Cmd+K list using it; each command's effect with stubbed navigation; Next question for newest-first order, the row after the current epic, wrap, other pages, each Needs you card kind (including a Follow-ups card on the triage page), the active-workspace limit, subtitle and the empty state; form prefill rules, including a restored draft; and navigation only after create. The existing form tests move with the forms to the app level.
- The matcher is a pure function and gets plain unit tests through the list's behaviour where possible.
- One Playwright step, added to the existing jump flow, which already opens the Cmd/Ctrl+K list on the Epics and Agents pages. It covers what vitest cannot see: the shortcut opening the palette on several pages, the shortcut in the Design page chat box, a second press closing it, the filter box focus and centering, the eight rows by name, Next question landing on a Needs you card, the gear menu AFK switch flipping after Toggle AFK, and the palette at the full-screen layout at 390px, all on the `united` scenario. No new flow is added; the flow budget stays as it is. If a step cannot be added there, the CATALOG and the budget guard are updated in the same ticket.
- Toggle AFK and navigation commands are covered in vitest; no server rule changes, so no Go test is needed.
## Verification

1. Story 1: run the jump flow with its new step; press the shortcut on the Epics, Epic, Agents, Stats and Settings pages of a fixture server and see the palette. Vitest check that Cmd+E and Cmd+K never stack in either order.
2. Story 2: vitest check of the Mac case (Ctrl+E does nothing, Cmd+E opens); no flow.
3. Story 3: vitest check of the non-Mac case; the jump flow step runs on the fixture's Linux/Chromium default with Ctrl+E.
4. Story 4: vitest check with a text box focused; also press the shortcut in the Design page chat box in the jump flow step.
5. Story 5: press the shortcut twice; the palette closes (vitest; also in the jump flow step).
6. Story 6: vitest check with another modal open.
7. Story 7: jump flow step; the filter box has focus and the list is centered.
8. Story 8: the jump flow step lists the eight rows by name.
9. Story 9: vitest check of row order with an empty filter.
10. Story 10: vitest check of fuzzy queries.
11. Story 11: vitest check of ordering by match quality.
12. Story 12: vitest check of Up, Down and Enter.
13. Story 13: vitest check of Escape and of a click outside.
14. Story 14: vitest check that New design opens the form over the page and the page stays; the existing `26. New Design form` flow still passes with the form at app level, and the page-level copies are gone.
15. Story 15: vitest check of prefill on an epic, Design and session page, and that a restored draft keeps its project.
16. Story 16: vitest check of the unchanged rule on other pages, and of every opener using the page filter on the Epics page.
17. Story 17: vitest check that navigation follows creation only; closing leaves the page.
18. Story 18: vitest check that New chat opens the form over the page; the existing `85. Create session` flow still passes.
19. Story 19: vitest check of prefill, blank draft only.
20. Story 20: vitest check that navigation follows creation only.
21. Story 21: the jump flow step on the `united` scenario (which holds Needs you cards): Next question lands on a Needs you card of the active workspace; vitest check that cards of other workspaces are skipped.
22. Story 22: vitest check of newest-first order.
23. Story 23: vitest check of the next row and wrap, including from a Design, a one-off and a Follow-ups triage page.
24. Story 24: vitest check from a non-epic page.
25. Story 25: vitest check that a question card opens its Answer link and another card opens its page.
26. Story 26: vitest check over one card of each kind, including Design, Follow-ups and one-off cards.
27. Story 27: vitest check that "needs you" finds Next question.
28. Story 28: vitest check of the subtitle, including a One-off and a Follow-ups card.
29. Story 29: vitest check of the empty state and Enter doing nothing.
30. Story 30: vitest check of the four Go to commands.
31. Story 31: vitest check that the shared toggle is called; in the jump flow step open the gear menu and see the AFK switch flip.
32. Story 32: the jump flow step at 390px shows the full-screen layout; existing modes-flow coverage of the Cmd+K sheet still passes.
33. Story 33: vitest check that the Cmd+K list filters fuzzily; existing jump-flow coverage still passes.
34. Story 34: vitest check that the palette holds no epic or session rows and Cmd+K holds no commands.

No CLI or server verb changes. New flows or scenarios to write: none beyond the one added step.

## Out of Scope

- The TUI.
- Page-specific commands; there is one list.
- A Restart server command.
- A history of recent commands.
- Searching epics or sessions from the palette; Cmd+K stays separate.
- A phone entry point or top bar button; the palette is keyboard only.
- Any on-screen hint of the shortcut in this version.
- Changing the server's Needs you order on the Epics page.

## Further Notes

- The shortcut is Cmd+E on Mac only and Ctrl+E elsewhere only, on purpose: Ctrl+E is end-of-line in Mac text boxes. Cmd+K keeps accepting Cmd or Ctrl on every platform.
- The glossary entry "Command palette" already defines the term; the jump menu stays a separate term. The entry is edited to drop the session switcher, which no longer exists.
