# Better navigation: workspaces, one sidebar, tablet, hold and shortcuts

## Problem Statement

I work on a few Designs, epics and agents at a time (4 or 5 Designs, plus the Queue epics that wait
for my answer), out of many more that sit mid-design or running. Today I cannot jump between them
quickly inside gx. The Epics page lists everything, the Cmd+K jump menu exists only on three pages and
lists epic cards only, and the Agents page has its own session switcher. So I open one browser tab per
tracked item, and I cannot use gx as a single app window. Designs I do not want to deal with now clutter
every list, and there is no way to say "not now". The iPad mini (about 744px wide, mostly portrait) gets
the phone layout. The phone Design page footer is cramped.

## Solution

One sidebar in the app shell, on every page, shows what I work on now: the items of one **workspace**
at a time, picked from a switcher. A workspace is a named theme I create ("gx bugs", "research").
Every Design, epic and chat is in exactly one workspace, and new items start in the **Default
workspace**. Agents sit under the epic they work for. A **hold** mark takes a Design or epic out of the
main lists into a collapsed On hold section. Cmd/Ctrl+K opens one jump menu on every page: it shows the
active workspace first and searches everything once I type. The Epics page stays the "see everything"
view, with each card's workspace name and a workspace filter. The web UI has three device types (phone,
tablet, desktop) by width alone, so the iPad gets a real layout. Keyboard shortcuts jump between rows and
workspaces. The phone Design page footer becomes one row. "Promote to Design" moves into the agent
page's "..." menu.

## User Stories

Sidebar and shell

1. As a person who tracks several items, I want one sidebar on every page, so that I can jump between my Designs, epics and agents without browser tabs.
2. As a person who used the Agents page sidebar, I want that sidebar replaced by the shared one, so that agents are reachable from every page.
3. As a person, I want the sidebar to list only the items of the active workspace, so that the list stays short.
4. As a person, I want the workspace switcher in its own row under the header, with a chevron and the active workspace's name, so that I can see and change where I am.
5. As a person, I want the switcher to list every workspace with its Needs-you count, then "New workspace..." and "Manage workspaces", so that I see where work waits for me.
6. As a person, I want each row to show a progress ring with a kind dot (purple Design, teal epic) or a pink chat icon, the name, and a status in words ("Needs you" in amber, "Working" in blue), so that I can scan the list without dots or counts.
7. As a person, I want a Design to be one row, so that nothing is nested under it, and the Design page links to its design epic.
8. As a person, I want an implementation epic to nest its agents behind a small chevron at the left of its row, with the open page's epic starting expanded, so that I reach an agent from the epic it works for.
9. As a person, I want plain chats as rows in their workspace, so that I do not need a separate Chats section.
10. As a person, I want the header to hold the wide logo with the build version and a "hide sidebar" panel icon, so that I can see the build and hide the sidebar.
11. As a person, I want a footer with "+ New" (New design, New agent, New workspace), icons for the Epics overview, Stats, health and a gear, each with a tooltip, so that creating and the rest of the app are one click away.
12. As a person, I want the gear menu to hold AFK, Vi mode, Settings, Restart server, spend and build, so that the old top bar's controls still have a home.
13. As a person, I want the warning (bad health), paused (the Server queue is paused) and disconnected icons to show in the footer next to the health icon only when they apply, each explaining itself with a tooltip, so that problems are visible without a top bar.
14. As a person, I want no top bar on any device, and one shared page header (large icon, title, subtitle, actions, with the ring or status icon before "..."), so that every page looks alike.
15. As a desktop person, I want the sidebar always visible at 280px and collapsible, so that I can reclaim the width.
16. As a tablet person, I want the sidebar to slide over the page at about 330px and close when I pick an item, so that it does not take width the page needs.
17. As a phone person, I want the sidebar to slide over the page at nearly full width (344px), so that rows are easy to tap.
18. As a tablet or phone person, I want a bare mascot in the page header to toggle the sidebar, and on desktop only while the sidebar is collapsed, so that I can open it.
19. As a person, I want the mascot to show an amber dot when something needs me and the sidebar is closed, following the server's total Needs-you count across all workspaces with held items left out (the same number as the Epics tab badge), so that I notice without opening it.
20. As a person, I want the UI to say "Chat" and "Chats" for plain agent sessions, so that the words match what I see.

Workspaces

21. As a person, I want every Design, epic and chat in exactly one workspace, with the remaining agents under their epic or in the global Other section, so that nothing is ungrouped.
22. As a person, I want new items to start in the Default workspace, so that nothing needs setup.
23. As a person, I want to create a workspace by name, with names unique without regard to case, so that two themes are never confused.
24. As a person, I want to rename a workspace, so that a theme can change.
25. As a person, I want to delete a workspace and have its members return to the Default workspace, with nothing deleted, so that cleanup is safe.
26. As a person, I want the Default workspace to be undeletable, so that items always have a home.
27. As a person, I want to reorder workspaces by hand, with new ones last, so that the switcher and Alt+Shift+number follow my order.
28. As a person, I want a "Move to workspace" action on a sidebar row, an Epics card and a jump menu row, so that I can file an item wherever I see it.
29. As a person, I want "Remove from workspace" on every member outside the Default workspace, with a confirmation, sending the item back to the Default workspace, so that I can undo a placement. The action is hidden on Default members, and the CLI verb succeeds as a no-op on them.
30. As a person, I want a Design and its `-impl` epic to be one member, with the epic inheriting the Design's workspace, so that the card keeps its place across Branch.
31. As a person, I want Design agents and Queue agents to follow their epic's workspace and not join alone, so that agents never drift from their epic.
32. As a person, I want One-off agents and project-level Fix agents unable to join, living in the global Other section, and Follow-ups to have no sidebar row (they are tickets, not sessions, and show on the Epics page only), so that workspaces hold only things I work on. One-off and Follow-ups cards on the Epics page carry no workspace name and no Move or Hold action.
33. As a person, I want a finished epic to stay in its workspace until I remove it, so that nothing moves on its own.
34. As a person, I want a chat to leave its workspace when its session exits, and a deleted epic or Design to lose its membership, so that workspaces hold no dead items.
35. As a person, I want joining a workspace to change nothing on the item, so that a workspace is only a view.
36. As a person, I want opening an item in a workspace to make that workspace active, and opening a held item or an Other row to leave the active workspace unchanged, so that the sidebar follows me without jumping to held work.
37. As a person, I want the active workspace remembered per browser, so that my phone and desktop can each show what I want.
38. As a person on two devices, I want workspaces, membership and holds to sync, so that a change on one shows on the other without a reload.

