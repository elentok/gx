# Nav fixes 2: tree rows, footer, New dialogs and the sidebar gesture

## Problem Statement

Seven rough edges in the web UI's sidebar and New dialogs bother me every day (web UI only).

- Agents nested under an epic do not look nested. They are indented a flat 40px, their text is a different size, their status words sit further right than the parent's, and they have no "..." menu, so I must open the agent page to Stop or Remove it.
- A chat row and an epic row do not line up. The chat's icon is 26px and the epic's ring is 20px, so their titles start at different places.
- In the sidebar footer the Epics and Follow-ups icons sit at different offsets and their count badges differ in size and position.
- The footer's settings button shows a sun-like icon. A burger is the icon I expect for a menu.
- Opening New workspace, New chat or New design leaves the keyboard focus nowhere, so I click into the form before typing.
- New chat always starts on the first project in the list, not the project I am working in.
- On an iPhone or iPad the left-edge swipe that opens the sidebar collides with the installed web app's back swipe, and in a Safari tab it never worked. I still want to open the sidebar quickly with my thumb.

## Solution

- Nested agent rows read as children: no icon, the title starts exactly where the parent's title starts, a thin guide line runs down under the parent's icon column, and the status words and "..." menu sit in the same columns as the parent's. The menu holds the Agent block, Stop (live sessions) and Remove, the same as the agent page.
- Chat and epic rows put their icons in one fixed-width slot, so every title starts at the same x.
- Every footer icon sits in one fixed-size box, and the Epics and Follow-ups badges share one style and position.
- The settings button is a burger (three horizontal lines). The menu items do not change.
- On tablet and desktop, opening New workspace, New chat or New design focuses the first text field. Phones keep focus on the dialog panel, so the on-screen keyboard stays closed.
- New chat and New design start on the project of the page I am on (for a blank form; a draft with text keeps its project), then the Epics page's one-project filter, then the last project I used, then the first project.
- A swipe right that starts anywhere except the left edge opens the sliding sidebar, in a browser tab and in the installed app. The question cards on a Design never trigger it. The left-edge open swipe is gone.

## User Stories

Tree rows

1. As a person, I want an agent nested under an epic to have no icon and a title that starts at the parent's title x, so that it reads as the epic's child.
2. As a person, I want a thin vertical guide line under the parent's icon column beside the nested rows, so that I see which rows belong to which epic.
3. As a person, I want nested titles at 13px while the parent's stay 14px, so that the hierarchy is visible in the type as well.
4. As a person, I want a nested row's status words and "..." menu in the same columns as the parent's, so that the right edge of the list is one clean column.
5. As a person, I want a "..." menu on a nested agent row with the Agent block (tier), Stop while the session is live and Remove, so that I manage an agent without opening its page.
6. As a person, I want that menu to hold the agent page's items for a session I manage (same items, same Remove confirmation), so that the two never differ.
7. As a person, I want removing the open agent from its row to take me to the agents list, so that I never sit on a dead page, as for a chat row.
8. As a person, I want an agent the gx queue drives (a queue session, which cannot be stopped or removed by hand) to show no menu button, so that there is no empty menu.
9. As a person on a mouse or keyboard, I want a row's menu button to appear on that row's hover or focus, and only that row's, so that hovering a nested row never shows its parent's button.
10. As a person on a touch screen, I want every row's menu button always shown, nested or not, so that I can tap it without a hover.
11. As a person, I want a Fix agent nested under an epic to keep its 18px wrench in the icon column with its title at the same x as other nested titles, so that wrenches do not shift the list.
12. As a person, I want the Other section's rows to use the same nested look, menu included, so that all agents look alike.
13. As a person, I want the current nested row to show the same current tint and left edge as a top-level row, so that I see where I am.
14. As a person, I want long nested names to truncate with an ellipsis before the status words, so that the columns never move.

Icon column