On hold

39. As a person, I want to put a Design or epic on hold, so that it leaves the main lists while I do not want to deal with it.
40. As a person, I want hold and "Take off hold" in the Epics card menu, the sidebar row menu, the Design or epic page header and the jump menu row, so that I can reach them where I am.
41. As a person, I want holding a Design, from the web or the CLI, to exit its live Design sessions and to stop auto-advance, with the web asking first and `gx server hold` refusing while Design sessions are live unless `--yes` is passed (then printing which sessions it ended), so that a held Design does no work and nothing ends by surprise. Other agents on a held epic keep running.
42. As a person, I want taking a hold off to start nothing and ask no confirmation, so that I continue with Continue as usual.
43. As a person, I want held items in a collapsed On hold section at the bottom of the sidebar's workspace, so that they stay out of the way but keep their workspace.
44. As a person, I want held items never to count in Needs you or the Epics tab badge, so that the badge means work waiting on me.
45. As a person, I want a loop epic to be holdable only while it is not in the Server queue, with the disabled action saying "Remove it from the queue first", so that a queued epic is not silently stalled.
46. As a person, I want queueing a held epic to take its hold off, so that queueing means what it says.
47. As a person, I want a hold to cover a Design and its `-impl` epic together, so that I hold one thing.
48. As a person, I want held rows to show the same status words dimmed, so that I can tell them from live ones.

Jump menu

49. As a person, I want Cmd/Ctrl+K to open the jump menu on every page, including the Epics page, so that I can jump from anywhere.
50. As a person, I want the empty menu to show the active workspace's items first, then a Needs you section for items outside it, so that I see only what is relevant.
51. As a person, I want typing to search everything: every workspace, held items, Ready to design, Finished and agents, so that nothing is unreachable.
52. As a person, I want an agent row to name its epic in the subtitle, a row from another workspace to show the workspace name, and a held row to say "On hold", so that I can tell rows apart.
53. As a person, I want page titles to stop being dropdowns and the Agents page session switcher removed, so that there is one way to jump.
54. As a tablet person, I want the jump menu as a popover near the top, so that it matches desktop.
55. As a touch person without Cmd+K, I want to reach items outside the active workspace through the switcher, so that I need no search button.

Epics page

56. As a person, I want the Epics page to keep all cards from all projects, so that it stays the "see everything" view.
57. As a person, I want every Design and epic card to show its workspace name, so that I know where it lives. One-off and Follow-ups cards show none, have no Move or Hold action, and appear only under "All workspaces". They still count in the Epics badge and the mascot dot; no workspace's switcher count includes them; a waiting One-off agent shows in the empty jump menu's Needs you section.
58. As a person, I want a workspace filter next to the text filter, defaulting to "All workspaces" and kept only in the page, so that I can narrow the view without storing anything.
59. As a person, I want the state sections (Needs you, Implementing, Ready to queue, Designing, Ready to design, Finished) unchanged, so that waiting work stays on top.
60. As a person, I want a last collapsed "On hold" section with a count in its title, opened by the text filter when it matches, with held cards dimmed and a "Take off hold" action, so that held work is findable but quiet.

Device types and shortcuts

61. As a person, I want the layout chosen by width alone (phone below 640px, tablet 640 to 992px, desktop from 992px), so that my iPad mini is not treated as a phone.
62. As a tablet person, I want every page to take its wide layout from 640px, so that no page breaks at 744px.
63. As a person, I want Alt+1 to Alt+9 to open the nth top-level row of the active workspace, with the number shown on rows while Alt is held, so that I can jump by keyboard.
64. As a person, I want Alt+Shift+1 to Alt+Shift+9 to pick the nth workspace in the switcher's order, so that I can switch by keyboard.
65. As a person, I want Cmd/Ctrl+B to toggle the sidebar and focus it when it opens, so that I can drive it from the keyboard.
66. As a person, I want Down/Up or j/k to move between rows, Right/Left or l/h to expand and collapse an epic's agents, and Enter to open the row, with j/k/h/l only while the sidebar has focus, so that vim-style keys never clash with typing.
67. As a person, I want the shortcuts off while a text field has focus, except Cmd/Ctrl+K, so that typing is never hijacked.

Agent and Design page

68. As a person, I want "Promote to Design" in the agent page's "..." menu on all three device types, disabled until the first message with the reason shown, so that the header is quiet and the reason is clear.
69. As a phone person, I want the Design page footer in one row (Add, Note, Auto, Send), with Add and Note as 44px icon buttons, so that it fits at 390px and 320px.
70. As a phone person, I want the Auto button to keep both mini switches visible with the short labels "Advance" and "Queue", and today's popover on tap, so that I see both states.
71. As a phone person, I want "Send" as the one wide primary button, dimmed with no answers, so that the main action is clear.
72. As a phone person, I want a dot on the Note icon while a note is open or non-empty, so that I know a note is there.
73. As a phone person, I want a "Continuing in 5s" strip with Cancel above the row only while Auto-advance counts down, so that the row stays one line otherwise.
74. As a tablet or desktop person, I want today's footer with full labels (Auto-advance, Auto-queue), the inline countdown and "Send answers", with Note moved next to Add, so that Send stays alone on the right.
75. As a person, I want the footer icons to come from a real icon library instead of emoji, so that they look consistent.

CLI and server

76. As a person using the CLI, I want `gx server workspaces` verbs (list, create, rename, delete, reorder, move, remove) with `--json`, so that I can script what the UI does.
77. As a person using the CLI, I want `gx server hold` and `gx server unhold <epic>` with `--json`, so that I can hold from a terminal.
78. As a person, I want my workspaces, membership and holds to survive a server restart, so that I never rebuild them.

Review additions

79. As a person, I want a global, fixed "Other" section at the bottom of the sidebar, below On hold and identical in every workspace, listing One-off agents (agents of tickets in a project's one-offs epic), project-level Fix agents (for a failed project ticket migration) and any other agent whose epic is a one-offs epic (a Fix agent on a One-off ticket, an Investigate agent for a failed one-off), with no Move or Hold actions and with opening a row leaving the active workspace unchanged, so that agents with no epic of mine are reachable without belonging to a workspace. A Fix agent on a ticket and an Investigate agent of any other epic stay nested under their epic's row.
80. As a person, I want "Manage workspaces" to open a dialog listing workspaces in order, where I rename in place, reorder with up and down buttons, and delete behind a confirmation that says how many members go back to the Default workspace, and where the Default row can be neither renamed nor deleted, so that I can tidy workspaces without the CLI.
81. As a person, I want holding a Design whose `-impl` epic is queued to be refused with "Remove it from the queue first", on the web and in the CLI, so that a queued epic is not silently stalled.
82. As a person, I want a Design made by Promote to Design to start in its chat's workspace, so that it lands where I was working.
83. As a person, I want the active workspace on load or deep link to be the open item's workspace if the item is not held, otherwise the stored workspace, and the Default workspace if that no longer exists, so that a link opens in the right place and a held item never moves me.
84. As a person, I want starting a Design session on a held Design (web Continue, CLI or TUI) to take its hold off, the same way queueing does, so that a live Design session never asks questions that stay out of Needs you.
85. As a person, I want a Ticket or agent page to stand for its epic, and a page with no workspace item (Epics, Stats, Settings, an Other row, a One-off or Follow-up ticket) to keep the stored active workspace, so that such pages never move me.
86. As a person, I want the sidebar to fall back to the Default workspace at once when the active workspace is deleted while a page is open, on this device or another, so that the sidebar never shows a dead workspace.
87. As a person, I want Alt+1 to Alt+9 to count only the workspace's own rows above On hold, and j/k and the arrows to move through every visible row, including an expanded On hold section and the Other section, so that the number chords stay stable while navigation reaches everything.
88. As a person, I want any hold that covers a Design, by the Design or the `-impl` address, to exit its live Design sessions under the same `--yes` rule, with `--json` listing the ended sessions, so that holding the joined card behaves the same from either address.

## Implementation Decisions

**Vocabulary.** Terms are in GLOSSARY.md: Workspace, Manage workspaces, Default workspace, Active workspace, On hold, Other section, Chat session, App shell, Tray, Device type, Design, Epic card. In the web UI "Workspace" wins over herdr's meaning, and "Chat" means a plain agent session (a Chat session), not the Slack/Telegram Chat surface. The UI says "Chat" for plain agent sessions. The word "Epic" is not renamed here.

**Server state: one workspace store.**
- Workspaces, membership and holds live in one store in the server's state dir. They do not go in epic `ticket.md` frontmatter, so there is no schema change and no migration step (ADR 0026 stays true for ticket data; this is per-person state, not ticket data).
- A Design or epic is keyed by `project:epic`. A plain chat is keyed by its session label (it survives `/clear`; the conversation ID does not). Design agents, Queue agents, a Fix agent on a ticket and Investigate agents (an Investigate ticket is a child ticket in the epic that failed) follow their epic, unless that epic is a one-offs epic and need no key. One-off agents, project-level Fix agents and any agent whose epic is a one-offs epic (a Fix agent on a One-off ticket, an Investigate ticket for a failed one-off) are in no workspace: they appear in the global Other section. One-off and Follow-ups cards on the Epics page carry no workspace name and no Move or Hold; they show only under "All workspaces", still count in the Epics badge and mascot dot, and no workspace's switcher count includes them. A waiting One-off agent shows in the empty jump menu's Needs you section. Follow-ups are tickets, not sessions, so they have no sidebar row. A Design and its `-impl` epic are one member, so the `-impl` epic reads its Design's workspace and hold.
- The store holds an ordered list of workspaces. Order is the list order. Names are unique, compared without case. The Default workspace is always present, cannot be deleted, and keeps its fixed name (the glossary name), so it cannot be renamed. Where it sits in the order is the person's to set like any other. Items with no stored membership read as Default. Removing a Default member from its workspace is a no-op: the web hides the action and the CLI verb succeeds without a change.
- Deleting a workspace moves its members to the Default workspace; nothing else changes. "Remove from workspace" does the same for one item, behind a confirmation in the web UI.
- The server drops a member when its epic or Design is deleted or its chat session exits.
- Workspace logic (membership rules, the inheritance of an `-impl` epic, the Default fallback, name uniqueness, cleanup) lives in the server module that owns the store. The web client and the CLI call it and do not reimplement it. The web routes and the CLI verbs share one code path (ADR 0028: the server API mirrors the CLI).
- A Design made by Promote to Design starts in the workspace of the chat it came from, not in the Default workspace.
- The snapshot carries workspaces (in order, with each one's Needs-you count), membership and holds. One new `workspaces-changed` event tells open pages to refresh, as `afk-changed` and `vimode-changed` do. The implementer names the event payload, the routes and the store's file format.
- There is no offline fallback: if the server is down, the web UI is down too.

**Hold.**
- Hold is a mark on a Design or epic in the same store. It is not a pause kind (ADR 0042); it adds no pause to the pause-to-verb table, and its web buttons are the card menu, sidebar row menu, page header and jump row, so nothing needs a CLI.
- The server reads the mark in exactly three places: it skips auto-advance for a held Design, it refuses to hold an epic that is in the Server queue (or a Design whose `-impl` epic is), and starting a Design session on a held Design clears the hold on every path (web Continue, CLI, TUI). Queueing a held epic also clears its hold.
- The server exits the live Design sessions of a Design on every hold path, web or CLI, and by either address: any hold covering a Design, by the Design or the `-impl` address, ends them under the same `--yes` rule, and `--json` lists the ended sessions. The web asks first; `gx server hold` refuses while Design sessions are live unless `--yes` is passed, and prints which sessions it ended. Other agents on a held epic keep running. Taking a hold off starts nothing.
- Holding a Design whose `-impl` epic is in the Server queue is refused with "Remove it from the queue first", web and CLI, the same as for a queued loop epic.
- A held item keeps its workspace and never counts in Needs you or the Epics tab badge. The server computes both, so the badge and the Needs-you section never count held cards.
- A Design never goes on hold by itself. Only the person holds and takes off hold.

**Epics list (`/v1/epics` and its wire type).**
- The server adds an `on-hold` card section, placed after Finished, and sends each card's workspace name. The server decides sections; the client only filters (the existing rule for `CardSection`).
- A held card appears only in `on-hold`, not in its state section, and carries the dimmed status words and a "Take off hold" action.
- `gx server epics` shows the new section and the workspace name; the implementer names any new column or flag.

**Web app shell and sidebar.**
- The jump menu, the sidebar and the page header are mounted once in the app shell, not per page. The sidebar replaces the Agents page sidebar. The top bar is removed on every device; its tabs, AFK and Vi toggles, Settings, Restart server, build, spend and the Tray move into the sidebar footer and gear menu (the Tray sits in the footer).
- One shared PageHeader component takes a large icon, title, subtitle and actions, and every page header is built from it. The title is 19px in the regular text font. Existing page headers (Epics, Epic, Ticket, Design, Agents, Stats, Settings) are ported to it.
- The active workspace is per browser, in localStorage, and the server stores nothing for it and sends no event on a click. On load or deep link the open item's workspace wins if the item is not held; otherwise the stored workspace is used; if that no longer exists (deleted on another device), the sidebar falls back to the Default workspace. A Ticket or agent page stands for its epic. A page with no workspace item (Epics, Stats, Settings, an Other row, a One-off or Follow-up ticket) keeps the stored workspace. If the active workspace is deleted while the page is open, on this device or another, the sidebar falls back to Default at once.
- The sidebar ends with a global, fixed Other section below On hold, the same in every workspace, listing One-off agents, project-level Fix agents and any agent whose epic is a one-offs epic. Its rows have no Move or Hold actions, and opening one leaves the active workspace unchanged. A Fix agent on a ticket and an Investigate agent of any other epic nest under their epic's row.
- "Manage workspaces" opens a dialog listing workspaces in order: rename in place, reorder with up and down buttons, delete behind a confirmation that says how many members go back to the Default workspace. The Default row has no rename or delete.
- "Remove from workspace" is hidden on Default members. Mock 07's row menu shows it unconditionally; this spec overrides the mock.
- The mascot's amber dot follows the server's total Needs-you count across all workspaces with held items left out, the same number as the Epics tab badge. The paused footer icon means the Server queue is paused.
- Rows, nesting, status words, the On hold section and the footer follow the approved mock 07. A Design is one row (its design session). An implementation epic nests its agents; the open page's epic starts expanded.
- Device type is a single shared hook used by the shell and every page: phone below 640px, tablet 640 to 992px, desktop from 992px. The wide-layout breakpoint for page content drops from 992px to 640px; the sidebar stays docked only from 992px. `useCoarsePointer` stays separate and only sizes touch targets. The gallery gets a forced device type next to its forced wide flag.
- Because the sidebar takes no width below 992px, every page's wide layout must fit 640 to 992px. Pages checked at 744px in a fixture: Epics, Epic, Ticket, Design, Agents, Stats, Settings. Anything that breaks is fixed in the wide layout; there are no separate tablet layouts. Popovers replace sheets on the tablet.
- The jump menu on the tablet is a popover near the top, like desktop. On the phone it keeps its present presentation. The implementer picks any detail the mock does not show.
- If the iPad proves hard without a search button, a search row at the top of the sidebar is the fallback (a later change, not part of this spec).

**Jump menu.**
- Mounted in the app shell; Cmd/Ctrl+K on every page. Page titles stop being dropdowns. The Agents page session switcher and its Actionable/Unread/Other sections and Queue/Design toggles are removed.
- Empty filter: the active workspace's items, then a Needs you section for items outside it. Typing searches everything. Rows: Designs, epics, plain chats and agents. No recent or pinned items.

**Keyboard shortcuts** (web UI only, fixed keys, no settings screen and no remapping).
- Alt+1 to 9 open the nth top-level row of the active workspace in sidebar order, counting only the workspace's own rows above On hold, and rows show their number while Alt is held. Cmd+number is not used (browsers keep it). Alt+Shift+1 to 9 pick the nth workspace in the switcher's order. Cmd/Ctrl+B toggles the sidebar and focuses it on open. Arrows or j/k move through every visible row, including an expanded On hold section and the Other section, Right/Left or l/h expand and collapse, Enter opens. Vim keys act only while the sidebar has focus. All shortcuts are off in text fields except Cmd/Ctrl+K. No shortcut for hold or move to workspace.
- Whether the Alt chord is read from the key's physical code or its produced character (Mac Option changes the character) is the implementer's choice; it must work on a Mac keyboard.

**Agent page.** The standalone "Promote to Design" button and its hint line leave the header. The "..." menu item is disabled until the agent has its first message, and its subtitle or tooltip says why. This applies on all three device types.

**Design page footer** follows approved mock 09 and replaces the two-row phone footer of mock 07.
- Below 640px: one row, Add, Note, Auto, Send. Add and Note are 44px icon-only buttons with tooltips; Auto keeps today's two stacked mini switches with the labels "Advance" and "Queue" and today's popover; Send is the one wide primary button, renamed "Send" on the phone; the countdown strip with Cancel (40px high) sits above the row only during the countdown. All targets are at least 40px high; it fits 390px and 320px.
- From 640px: today's footer (full Auto labels, inline countdown, "Send answers"), with Note moved next to Add.
- A dot on the Note icon shows an open or non-empty note.
- react-icons is a new webui dependency (Lucide paperclip for Add, Phosphor note-pencil for Note); the emoji icons in the footer are replaced. Other emoji in the webui are left as they are.

**CLI.** `gx server workspaces` (list, create, rename, delete, reorder, move, remove) and `gx server hold` / `gx server unhold <epic>`, all with `--json`. Argument shapes, flag names and table output are the implementer's to pick; they follow the existing `gx server` verbs.

**Test seams for fixture and gallery.** A new fixture scenario seeds several workspaces, a Design, an implementation epic with agents, a chat, a held Design and a held epic, a One-off agent and a project-level Fix agent (for the Other section), and one card per section. The implementer names it. The gallery gets the sidebar (each device type, with the Other section), the Manage workspaces dialog, the PageHeader, the Design footer states and the jump menu rows.

## Testing Decisions

- A good test drives a public seam and checks what a caller sees: the JSON of `/v1/epics`, the CLI output, the rendered page by role and visible text, and the real browser only for what needs it. It never checks internal helpers, store file layout or class names.
- **Seam 1, server (the highest seam).** The `/v1/epics` route and the workspace and hold routes through `servertest`. Table tests: section rules with held cards (a held card lands only in `on-hold`, never in Needs you or the badge count), workspace name per card, `-impl` epic inheriting its Design's workspace and hold, Default fallback, name uniqueness, delete-moves-to-Default, Default undeletable, chat and epic cleanup, hold refused on a queued epic and on a Design whose `-impl` epic is queued, queueing clearing the hold, Promote to Design starting the Design in its chat's workspace, remove-from-workspace as a no-op on Default members, a held Design skipping auto-advance, hold exiting live Design sessions on the web and CLI paths and by the `-impl` address (the CLI refusing without `--yes`, `--json` listing the ended sessions), starting a Design session on a held Design clearing the hold, and `workspaces-changed` after each write. Prior art: `server/epics` list and state tests, `servertest` fixtures.
- **Seam 2, CLI.** The `gx server workspaces`, `hold` and `unhold` verbs, including `--json`, run against a test server. Prior art: the `gx server epics` and parity tests.
- **Seam 3, web unit (vitest, jsdom).** Component behaviour given props or an injected fetcher: device type from width, sidebar rows, nesting, status words and switcher, jump menu contents for empty and typed filters, shortcut handling (Alt chords, sidebar focus, vim keys, off in text fields), Design footer variants by width, the "..." menu's disabled Promote item, the Epics page workspace filter and On hold section. Prior art: `JumpMenu.test.tsx`, `useWide.test.tsx`, `AgentsPage.promote.test.tsx`, `Composer.test.tsx`.
- **Seam 4, browser (Playwright).** Per the gx-playwright-e2es rules, no render-only flows; extend before adding. Port the flows whose navigation steps change: `jump.e2e.ts` ("22. Back link count, then the phone layout"), `agents-sidebar.e2e.ts` ("73. Agents sidebar"), `modes.e2e.ts` (wide layout and popover jump menu), `phone.e2e.ts` ("24. Phone, Design page"), `united.e2e.ts` ("16. Needs you"), `design.e2e.ts` ("Auto-advance"). Add at most one new flow, "Workspaces", on the new scenario (it also covers the Other section and the Manage workspaces dialog): it needs the SSE round trip (`workspaces-changed` between two pages without a reload), real keyboard focus and chords, and the 744px layout, which only a browser sees. Update `CATALOG.md` and `maxFlows` in step with the code and say in the commit which vitest tests were considered.
- Look checks use `gx-ui-check` against the fixture, driven from the Mock states list below.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`). "The new scenario" is the workspaces fixture scenario above (name chosen by the implementer); "design" is the existing `design` scenario with `design-epic`. Catalog flows are in `webui/e2e/CATALOG.md`. Look checks use `gx-ui-check` and the Mock states list at the end.

1. Story 1: flow "73. Agents sidebar" (ported) and the new flow "Workspaces": the sidebar is on the Epics, Epic and Design pages after in-app navigation. New flow.
2. Story 2: "73. Agents sidebar" ported; the Agents page has no separate session list. Mock states `design-desktop`.
3. Story 3: new flow "Workspaces": switching workspace changes the rows. Mock states `design-desktop`, `design-desktop-workspaces`.
4. Story 4: Mock states `design-desktop`, `design-desktop-workspaces`.
5. Story 5: Mock states `design-desktop-workspaces`; `gx server workspaces list --json` shows each workspace with a Needs-you count.
6. Story 6: Mock states `design-desktop`; vitest row rendering. Look check only.
7. Story 7: Mock states `design-desktop`; the Design page links to its design epic (flow "50. Design epic page", extended).
8. Story 8: new flow "Workspaces": the chevron expands an epic's agents and the open epic starts expanded. Mock states `design-desktop`.
9. Story 9: new flow "Workspaces": a chat row appears in its workspace. Look check on `design-desktop`.
10. Story 10: Mock states `design-desktop`, `design-desktop-collapsed`.
11. Story 11: Mock states `design-desktop-new`, `design-desktop`.
12. Story 12: Mock states `design-desktop-gear`; flows "129. Restart server keyboard" and "130. Restart dialog confirms" still pass through the gear menu (ported); "108. Change password" reaches Settings from the gear menu.
13. Story 13: flow "81. Tray popover (and 82. Tray offline)" (ported to the footer): the disconnected icon shows only while offline. Vitest: the warning (bad health), paused and disconnected icons, each shown when it applies and hidden otherwise, with its tooltip text; the paused icon means the Server queue is paused. Mock states `design-desktop`.
14. Story 14: flows "Header shape" and "Epic description popover" (ported to the shared PageHeader); no top bar at 1440px and 375px.
15. Story 15: Mock states `design-desktop`, `design-desktop-collapsed`; flow `modes.e2e.ts` (ported).
16. Story 16: Mock states `design-tablet-open`, `design-tablet-closed`; new flow "Workspaces" at 744px: picking a row closes the sidebar.
17. Story 17: Mock states `design-phone-open`, `design-phone-closed`; flow "24. Phone, Design page" (ported).
18. Story 18: Mock states `design-tablet-closed`, `design-phone-closed`, `epic-phone-closed`, `agent-phone-closed`, `design-desktop-collapsed`.
19. Story 19: Mock states `design-phone-closed` (dot when something needs me); vitest: the dot follows the server's total Needs-you count across all workspaces, held items excluded, and equals the Epics tab badge.
20. Story 20: vitest on the sidebar, jump menu and page headers: the visible text says "Chat" and "Chats".
21. Story 21: `gx server workspaces list --json` after creating items: every Design, epic and chat has a workspace; one-off agents, project-level Fix agents and agents of a one-offs epic are given none (the Other section is checked in verification 79).
22. Story 22: `gx server workspaces list --json` shows a new Design under the Default workspace. Servertest.
23. Story 23: `gx server workspaces create "gx bugs"` exits 0; creating `GX BUGS` exits non-zero with an error naming the clash.
24. Story 24: `gx server workspaces rename` exits 0 and the sidebar switcher shows the new name without reload (new flow "Workspaces"); in the Manage workspaces dialog, rename in place works on the web and the Default row cannot be renamed.
25. Story 25: `gx server workspaces delete` exits 0 and members show under the Default workspace; the epics and agents still exist. On the web, the Manage workspaces dialog deletes behind a confirmation that names how many members go back to Default (new flow "Workspaces").
26. Story 26: `gx server workspaces delete` on the Default workspace exits non-zero; on the web the Default row in the Manage workspaces dialog has no delete.
27. Story 27: `gx server workspaces reorder` changes the switcher order and the order Alt+Shift+number follows (new flow "Workspaces"); the up and down buttons in the Manage workspaces dialog do the same on the web.
28. Story 28: new flow "Workspaces": Move to workspace from a sidebar row, an Epics card and a jump menu row. Mock states `design-desktop-row-menu`.
29. Story 29: new flow "Workspaces": Remove from workspace asks for confirmation and the item shows under the Default workspace; the action is hidden on Default members; `gx server workspaces remove` exits 0, and exits 0 as a no-op on a Default member (servertest).
30. Story 30: `/v1/epics` (servertest): the `-impl` epic's card carries its Design's workspace name; `gx server workspaces move` on the Design moves both.
31. Story 31: servertest: moving a Queue agent's epic moves the agent row; `gx server workspaces move` on an agent address exits non-zero.
32. Story 32: `gx server workspaces move` on a one-off agent, project-level Fix agent or follow-up address exits non-zero; Other section rows offer no Move or Hold; a follow-up has no sidebar row (it shows on the Epics page only).
33. Story 33: servertest: a finished epic keeps its workspace after the epic finishes.
34. Story 34: servertest: deleting an epic and exiting a chat session each drop the membership.
35. Story 35: servertest: moving an item changes no ticket or epic field (the `/v1/epics` row differs only in workspace name).
36. Story 36: new flow "Workspaces": opening an item in another workspace switches the sidebar; opening a held item does not.
37. Story 37: new flow "Workspaces": after reload-free navigation the active workspace stays; vitest on the localStorage read, including the deleted-workspace fallback and the load or deep-link rule (open item's workspace wins unless held, then the stored one, then Default).
38. Story 38: new flow "Workspaces": a CLI change shows in an open page with no reload (`workspaces-changed`).
39. Story 39: `gx server hold <epic>` exits 0 and `gx server epics --json` shows the card in `on-hold`.
40. Story 40: new flow "Workspaces": hold from the card menu, sidebar row menu, page header and jump row. Mock states `design-desktop-row-menu`.
41. Story 41: servertest with `WithTestHolder`: holding a Design with a live session exits it on the web and CLI paths and auto-advance does not fire; other agents on a held epic keep running; `gx server hold` on a Design with live sessions exits non-zero without `--yes`, and with `--yes` exits 0 and prints the sessions it ended; new flow "Workspaces" shows the web confirmation. Servertest: holding the joined card by its `-impl` address (Ready to queue, not queued) exits the Design's live sessions under the same rule: non-zero without `--yes`, and with `--yes --json` exits 0 and lists the ended sessions.
42. Story 42: `gx server unhold <epic>` exits 0, no session starts, and the web "Take off hold" asks no confirmation.
43. Story 43: Mock states `design-desktop` (collapsed On hold section); new flow "Workspaces".
44. Story 44: servertest: Needs-you cards and the Epics tab badge exclude a held card that would otherwise need me; flow "16. Needs you" extended.
45. Story 45: `gx server hold` on a queued loop epic exits non-zero with "Remove it from the queue first"; the web action is disabled with that reason.
46. Story 46: servertest: queueing a held epic clears its hold; `gx server epics --json` no longer shows it in `on-hold`.
47. Story 47: `/v1/epics`: holding a Design puts its `-impl` epic card in `on-hold` too.
48. Story 48: Mock states `design-desktop` (held rows dimmer) via look check; vitest.
49. Story 49: flow `jump.e2e.ts` (ported): Cmd/Ctrl+K opens the menu on the Epics page and on the Agents page. Mock states `design-desktop-jump`.
50. Story 50: `jump.e2e.ts` extended: the empty menu shows the active workspace, then Needs you from elsewhere; nothing else. Mock states `design-desktop-jump`.
51. Story 51: `jump.e2e.ts` extended: typing finds a held item, a Ready to design item, a Finished item and an agent.
52. Story 52: `jump.e2e.ts` extended: an agent row names its epic, an outside row shows the workspace name, a held row says "On hold".
53. Story 53: flow "22. Back link count, then the phone layout" (ported); the page title is not a button; the Agents page has no session switcher or Queue/Design toggles.
54. Story 54: flow `modes.e2e.ts` at 744px: the jump menu is a popover.
55. Story 55: new flow "Workspaces" at 390px: an item outside the active workspace opens through the switcher.
56. Story 56: flow "16. Needs you" ported: cards from several projects remain.
57. Story 57: `/v1/epics` (servertest): every Design and epic card carries its workspace name; One-off and Follow-ups cards carry none and offer no Move or Hold; flow "18. Joined card" shows it. Vitest: the workspace filter shows One-off and Follow-ups cards only under All workspaces; they count in the Epics badge and mascot dot but in no switcher count; a waiting One-off agent is in the empty jump menu's Needs you section.
58. Story 58: vitest on the Epics page: the filter defaults to All workspaces, filters locally and is not stored on the server.
59. Story 59: flows "2. Epic looks right" and "16. Needs you" (ported): section order is unchanged.
60. Story 60: `gx server epics --json` section `on-hold`; vitest: the section is last, collapsed, has a count, and the text filter opens it.
61. Story 61: vitest on the device hook at 639, 640, 991 and 992px; flow `modes.e2e.ts`.
62. Story 62: new flow "Workspaces" at 744px visits Epics, Epic, Ticket, Design, Agents, Stats and Settings, and none scrolls sideways; `gx-ui-check` look check of each page at 744px.
63. Story 63: new flow "Workspaces": Alt+1 opens the first row and rows show numbers while Alt is held. Vitest: Alt+1 to 9 count only the workspace's own rows above On hold; held and Other rows get no number.
64. Story 64: new flow "Workspaces": Alt+Shift+2 picks the second workspace.
65. Story 65: new flow "Workspaces": Cmd/Ctrl+B toggles and focuses the sidebar.
66. Story 66: new flow "Workspaces": j/k/h/l and arrows move, expand and open; vitest: vim keys do nothing off the sidebar; j/k and the arrows move through every visible row, including an expanded On hold section and the Other section.
67. Story 67: vitest: no shortcut fires in a text field except Cmd/Ctrl+K.
68. Story 68: the agent page "..." menu shows "Promote to Design" disabled with a reason before the first message and enabled after; vitest in `AgentsPage.promote.test.tsx` (ported); `agents-session.e2e.ts` checks the header has no standalone button.
69. Story 69: Mock states `design-phone-footer-empty`, `design-phone-footer-320`; flow "24. Phone, Design page" extended.
70. Story 70: Mock states `design-phone-footer-answered`, `design-phone-footer-auto`.
71. Story 71: Mock states `design-phone-footer-empty`, `design-phone-footer-answered`.
72. Story 72: Mock states `design-phone-footer-note`.
73. Story 73: Mock states `design-phone-footer-countdown`, `design-phone-footer-320-countdown`; flow "Auto-advance" (design.e2e.ts) at 390px.
74. Story 74: Mock states `design-tablet-footer`, `design-tablet-footer-countdown`.
75. Story 75: Mock states `design-phone-footer-empty`, `design-tablet-footer`; vitest renders the icons (no emoji characters).
76. Story 76: `gx server workspaces list --json` exits 0 and prints the JSON the route sends; each verb (create, rename, delete, reorder, move, remove) exits 0 on success and non-zero on a bad name.
77. Story 77: `gx server hold <epic> --json` and `gx server unhold <epic> --json` exit 0 and print the new state.
78. Story 78: servertest or fixture restart: after `gx server restart`, `gx server workspaces list --json` is unchanged and `gx server epics --json` still has the held card (flow "130. Restart dialog confirms" scenario can be extended).
79. Story 79: new flow "Workspaces": the Other section sits below On hold, is the same after switching workspace, lists a One-off agent, a project-level Fix agent and an agent of a one-offs epic (a Fix agent on a One-off ticket), offers no Move or Hold, and opening a row leaves the active workspace unchanged; a Fix agent on a ticket and an Investigate agent of another epic show nested under their epic. Mock states `design-desktop`.
80. Story 80: new flow "Workspaces": the Manage workspaces dialog lists workspaces in order, renames in place, reorders with up and down, and deletes behind a confirmation naming the member count; the Default row has no rename or delete. Vitest on the dialog. Gallery entry.
81. Story 81: servertest: holding a Design whose `-impl` epic is queued is refused with "Remove it from the queue first"; `gx server hold` exits non-zero with that message; the web action is disabled with that reason.
82. Story 82: servertest: a Design made by Promote to Design from a chat in a named workspace is in that workspace, and `gx server workspaces list --json` shows it there.
83. Story 83: vitest on the active-workspace read (see story 37): the open item's workspace wins when the item is not held; a held item leaves the stored workspace; a deleted stored workspace falls back to Default; a page with no workspace item (Epics, Stats, Settings, an Other row, a One-off or Follow-up ticket) keeps the stored workspace; a Ticket or agent page resolves to its epic's workspace.
84. Story 84: servertest: starting a Design session on a held Design clears the hold on the web Continue and CLI paths; `gx server epics --json` no longer shows the card in `on-hold`. The TUI path calls the same server code and is not tested separately.
85. Story 85: vitest (see verification 83): the Ticket and agent page cases and the no-workspace cases.
86. Story 86: new flow "Workspaces": deleting the active workspace (from the CLI, as another device) while a page is open drops the sidebar to the Default workspace with no reload. Vitest: the same fallback on a `workspaces-changed` refresh.
87. Story 87: vitest, as in verifications 63 and 66.
88. Story 88: servertest, as in verification 41 (the `-impl` address case).

New flows or scenarios to write (these become implementation tickets):

- The new fixture scenario (name chosen by the implementer): workspaces "Default workspace", two named workspaces, a Design, an implementation epic with Design and Queue agents, a chat, a held Design, a held epic, a One-off agent, a project-level Fix agent, and one card per Epics page section. The scenario also gets an option that leaves one project with a failed ticket migration, so it can seed a project-level Fix agent (the fixture startup marks every project fully migrated today).
- The new flow "Workspaces" on that scenario (one flow, one journey; the only new Playwright flow).
- Gallery entries for the sidebar (with the Other section), the Manage workspaces dialog, PageHeader, jump menu rows and Design footer states.
- Ports of the existing flows named above.

**Mock states**

- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop`, 1000px, the new scenario opened on a Design page: sidebar, sidebar-brand, workspace-switcher, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-workspaces`, 1000px, the new scenario with the switcher open: sidebar, sidebar-brand, workspace-switcher, menu, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-new`, 1000px, the new scenario with the "+ New" menu open: sidebar, sidebar-brand, workspace-switcher, menu, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-gear`, 1000px, the new scenario with the gear menu open: sidebar, sidebar-brand, workspace-switcher, menu, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-row-menu`, 1000px, the new scenario with a row menu open: sidebar, sidebar-brand, workspace-switcher, menu, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-jump`, 1000px, the new scenario with the jump menu open and an empty filter: sidebar, sidebar-brand, workspace-switcher, jump-menu, menu, current-row, expand-chevron, new-button, sidebar-footer, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-desktop-collapsed`, 1000px, the new scenario with the sidebar hidden: header-toggle, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-tablet-open`, 744px, the new scenario: sidebar, sidebar-brand, workspace-switcher, current-row, expand-chevron, new-button, sidebar-footer, header-toggle, scrim, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-tablet-closed`, 744px, the new scenario: header-toggle, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-phone-open`, 390px, the new scenario: sidebar, sidebar-brand, workspace-switcher, current-row, expand-chevron, new-button, sidebar-footer, header-toggle, scrim, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `design-phone-closed`, 390px, the new scenario on a Design page (its two-row footer is replaced by mock 09): header-toggle, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `epic-phone-closed`, 390px, the new scenario on an implementation epic page: header-toggle, page-header.
- `$(gx tickets root)/better-nav/mocks/07-unified-sidebar.html`, `agent-phone-closed`, 390px, the new scenario on an agent page: header-toggle, page-header.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-empty`, 390px, the `design` scenario with no answers: page-header, question-card, footer, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-answered`, 390px, the `design` scenario with one answer: page-header, question-card, footer, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-note`, 390px, the `design` scenario with a note open: page-header, question-card, note-field, footer, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-auto`, 390px, the `design` scenario with the Auto popover open: page-header, question-card, footer, auto-popover, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-countdown`, 390px, the `design` scenario with all answers and Auto-advance counting down: page-header, question-card, footer, countdown-strip, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-320`, 320px, the `design` scenario with one answer: page-header, question-card, footer, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-phone-footer-320-countdown`, 320px, the `design` scenario with the countdown: page-header, question-card, footer, countdown-strip, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-tablet-footer`, 744px, the `design` scenario with one answer: page-header, question-card, footer, add-button, note-button, auto-button, send-button.
- `$(gx tickets root)/better-nav/mocks/09-design-phone-footer.html`, `design-tablet-footer-countdown`, 744px, the `design` scenario with the countdown: page-header, question-card, footer, add-button, note-button, auto-button, countdown-text, cancel-button, send-button.

## Out of Scope

- The TUI, including its ctrl+o view switcher. The person switches between Designs and queue epics in the web UI.
- Renaming the word "Epic".
- A search button or "Jump to..." row in the sidebar (a search row at the top of the sidebar is only a fallback if the iPad needs it).
- Recent or pinned items in the jump menu.
- Keyboard shortcuts for hold or move to workspace, a shortcuts settings screen and key remapping.
- A new pause kind for hold (ADR 0042).
- An offline fallback when the server is down.
- Separate tablet page layouts.
- Replacing emoji outside the Design page footer.
- Per-person state beyond workspaces, membership and holds (the active workspace stays in localStorage).

## Further Notes

- The earlier map decisions were changed along the way. Final rules: the sidebar is a workspace switcher showing one workspace at a time (not a tree of all workspaces); every item is in exactly one workspace and the Default workspace is where new items land and where removal sends them; there is no top bar and no search button on any device; the phone Design footer is the one-row layout of mock 09, not the two-row layout of mock 07.
- Hold and the queue: a loop epic cannot be held while queued, because a held, queued epic would leave a Server queue entry that never runs.
- Mock 07's row menu shows "Remove from workspace" unconditionally; the spec overrides it (hidden on Default members). Mock 07 has no Other section or Manage workspaces dialog; they follow the sidebar's look.
- Stored mocks (`mocks/07-unified-sidebar.html` and `mocks/09-design-phone-footer.html`) are the approved look. The implementer matches them; where a mock and this spec differ, the mock wins on look and this spec wins on behaviour.
- Breakdown decisions: Fix agent sessions record what they fix (a new wire field) so the sidebar can place them, and nothing parses names. A chat whose session exited has no sidebar row; jump search still finds it. The Needs-you count lives on the footer Epics icon: the server total, held items left out, no "other than this page" exclusion, the same number as the mascot dot. Only Designs and epics can be held; chats can move and be removed. A switcher count is a workspace's non-held Needs-you cards. Row order follows mock 07, else the server's card order and then chats by creation. On the phone footer the main button (Continue, Interrupt, Send answers) takes the same wide slot as Send. Taking a hold off from a card is a `CardActionUnhold` labelled "Take off hold".
- The unified sidebar mock was also published as an artifact with the rejected variants: https://claude.ai/artifact/HyQJuKUD4EJmqX2oNjobnV. The footer mock: https://claude.ai/artifact/NDDV9WSNxXcNqLRuRZAKt6.