15. As a person, I want chat and epic icons in one fixed-width slot, so that a chat's title and an epic's title start at the same x.
16. As a person, I want the epic ring's kind dot and the chat bubble to stay centered in that slot, so that neither looks pushed aside.

Footer

17. As a person, I want every footer icon (Epics, Follow-ups, Stats, the settings button and the optional warning, paused and offline indicators) in one fixed-size box that never shrinks, so that the icons sit evenly.
18. As a person, I want the Epics and Follow-ups badges to share one size, one font and one corner position, so that they read as the same kind of thing.
19. As a person, I want the badge colour to keep meaning (amber when something needs me, blue for open follow-ups with none waiting), so that nothing I rely on changes.
20. As a person, I want the whole footer to fit the 280px desktop sidebar without squeezing any box, wrapping the warning, paused and offline indicators onto a second line above the row when they do not fit, so that nothing misaligns at that width.

Settings button

21. As a person, I want a burger icon (three horizontal lines in the footer's 20px line style) on the settings button, so that it looks like a menu.
22. As a person, I want the same menu items behind it, so that nothing else changes.

New dialogs

23. As a person on tablet or desktop, I want New workspace to focus its Name field when it opens, so that I type at once.
24. As a person on tablet or desktop, I want New chat to focus its Name field (not the Project select and not First prompt), so that I type at once.
25. As a person on tablet or desktop, I want New design to focus its Epic slug field, so that I type at once.
26. As a person, I want this to work from every entry point (the sidebar "+ New" menu, the workspace switcher's "New workspace...", the Epics page button, the Agents page button), so that it does not depend on where I clicked.
27. As a phone person, I want focus to stay on the dialog panel, so that the on-screen keyboard does not pop up.
28. As a person using the keyboard, I want the first Tab to stay inside the dialog, as today, so that nothing behind it takes focus.

Default project

29. As a person on an epic, Design, ticket or chat page, I want New chat and New design to start on that page's project, so that I create in the project I am in. The page project applies to a blank form only; a draft with text keeps its own project (story 33).
30. As a person on another page, I want them to start on the Epics page's project filter when it picks exactly one project, so that my filter guides new work.
31. As a person with no page project and no one-project filter, I want them to start on the last project I used, then the first project, so that the form is never empty.
32. As a person, I want New chat and New design to follow one rule, so that they never disagree.
33. As a person, I want a draft with my own text in it to keep its project, so that I never lose work in progress.

Sidebar gesture

34. As a phone or tablet person, I want a swipe right that starts anywhere except the left edge (the first ~30px) to open the sidebar, so that I open it with my thumb.
35. As a person using the installed web app, I want the same gesture, and the left edge left alone, so that the system's back swipe still works.
36. As a person in a browser tab, I want the same gesture, so that the app behaves the same in both.
37. As a person, I want a touch that starts inside a horizontal scroller to never open the sidebar, even when that scroller cannot scroll that way, so that I never open it while scrolling a code block or a row of cards.
38. As a person answering a Design's questions, I want a swipe on the question cards, including the first card, to move between questions and never open the sidebar.
39. As a person, I want a swipe left on the open sidebar or its scrim to keep closing it, so that closing is unchanged.
40. As a person, I want a tap, a vertical scroll or a movement under 40px to never open or close the sidebar, so that nothing happens by accident.
41. As a person, I want the gesture to do nothing while a dialog or the jump menu is open, so that a swipe in a dialog stays in the dialog.
42. As a desktop person with the sidebar docked, I want no swipe at all, so that nothing changes there.

## Implementation Decisions

**Vocabulary.** Terms are in GLOSSARY.md: App shell, Chat session, Fix agent, Queue agent, Other section, Tray, Device type, Active workspace. The "gear menu" of the earlier specs is now the settings menu with a burger button; its accessible name stays "Menu". Nested agent rows are the agent rows under an epic row and in the Other section. A queue session is a Queue agent's session. No GLOSSARY entry is added.

**Reversals.** This reverses better-nav-fixes story 12 (nested rows stay without a menu) and stories 47 to 50 where they open the sidebar by a left-edge swipe (story 47's left swipe to close and story 50's 40px rule stay). The earlier specs are not edited.

**Tree rows (approved mock).**
- Layout follows the stored mock. Each row is a chevron column, one 26px icon column, the title, the status words and a menu slot. A nested row has an empty chevron cell and no icon, but keeps the icon column's width as a spacer, so its title starts at the parent's title x. A Fix agent puts its 18px wrench in that column instead of the spacer.
- The guide line is 1.5px, in the border colour, drawn under the parent's icon column for the full height of the nested rows; the last nested row's line stops at its vertical middle.
- Nested titles are 13px, parent titles 14px. Status words and the menu slot use the parent's columns.
- Menu: the Agent block, Stop (live only) and Remove, built by the agent page's own menu pieces (`SessionActions` over `useSessionManage`), not copied. It carries no Promote item and no Move or Hold. A session the person cannot manage (a queue session) renders no menu button; `canManage` only gates Stop and Remove inside `useSessionManage`, and `SessionActions` still renders an Agent-block-only menu for a queue session that has a tier. So the nested row renders `SessionActions` only when `canManage(session)` holds, and a queue session shows no menu even when it has a tier.
- The sidebar model must carry the agent's session on each nested agent so the row can build the menu. The Other section gets the same data.
- Hover and focus: the menu button shows on its own row's hover or keyboard focus, and always when the device has no hover. Today every row is wrapped in one hover group that also contains its nested rows, so hovering a nested row would show the parent's button and the reverse. Each row, nested or not, is its own hover group. This is the cause fix; do not stack a second selector on top of it.
- Alt numbers apply to own rows only. A nested row's menu slot never shows a number. The slot component is shared by both kinds of row.
- Removing the open agent from a nested row navigates to the agents list, as chat rows do.
- Keyboard navigation of rows is unchanged. The nested menu button is as reachable as a top-level one.

**Icon column.** One fixed 26px slot holds the chat bubble, the epic or Design ring with its kind dot, and the wrench's spacer. The slot centers its content, so nothing sets a width on the icon itself. The ring's size and the chat glyph's size are unchanged.

**Footer.**
- Today the footer's icon buttons are declared 34px wide but shrink (they are flex children that can shrink) to 22px at the 280px desktop sidebar, so icons land at different offsets. The Epics badge is 20px by 18px at the corner, the Follow-ups badge 18px by 18px offset outward. Fix the cause: one footer button component serves the link and the button variants, with a fixed-size box that does not shrink and the badge as a slot inside it. Both badges use it, so they share size, font and corner by construction. The tone logic stays per button.
- At 280px the row (+ New, four icon boxes, the health tray, the limits tray, the gaps) leaves about 25px per box with 4px gaps. The implementer picks one box size per device type: touch layouts (tablet 330px, phone 344px) keep 34px, which fits; desktop gets the largest size and gap that fit with nothing shrinking. The optional warning, paused and offline indicators use the same box and the row never shrinks a box for them; when the row would overflow (the optional indicators shown at 280px), the footer wraps those indicators onto a second line above the row, and no box shrinks.
- The two tray meters (health bars, limits bars) keep their bar graphics and share the row's vertical centre line with the boxes.

**Settings button icon.** The sun-like icon is removed and replaced by a burger: three horizontal lines in a 20px viewBox with the footer's 1.6px stroke and round caps. The button, its menu and its items are unchanged.

**New dialogs: focus (fact-finding result).** Checked in a running `workspaces` fixture at 1280px by logging focus events.
- New workspace and New chat opened from the "+ New" menu or the workspace switcher end with focus on the page body. New workspace's Name field does take focus (its `autoFocus` fires), but the menu then returns focus to its own trigger and, while it unmounts, to its own panel, and focus ends on the body. New chat has no `autoFocus` at all, and its fields appear only after "Loading projects".
- New design from the "+ New" menu is a race. Its sheet renders its form after an async draft load. In one run that landed after the menu finished closing and focus ended on Epic slug; in another the menu's panel still held focus. From the Epics page button (no menu) it always focuses Epic slug.
- Root cause: the shared menu returns focus to its trigger and its panel after an item opens a dialog, so it overrides whatever the dialog focused. The modal's own mount rule (focus the panel, so a phone keyboard stays closed) does not cause it.
- Fix: (1) the shared menu must not move focus back to its trigger or hold it when the selected item opened a layer; the layer owns focus. When the layer closes, focus return is not required by this spec. (2) One shared mechanism focuses a dialog's first text field on mount, on tablet and desktop only (the width rule the shell already uses), and does nothing on a phone. It replaces the unconditional `autoFocus` attributes on New workspace and New design (today those also fire on a phone when no menu interferes) and is added to New chat's Name. Because New design and New chat render their fields after async loads, the mechanism acts when the field mounts, not when the dialog mounts, and only while focus is still on the dialog panel or the body. A person who has already moved into another field (the Project select, First prompt) keeps their place. The implementer picks its shape. On a phone the dialog panel keeps focus, as the modal intends.
- The first field is the first text field: New workspace, Name; New chat, Name (not the Project select, Worktree picker or First prompt); New design, Epic slug (not the Project select).

**New dialogs: default project.**
- One shared rule, in one module both forms call: a project is chosen from, in order, (1) the project of the current page, (2) the Epics page filter when it picks exactly one known project, (3) the last project used, (4) the first project. A candidate not in the form's project list is skipped.
- Current page: an epic, Design or ticket address carries its project as the prefix before the colon. A chat's address is its session id, and its project comes from the session in the live snapshot. Other pages (Epics list, Stats, Settings, the agents list with no chat open) have no page project.
- The rule reads the Epics filter from where the Epics page keeps it, so every entry point (sidebar, Epics page, Agents page) gets one answer. A filter counts only on the terms the Epics page itself uses (it applies the filter only when there is more than one project).
- The rule is computed by the forms from the router and the snapshot, not passed in by each caller. The `prefillProject` parameter of the New chat form, which no caller passes, is removed, and the Design form's separate filter prop is replaced by this rule.
- A draft keeps its project when it holds the person's own text, as today (New chat's local draft, and New design's saved server draft). The rule picks only for a draft with no text, so it never overrides work in progress.
- "Last project used" is the one the server already stores (the Design drafts last-project read and write, which New design writes when a project is picked). Both forms read it, and New chat writes it when it creates a chat, so the two never disagree. There is no per-device New chat memory. The Go doc comment on `DesignLastProject` is updated to say both forms share it.

**Sidebar gesture.**
- The hook that handles sidebar swipes keeps its 40px, mostly horizontal rule and its close behaviour (swipe left on the open sidebar or its scrim, and the existing scroller rule for it) as they are.
- Open: a rightward swipe that starts more than about 30px from the left edge opens the sliding sidebar on phone and tablet, in a tab and in the installed app. The start within 30px is ignored so the system's back swipe keeps it. The home-screen check, `isStandalone`, and its use are removed. Docked sidebars (desktop) never swipe.
- A touch that starts inside a horizontal scroller never opens. A horizontal scroller is an element that overflows sideways with `overflow-x` auto or scroll, at any scroll position and in either direction, so one at its first or last card still counts. Note that an element with only a vertical scroll computes `overflow-x` as auto too, so the test must also require real horizontal overflow (width of the content over the width of the box), or every page and the sidebar list would block the gesture.
- That test cannot see a surface that holds one item (no overflow yet). The question cards row on a Design (scroll-snap) must never open the sidebar, including with one question and on the first card, so surfaces that own horizontal swipes also opt out explicitly with a marker the hook honours. The implementer picks the marker. The question cards row carries it.
- A swipe does nothing while any layer is open (a dialog, the jump menu), using the existing layer stack, and does nothing when the touch starts in a text field or a range slider. Multi-touch is ignored, as today.
- Real iPhone behaviour (the installed app's back swipe, the Safari tab) is not automatable and is checked by the person.

## Testing Decisions

- A good test drives a public seam and checks what a person sees: roles, visible text, focus, geometry measured in a real browser, and the request bodies. It never checks class names, internal helpers or file layout.
- **Web unit (vitest, jsdom)** is the main seam. It covers: nested rows (no icon, the menu items by agent kind, no menu for a queue session, Remove's confirmation and navigation, the menu button belonging to its own row), the Other section reusing it, the shared default-project rule (page project from each address kind, a chat's project from the snapshot, the one-project filter, last used, first, a text draft keeping its project, a candidate missing from the list), both forms calling it, the focus mechanism (wide focuses, a phone does not, fields that mount late, and focus already moved to another field before the late mount is not stolen), the Menu not taking focus back after an item opens a dialog, the gesture hook (open from away from the edge, ignored within 30px, 40px and mostly-horizontal rules, a horizontal scroller start ignored even at its end, the marker, an open layer, a text field start, close unchanged, no standalone gate), the burger icon button, and the footer button's badge. jsdom has no layout, so the scroller test stubs the scroll sizes. Prior art: `SidebarRows.test.tsx`, `SidebarFooter.test.tsx`, `GearMenu.test.tsx`, `Menu.test.tsx`, `Modal.test.tsx`, `NewSessionForm.test.tsx`, `AppShell.test.tsx`.
- **Browser (Playwright)** only for what jsdom cannot see, per gx-playwright-e2es: real focus after a real menu closes, real layout (title and icon alignment, footer box and badge geometry, which depend on real widths) and real touch events with real scroll sizes. No new flow. Extend the existing "Workspaces" flow (and the Design page phone flow for the question cards), and update the step that says the edge swipe does not open the sidebar in the same change. Keep `maxFlows` and CATALOG.md in step. Reads follow an `expect` on the same locator.
- No server or CLI change, so no Go test.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`). Flow names are from `webui/e2e/CATALOG.md`; "the `workspaces` scenario" is the existing scenario. Look checks refer to the Mock states list below by state name, and are measured at 1280px with the sidebar docked (280px wide) unless noted.

1. Story 1: Mock states `collapsed-and-expanded` and `long-names`; flow "Workspaces" (extended): with an epic expanded, a nested row's title x equals its parent's title x (bounding boxes) and the nested row has no icon.
2. Story 2: Mock states `collapsed-and-expanded`: the guide line sits under the parent's icon column and ends at the last nested row's middle.
3. Story 3: Mock states `collapsed-and-expanded`: nested title 13px, parent 14px (computed style).
4. Story 4: Mock states `long-names`; flow "Workspaces" (extended): a nested row's status words and menu button share their x with the parent's.
5. Story 5: flow "Workspaces" (extended): open a nested agent's menu: the Agent block, Stop (live agent) and Remove. Mock states `hover-menu` for the look. Vitest on the items.
6. Story 6: vitest: for a session the person manages, the items equal the agent page's menu; flow "75. Read a session" still passes.
7. Story 7: flow "Workspaces" (extended): Remove a nested agent from its row after the confirmation, the row leaves, `gx agents` no longer lists it; with that agent open, the page is the agents list.
8. Story 8: vitest: a queue session's nested row renders no menu button. Vitest: a queue session that has a tier shows no menu. Flow "Workspaces" (extended): the `workspaces` fixture gets a queue session under one of its epics, and its nested row shows none.
9. Story 9: flow "Workspaces" (extended) at 1280px: hovering a nested row shows its button and not its parent's; Tab onto it shows it. Mock states `hover-menu`.
10. Story 10: Mock states `touch`: with touch emulation (no hover) every row, nested ones included, shows its button.
11. Story 11: Mock states `long-names` (fix agent row): wrench 18px, title x equal to other nested titles, on `paused-epic`'s Fix agents.
12. Story 12: Mock states `other-section`: the Other section's rows, including the project-level Fix agent, use the nested look and menu.
13. Story 13: Mock states `current-nested`.
14. Story 14: Mock states `long-names`: truncation with an ellipsis, status words not pushed.
15. Story 15: Mock states `icon-column`; flow "Workspaces" (extended): the default workspace's chat row and an epic row have the same title x.
16. Story 16: Mock states `icon-column`: the bubble and the ring with its dot are centered in the slot.
17. Story 17: flow "Workspaces" (extended): measure the footer's boxes at 1280px: Epics, Follow-ups, Stats and the settings button have equal width and height, and the icon centers sit evenly; repeat at 390px. With the health warning, paused and offline indicators shown, no box shrinks (the fixtures' `health` and `disk-low` shapes raise them).
18. Story 18: flow "Workspaces" (extended): the Epics badge and the Follow-ups badge have the same size, font and corner offset (bounding boxes and computed style). Vitest on the footer button.
19. Story 19: vitest: the Epics badge is amber, the Follow-ups badge amber when something waits and blue otherwise; counts and labels unchanged.
20. Story 20: flow "Workspaces" (extended): at 1280px the footer's contents do not overflow the sidebar's width.
21. Story 21: flow "Workspaces" (extended): the settings button's icon has three horizontal paths and no circle. Vitest on the icon.
22. Story 22: flow "Workspaces" (extended): the menu lists AFK, Vi mode, Sort, Restart server, "More settings...", Cleanup and Tips, and nothing else; flows "129. Restart server keyboard" and "130. Restart dialog confirms" still pass.
23. Story 23: flow "Workspaces" (extended) at 1280px: "+ New" > "New workspace..." focuses the Workspace name field, and so does the switcher's "New workspace...". Vitest on the mechanism.
24. Story 24: flow "85. Create session" or the "Workspaces" flow (extended) at 1280px: "+ New" > "New chat" focuses Name, not Project or First prompt, once the form has loaded.
25. Story 25: flow "Workspaces" (extended) at 1280px: "+ New" > "New design" focuses Epic slug every time (repeat the open several times to catch the race); flow "26. New Design form" still passes.
26. Story 26: the Epics page and Agents page buttons focus the same fields (flow "26. New Design form" and "85. Create session" gain a focus assertion).
27. Story 27: flow "41. Phone, New Design sheet" (extended) at 375px: the dialog panel has focus and no text field does; the same for New chat and New workspace in "Workspaces" at 390px.
28. Story 28: flow "41. Phone, New Design sheet": Tab keeps focus inside the dialog (existing step); vitest on `Modal`.
29. Story 29: vitest: the rule for each address kind. Flow "Workspaces" (extended): on the second project's Design page, "+ New" > "New chat" and "New design" show that project; on its chat's page, "New chat" shows it.
30. Story 30: vitest: the one-project filter wins when the page has no project and is skipped when it picks two or none. Flow "Workspaces" (extended): from the Epics page filtered to the second project, both forms show it.
31. Story 31: vitest: last used then first; flow "Workspaces" (extended): create a chat in the second project, then open New chat from the Stats page: it shows that project.
32. Story 32: vitest: both forms resolve through the one rule (same inputs, same project).
33. Story 33: vitest: a New chat draft with text keeps its project, and a saved New design draft keeps its project, whatever the page.
34. Story 34: flow "Workspaces" (extended) at 744px and 390px: a swipe right starting at x=150 opens the sidebar.
35. Story 35: vitest: with and without the standalone signals the swipe behaves the same; a start within 30px opens nothing. Real home-screen behaviour is checked by the person on an iPhone (not automatable); the verify ticket says so.
36. Story 36: flow "Workspaces" (extended) at 390px in the default browser context: the swipe from x=150 opens the sidebar and from x=5 does not (replaces the existing edge-swipe step).
37. Story 37: vitest: a start inside an element with horizontal overflow, at its start or end, opens nothing, and an element with only vertical scrolling does not block it. Flow "24. Phone, Design page" (extended): a swipe right on a wide code block or other horizontal scroller on a page leaves the sidebar closed, where the fixture has one; otherwise vitest covers it.
38. Story 38: flow "24. Phone, Design page" (extended) at 375px: in the Questions view, a swipe right on the first question card leaves the sidebar closed and a swipe left moves to the next question; also with a single question (the marker).
39. Story 39: flow "Workspaces" (extended) at 744px and 390px: a swipe left on the open sidebar closes it (existing step).
40. Story 40: flow "Workspaces" (extended): a vertical drag and a tap do not open it (existing step); vitest on the 40px and mostly-horizontal rule.
41. Story 41: vitest: with a Modal or the jump menu open, a swipe opens nothing.
42. Story 42: flow "Workspaces" (extended) at 1280px: nothing swipes the docked sidebar; vitest on the hook with `docked`.

New flows or scenarios to write (these become implementation tickets):

- No new Playwright flow. Extend "Workspaces", "24. Phone, Design page", "41. Phone, New Design sheet", "26. New Design form" and "85. Create session" with the steps above, and change the step that asserts the edge swipe does not open the sidebar. Keep CATALOG.md and `maxFlows` in step.
- A second project in the `workspaces` scenario (or, if that breaks other flows' counts, a new scenario for it), holding a Design and a chat, so the default-project steps can tell projects apart. The implementer picks which.
- A queue session nested under one epic of the `workspaces` scenario, for the no-menu check.
- A fixture step that raises the health warning, paused and offline indicators together, for the footer overflow check, if the existing shapes do not combine them.

**Mock states**

- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `collapsed-and-expanded`, 280px, the `workspaces` scenario with epics collapsed and expanded: icon-column, icon, title, status, menu, nested-row.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `long-names`, 280px, the `workspaces` scenario with an epic expanded to its long-named agents and `paused-epic` with a Fix agent: icon-column, icon, title, status, menu, nested-row.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `current-nested`, 280px, the `workspaces` scenario with a nested agent's page open: icon-column, icon, title, status, menu, nested-row.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `hover-menu`, 280px, the `workspaces` scenario with the pointer on a nested row and its menu open: icon-column, icon, title, status, menu, nested-row, menu-popup.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `touch`, 280px, the `workspaces` scenario in a no-hover (touch) context: icon-column, icon, title, status, menu, nested-row.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `other-section`, 280px, the `workspaces` scenario with the Other section open (a Fix agent among its rows): title, status, menu, nested-row, icon-column.
- `/Users/david/.local/share/gx/tickets/gx/nav-fixes2/mocks/02-tree-row-mock.html`, `icon-column`, 280px, the `workspaces` scenario with the chat row and epic rows visible: icon-column, icon, title, status, menu.

## Out of Scope

- Persisting the tree's expand state.
- Changing the settings menu's items.
- Any server, CLI, wire or schema change.
- Left-edge open swipe, in a tab or the installed app.
- Move, Hold or Promote items on nested agent rows.
- Changing the chat row's menu, the sort order or the Alt numbers.
- Returning focus to the trigger after a dialog opened from a menu closes.

## Further Notes

- The approved mock is stored in the map's `mocks/` directory. Where the mock and this spec differ, the mock wins on look and this spec wins on behaviour. The mock shows no footer, burger or dialogs, so those are checked by measurement and by eye.
- The focus finding was made in a running fixture with `playwright-cli` at 1280px, following gx-ui-check: New workspace and New chat from the menu end on the body, and New design from the menu depends on timing. The focus log is the evidence the implementer should reproduce before the fix and after it.
- The row hover group and the footer's shrinking boxes are the root causes behind the "menu appears on the wrong row" and "misaligned icons" symptoms; fix them there and not with offsets or per-row special cases.
- The Epics page's filter, the New chat draft and the Design draft already persist in the browser or the server; this spec adds only New chat writing the shared last-used project.
