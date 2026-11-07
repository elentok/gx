# Better navigation fixes: sidebar, menus, footer and mobile shell

## Problem Statement

The sidebar, footer and menus that shipped with better-nav (docs/specs/better-nav.md) have rough edges
that I hit every day.

- Clicking an epic row also expands it, so I cannot open a page without the list shifting. A Design row
  opens the design epic page when I expect the Design page.
- Pressing Enter on a chat opens it but I still have to click the prompt. A chat's "..." menu has no Stop
  or Remove; I must open the agent page first.
- Holding Alt makes row text flicker and, I think, refreshes more of the page than the sidebar.
- Sidebar rows have no order I can predict, and I cannot choose one per workspace.
- The sidebar looks the same as the page. The keyboard focus outline is white, square and ugly.
- Menus differ in look and behaviour. "Move to workspace" opens a modal, "Remove from workspace" is a
  second way to do the same job, and on the phone menus open as a bottom sheet far from the button I
  tapped. Menus and popovers inside the sidebar stay open after it closes.
- "New design" and "New chat" ignore the workspace I am in. "Fix it with an agent" starts the agent and
  leaves me where I was.
- The gear menu mixes unrelated things (switches, Restart server, Claude and Codex limits, the build).
- On the iPhone and iPad, returning to the web app shows a stale view. There is no search button on the
  phone, and no swipe to open or close the sidebar.

## Solution

A batch of fixes, built as one `better-nav-fixes-impl` epic:

- Epic rows expand only by their chevron; a Design row opens the Design page; Enter on a chat focuses its
  prompt; chat rows get Stop and Remove; Alt numbers show in the row's menu-button slot without touching
  anything else; a Sort setting, kept per workspace, orders the sidebar (needs you first by name, newest
  activity first, or alphabetical) and rows reorder live as data changes while keyboard focus stays on the
  same row.
- One standard Menu component serves every menu, with submenus: a side submenu on tablet and desktop, an
  anchored 280px popover with 48px rows and an in-place submenu with a "Back" row on the phone. "Move to
  workspace" becomes a submenu listing every workspace; "Remove from workspace" leaves the web UI.
- The gear menu holds AFK, Vi mode, Sort (the active workspace's, named in its label), Restart server and "More settings...". Claude and Codex limits
  move to a new limits tray button of two small bars. The sidebar gets its own background colour. Fix
  agent rows get a wrench icon. One global focus ring (a gap, then a blue ring) shows for keyboard focus
  only.
- New design and New chat (from the sidebar, the Epics page and the Agents page) start in the active
  workspace. "Fix it with an agent" opens the new agent's page.
- A search button in the sidebar header opens the jump menu on every device. Swiping left closes the
  sidebar on phone and tablet; swiping right from the left edge opens it in the home-screen app. Coming
  back to the app restarts the event stream and refetches. Closing the sidebar closes the menus and
  popovers opened from it. A form or dialog started from one of its menu items stays open until the person
  closes it, and the sidebar cannot be closed while it is open.

## User Stories

Sidebar rows

1. As a person, I want clicking or tapping an epic row to open its page without expanding it, so that the list does not shift when I only want to navigate.
2. As a person, I want an epic's row to expand or collapse only by its chevron, so that expanding is a deliberate act.
3. As a person using the keyboard, I want Right/l to expand and Left/h to collapse the focused epic row, as today, so that keyboard users keep their way.
4. As a person, I want opening an epic's page to leave its row as I left it (expanded or collapsed), so that opening a page never expands or collapses a row on its own. (Rows may still reorder when their data changes, see story 55.)
5. As a person, I want a Design row to open the Design page (its design session), so that a click lands where I work on the Design.
6. As a person, I want the Epics page card link for a Design to stay as it is, so that only the sidebar row changes target.
7. As a person using the keyboard, I want Enter on a chat row to open the chat and focus its prompt input, on every device, so that I can type at once.
8. As a person using a mouse or touch, I want a click or tap on a chat row to open the chat without focusing the prompt, so that a tap on a phone does not pop up the keyboard.
9. As a person, I want Stop in a chat row's "..." menu while the session is live, so that I can end a chat without opening it.
10. As a person, I want Remove in a chat row's "..." menu, with the agent page's confirmation, so that I can delete a chat from the sidebar.
11. As a person, I want removing the open chat to move me to another page, so that I do not sit on a dead chat.
12. As a person, I want nested agent rows to stay without a menu, so that nothing else changes there.
13. As a person, I want Alt+1 to Alt+9 numbers to appear in the row's menu-button slot while Alt is held, so that the row text never changes or flickers.
14. As a person, I want the status words to never change while Alt is held, so that the row stays readable.
15. As a person typing in a text field, I want Alt to do nothing there (I use Alt+arrows to move between words), so that typing is never disturbed.
16. As a person, I want holding Alt to cause no refresh of the page content outside the sidebar, so that nothing else on the screen moves or reloads.
17. As a person, I want a Sort submenu in the gear menu with "Needs you first, then by name" (default), "By last activity, newest first" and "Alphabetically by name", showing and setting the active workspace's value, with the active workspace's name in its label (for example "Sort · research"), so that I can choose the order I scan in each workspace and see which one I am changing.
18. As a person, I want each workspace to keep its own sort value, the same on every device, so that a change on one device shows on the others without a reload, switching workspace switches the order, a new workspace starts at the default, renaming a workspace keeps its value and deleting it drops the value.
19. As a person, I want the active workspace's order to apply to every list the sidebar shows for it (its own rows, its On hold rows, an epic's nested agents and the Other section), so that nothing looks unsorted.
20. As a person, I want Alt+1 to Alt+9 to follow the sorted order, so that the number I see is the row I open.

Menus and workspaces

21. As a person, I want one standard Menu component behind every menu in the web UI (sidebar row menus, "+ New", gear, workspace switcher, page header "..." menus, Epics card menus), so that menus look and behave alike.
22. As a person on tablet or desktop, I want a submenu to open to the side of its item, so that I see the parent list and the options together.
23. As a person on a phone, I want a menu to open as an anchored popover, 280px wide with 48px rows, below the button I tapped or above it when there is no room, kept 8px inside the screen with its right edge on the button's, so that the menu opens next to my thumb. Only menus lose the bottom sheet; forms (such as the phone New chat form) keep their layout.
24. As a phone person, I want a light dim behind the menu and a tap outside to close it, so that I can dismiss it.
25. As a phone person, I want a submenu to swap into the same panel with a "Back" row and a divider at the top, so that the panel does not move.
26. As a person using the keyboard, I want every menu to work with the arrow keys, Enter, Escape and focus returning to its button, so that the component is a base for full keyboard control later.
27. As a person, I want "Move to workspace" to be a submenu listing every workspace, the Default workspace included, with the current one marked, so that I move an item without a modal.
28. As a person, I want that submenu on a sidebar row, an Epics card and a jump menu row, so that I can move an item wherever I see it.
29. As a person, I want "Remove from workspace" and its confirmation gone from the web UI (Move to the Default workspace does the job), so that there is one way to undo a placement.
30. As a person using scripts, I want `gx server workspaces remove` to stay, so that existing scripts keep working.
31. As a person, I want "New design" from the "+ New" menu to create the Design in the active workspace, so that it lands where I am.
32. As a person, I want "New chat" to create the chat in the active workspace, so that it lands where I am.
33. As a person, I want every web create entry point (the sidebar "+ New", the Epics page creates and the Agents page New session form) to use the active workspace, so that the page I create from does not matter.
34. As a person using the CLI or TUI, I want creates to keep landing in the Default workspace, so that scripts behave as before.

Footer, look and focus

35. As a person, I want the gear (Settings) menu to hold AFK, Vi mode, Sort, Restart server and "More settings...", so that its items are related.
36. As a person, I want "More settings..." to open the Settings page, so that the full settings are one click away.
37. As a person, I want the build string out of the gear menu (the sidebar header shows the version), so that the menu holds only actions.
38. As a person, I want a limits tray button between the health meter and the gear, showing two vertical bars labelled C and X, so that I see Claude and Codex limits at a glance.
39. As a person, I want each bar to fill with that provider's 5h usage (at least a sliver) and turn green below 80%, amber from 80% and red at 100%, so that I see trouble coming.
40. As a person, I want clicking the tray to open a popover with the Claude and Codex limits (5h and weekly each, with reset times) and, in spending mode, the spend, so that no information is lost from the old menu.
41. As a person, I want the sidebar to have its own background (`#11161d`, the page stays `#0a0d11`) with row hover and current tints adjusted, so that the sidebar reads as a separate panel.
42. As a person, I want Fix agent rows to show a wrench icon in the row's status colour, so that I tell them from chats and Queue agents.
43. As a person using the keyboard, I want one global focus ring on every focusable control, a 2px gap in the surrounding surface colour and a 2px blue ring that follows the control's rounded corners, so that focus is clear and not ugly.
44. As a person using the keyboard, I want the ring on full-width sidebar rows drawn inset with 8px corners, so that it is never clipped.
45. As a mouse or touch person, I want the ring to show for keyboard focus only, so that a click never draws it.

Search, mobile and refresh

46. As a person, I want a search icon button in the sidebar header that opens the jump menu on every device, with a tooltip naming Cmd/Ctrl+K, so that I can search on a phone.
47. As a phone or tablet person, I want a swipe left on the open sidebar or its overlay to close it, so that I close it with my thumb.
48. As a home-screen app person, I want a swipe right from the left edge to open the sidebar, so that I open it with my thumb.
49. As a Safari person, I want the edge swipe to leave the sidebar alone, so that Safari's back gesture still works.
50. As a touch person, I want a swipe to count only after about 40px of mostly horizontal movement, so that a tap or a scroll never opens or closes the sidebar by accident.
51. As an iPhone or iPad person, I want the app to restart its event stream and refetch the snapshot and page data whenever it becomes visible again, so that I never see a stale view.
52. As a person, I want closing the sidebar (toggle, Cmd/Ctrl+B, scrim, swipe or picking a row) to close every menu and popover opened from inside it, and a form or dialog started from a sidebar menu item to stay open, with the sidebar open behind it, until I close it, so that nothing is left floating and nothing I am filling in vanishes. While such a form or dialog is open the sidebar cannot be closed: a backdrop tap, a swipe and Cmd/Ctrl+B act only on the dialog, by the dialog's own rules. A submit that navigates (New chat, New design) closes the dialog and then the sliding sidebar, like picking a row. A menu item that navigates ("More settings...") closes a sliding sidebar (phone and tablet), like picking a row.
53. As a person, I want "Fix it with an agent" to close its dialog and open the new agent's page once the agent starts, from a ticket card and from the failed-migration strip, and to stay open showing the reason when the server refuses, so that I land in the agent or see why not.
54. As a person, I want that agent to stay under its epic and the workspace to follow the epic, even when the agent's page opens before the sidebar knows the agent's placement, so that workspaces stay as the better-nav rules say.
55. As a person, I want sidebar rows to reorder live as their data changes (for example a chat's status flips to Needs you) while my roving keyboard focus stays on the same row, so that the order is always current and the keyboard never jumps to a different row. Focus follows the row's key while it is rendered, including into On hold. When the row is gone (moved to another workspace from another device or the CLI, removed, or swapped out by a workspace switch), focus goes to the row now at the old index, clamped to the list, or to the list itself when it is empty. A row menu that is open stays attached to its row and repositions as the row moves, and closes if the row leaves the list; only the open menu waits, the order still changes at once.

## Implementation Decisions

**Vocabulary.** Terms are in GLOSSARY.md: Workspace, Default workspace, Active workspace, On hold, Other section, Chat session, App shell, Tray, Device type, Fix agent. "New chat" is the sidebar "+ New" item that creates a Chat session, labelled "New chat" everywhere (stories, Verification, menus); the CATALOG wording for flows 85 and 88 is refreshed when they are ported. No new workspace or hold concept is added. The impl epic updates GLOSSARY.md: the Other section's contents are the same in every workspace, but its order follows the active workspace's Sort (the entry no longer says it is "the same in every workspace"), and the Workspace entry gains "each keeps its own Sort".

**Schema.** None for tickets and epics: no frontmatter changes, so no migration step. The Sort value is stored per workspace in the workspaces state file (`workspaces.json`), which stores workspaces as a bare `order` list and so has no per-workspace object. The file gains one optional top-level map from workspace name to sort value, beside `members`. Rename re-keys the entry and Delete drops it, both inside the same `write` that updates `members`. A missing map, a missing entry or an unknown value reads as the default, so no migration is needed and `order` is not restructured. No pause kind is added (ADR 0042).

**Sort setting (per workspace).**
- Each workspace has its own sort value. Default "Needs you first, then by name"; a new workspace starts at the default; deleting a workspace drops its value; renaming keeps it. It lives with the workspace store (better-nav) and is part of the workspaces snapshot the web already reads. A change publishes the existing workspaces-changed event, so other devices update without a reload. The write route and a CLI verb share one code path (the route/verb parity test needs the verb) and take a workspace name, mirroring the existing workspace verbs; the implementer names the route, verb and field.
- Write validation: the write refuses an unknown sort value (a new Reason that lists the valid values) and an unknown workspace name (`ReasonUnknown`) and stores nothing. This differs from creates, where an unknown name reads as Default; the Sort write must not copy that. The CLI exits non-zero with the reason.
- CLI shape: `<verb> <workspace>` prints that workspace's value; `<verb> <workspace> <value>` sets it and prints the new value. `gx server workspaces list --json` carries each workspace's sort. The values are stable machine names (for example `needs-you`, `activity`, `name`) used in the state file, the wire and the CLI; labels exist only in the web. The implementer picks the names.
- Three values: "Needs you first, then by name" (default), "By last activity, newest first" and "Alphabetically by name" (case-insensitive, ties kept in server order). On read, unknown or missing is the default.
- The gear menu's Sort submenu shows and sets the Active workspace's value, and its label names that workspace (for example "Sort · research"), so a change is not mistaken for a global one. The submenu is disabled until the active workspace resolves (the snapshot and the workspace list have loaded), so a pick never writes to "" or to a workspace another device just deleted or renamed. `buildSidebar` uses the default sort when the active workspace has no value. A refused write shows the same error toast as the other workspace writes, and the radio stays on the stored value. While a workspace is active, its sort orders everything the sidebar shows for it: its own rows, its On hold rows, an epic's nested agents and the Other section. Alt+1 to Alt+9 follow it. Switching workspace switches the order.
- Ordering lives in the sidebar model (`buildSidebar`), the one place that builds rows, so the view and Alt+1 to Alt+9 read the same list. It orders a workspace's own top-level rows (Designs, epics and chats together, replacing "cards then chats").
- Needs-you-first: rows whose status is "Needs you" first, then the rest; inside each group by name, case-insensitive, ties kept in server order. By-last-activity: newest first; a row with no activity time goes last, by name.
- Last activity needs no new wire field. Chats and agents already send `status_changed_at` (set on every status flip, and falling back to the creation time), and cards carry `last_activity_at`. A row's activity is the newest of its card's `last_activity_at` and the `status_changed_at` of its sessions (the Design's session; an epic's nested agents). The sidebar model computes it from the session placements for the card's addresses, whatever the row displays: `cardRow` passes no agents for On hold rows and for Designs, so reading `row.agents` would silently drop their session times. A Design with a busy session and no ticket activity sorts by the session time, and so does a held row whose session is newer than its card.
- Rows reorder live as the data changes, and reordering never waits for hover. Roving keyboard focus stays on the same row, tracked by key and not by position, so a status change that moves a row never moves focus to another one. Focus follows the key while that row is rendered, including into On hold. When the key is gone (moved to another workspace from another device or the CLI, held, removed, or swapped out by a workspace switch), focus goes to the row now at the old index, clamped to the list, or to the list itself when it is empty.
- An open row menu (the "..." popover, and the phone popover anchored to its button) stays attached to its row and repositions as the row moves; it closes if the row leaves the list. Only the open menu is protected: a row can still slide under a pointer just before a click, which is accepted for now (see Further Notes).
- The Sort setting orders only the sidebar. The Epics page, jump menu and the server queue order stay as they are.

**Menu component (one for every menu).**
- A single shared Menu component replaces `MoreActionsMenu` / `menuFrame`, the "+ New" menu, the gear menu, the workspace switcher and the page header "..." menus. Its API (items, checkbox and radio items, submenu items, disabled reasons, anchor, close signals) is the implementer's to choose. It must serve the later full-keyboard-control effort: arrow keys, Enter, Escape, typeahead is optional, focus returns to the trigger, the focus ring from this spec on its items.
- The Epics card menu and the jump menu row's actions also use it. The jump menu's own searchable list is a combobox, not an action menu, and keeps its present behaviour; only its row actions (Move, Hold) go through the Menu. The implementer decides how each existing menu is ported as long as behaviour does not change.
- Tablet and desktop: a submenu opens to the side of its parent item. Phone (from the approved phone menu prototype): an anchored popover 280px wide, 48px rows, opening below the tapped button and flipping above when there is no room, kept 8px inside the screen, its right edge on the button's right edge, a light dim behind it, a tap outside closes it. No menu bottom sheet remains; forms (the phone New chat form, for one) keep their layout. A submenu swaps its content into the same panel: a "‹ Back" row and a divider at the top, then the options; the panel keeps its place.
- "Move to workspace" is a submenu of every workspace in switcher order, Default included, the current one marked; choosing one moves the item and closes the menu. "Remove from workspace", its item and its confirmation dialog are deleted from the web UI. The `gx server workspaces remove` verb and its route stay.
- The gear menu's Sort is a submenu with three radio items on every device type, for the Active workspace, whose name is in the item's label.

**Sidebar rows and keyboard.**
- Rows: clicking the row (not the chevron) navigates only. `cardRow`'s `expanded` flag, set when the open page is the row's epic, is removed, and so is the effect that opens the row from it. The expanded set stays as the person left it while the page is open; where it is kept (component state or localStorage) is the implementer's choice, with the present behaviour on reload.
- Design row target: the sidebar's row href for a Design is the Design page route. `titleHref` is shared with the Epics page card, which keeps its link, so the sidebar stops using it for Designs (the sidebar model owns its own href).
- Enter on a chat row opens the chat and focuses the prompt input; click and tap open without focusing. How the intent reaches the page (route state, a query flag or a shared focus request) is the implementer's choice; it must work on a fresh navigation and when the chat is already open.
- Chat rows get a "..." menu with Stop (shown while the session is live) and Remove (with the agent page's confirmation, same wording). Both reuse the agent page's actions rather than copying them (`SessionActions`). Removing the open chat navigates to a page that exists (the implementer picks, matching what the agent page does today).
- Alt numbers: while Alt is held, the number shows in the row's menu-button slot (hidden until hover today). Rows render both the status words and the slot at all times; Alt toggles only the slot's content. Alt chords and the Alt-held state do nothing while a text field has focus. The Alt-held state must not live where a change re-renders the page tree: the implementer first reproduces the "wider page refresh" in a fixture (see Verification) and fixes the cause where it is found, not only the symptom in the row.

**Workspace-aware creates.**
- Three web entry points pass the Active workspace: the sidebar "+ New" (New design, New chat), the Epics page creates and the Agents page New session form. Two server routes gain an optional workspace name: the design create (`/v1/designs/open`) and the sessions create (`/v1/sessions/create`). A missing or no-longer-existing name reads as Default, as today. The design create's name applies only when a new Design is created: `FromSession` (promote, which puts the Design in its chat's workspace) wins over it, and reopening an existing Design through `/v1/designs/open` never changes its membership. CLI and TUI sends no workspace and keeps landing in Default. The membership logic stays in the server module that owns the store (better-nav); the client only sends the name.

**Footer, tray and sidebar look (from the sidebar look prototype).**
- Gear menu: AFK, Vi mode, Sort (submenu), Restart server, "More settings..." (opens the Settings page). The build string leaves the menu; the header keeps the version.
- Limits tray: a new tray button between the health meter and the gear, two vertical health-meter-style bars labelled C and X. Each fills with that provider's 5h usage (at least a sliver) and uses the limit tone: green below 80%, amber from 80%, red at 100%. In spending mode the bars still show limits; spend shows in the popover only. Clicking opens a popover with today's `LimitsBlock` content (Claude and Codex, 5h and weekly each, reset times) and, in spending mode, the spend. The old gear menu limits block and spend lines are removed. When there is nothing to show (no limits data and no spend) the tray follows today's rules for showing the block; the implementer applies the same rules and picks how a provider with no 5h window looks.
- Sidebar background: a new `sidebar` colour token, `#11161d`; the page stays `#0a0d11`. Row hover `rgba(255,255,255,0.06)`; current row `rgba(108,180,255,0.14)` with the 3px blue left edge. Menus and buttons keep the `surface.raised` fill (`#141920`).
- Fix agent rows: an 18px wrench in the status colour (amber needs you, blue working, green done). The Queue agent row stays without an icon and the chat keeps the pink bubble.

**Global focus ring (from the focus ring prototype).**
- One global `:focus-visible` rule, keyboard focus only. Controls: a 2px gap in the surrounding surface colour, then a 2px ring in the running blue (`#6cb4ff`), drawn as a two-layer shadow so it follows rounded corners and reads on the page and the sidebar background. Full-width sidebar rows: an inset 2px blue ring with 8px corners. It replaces the undefined `border.focus` token and the menu items' 2px running-tone outline. Surface colour for the gap comes from the control's surrounding surface (page or sidebar); the implementer picks the mechanism.

**Search, swipe and mobile refresh.**
- A search icon button in the sidebar header opens the jump menu on every device; its tooltip names Cmd/Ctrl+K (the platform's key). The button is a plain button that calls the same open action as the shortcut.
- Swipe: one gesture hook in the app shell. A swipe counts after about 40px of mostly horizontal movement. Swipe left on the open sidebar or its overlay closes it on phone and tablet. Swipe right from the left edge opens it only when the app runs as a home-screen app (`navigator.standalone` or `display-mode: standalone`), because in a Safari tab the edge swipe is back. On desktop, where the sidebar is docked, nothing swipes. Vertical scrolling, taps and horizontal scroll areas inside the sidebar (or a horizontally scrollable code block on the page) must not trigger it; the implementer defines the exact rules and tests them.
- Refresh on return: on `visibilitychange` to visible and on `pageshow`, the event stream is always restarted (not only when it looks dead), and the snapshot and page data are refetched. The refetch hook is shared (one resync signal from the connection that data hooks subscribe to), not per-page code. The implementer picks the shape.
- Closing the sidebar closes every menu and popover opened from inside it, however it closes (toggle, Cmd/Ctrl+B, scrim, swipe, picking a row). This includes the Tray popovers and the new limits popover. A form or dialog started from a sidebar menu item stays open, with the sidebar open behind it, until the person closes it. While one is open the sidebar cannot be closed: the dialog's backdrop sits over the scrim and takes the tap, a swipe lands on the dialog, and Cmd/Ctrl+B acts only on the dialog, by the dialog's own rules; the implementer scopes the sidebar's close paths accordingly. A submit that navigates (New chat, New design) closes the dialog and then the sliding sidebar, like picking a row. A menu item that navigates ("More settings...") closes a sliding sidebar (phone and tablet), like picking a row (`AppShell.tsx` closes it when not docked).
- "Fix it with an agent": on success (`/v1/tickets/fix-agent` returns the new session in the existing `SessionResult`; no wire change) the dialog closes and the page navigates to the agent, taking the id from that result. On refusal the dialog stays open and shows the reason. The navigation does not wait for the sidebar to learn the placement. `useActiveWorkspace` / `resolveActive` resolves a route only once, so a route whose item is unknown to the snapshot (not found, which differs from found with no workspace) stays pending and is applied once the placement arrives; otherwise the stored workspace would win and the epic's workspace would never become active. The "Agent started. View it in the gx TUI..." line goes. Both entry points navigate: `FixAgentDialog` on a ticket card, and the project-level Fix agent in the failed-migration strip (that agent goes to the Other section). The server places the agent under its epic (workspace follows the epic; a Fix agent of a one-offs ticket goes to the Other section and leaves the Active workspace unchanged), as today.

## Testing Decisions

- A good test drives a public seam and checks what a caller sees: the JSON of the routes and snapshot, CLI output, the rendered page by role and visible text, and the real browser only for what needs it. It never checks internal helpers, file layout or class names.
- **Seam 1, server (servertest).** Per-workspace Sort round trip in the workspace store (default, set per workspace, a new workspace starts at the default, rename keeps and delete drops the value, restart persistence, an old state file without the sort map loads as default, the workspaces-changed event, a write with an unknown value or an unknown workspace is refused and stores nothing), web creates with a workspace name (missing or unknown name reads as Default; for the design create, `FromSession` wins over the name and reopening an existing Design never changes its membership), the Fix-agent response. Prior art: the workspace store tests from better-nav, `web_vimode_test.go` for the web write shape.
- **Seam 2, CLI.** The Sort verb (read form and set form, `workspaces list --json` carrying the value, non-zero exit with the reason on an unknown value or workspace) and its parity-test entry; `workspaces remove` still works. Prior art: `server_parity_test.go`, the existing workspace verbs.
- **Seam 3, web unit (vitest, jsdom).** The highest seam for most items: `buildSidebar` ordering for all three sort values (all lists, Alt numbers, activity from sessions), live reorder with focus staying on the same row (and the fallback when the row leaves, and an open row menu following its row or closing), active-workspace resolution when a route's item is not yet in the snapshot, the Sort submenu disabled until the workspace resolves, the Fix agent dialog's refusal path, the row's chevron-only expansion and no auto-expand, Design row href, Enter versus click on a chat, chat row menu items, the Alt-held hook (text-field gate, no state above the sidebar), the Menu component (item kinds, submenu side versus phone swap with Back, flip near the bottom, keyboard, focus return), the gear menu and Sort, the limits tray (tone thresholds, popover content, spending mode), swipe thresholds and the standalone gate, resync on `visibilitychange` and `pageshow`, closing menus when the sidebar closes, create requests carrying the active workspace. Prior art: `GearMenu.test.tsx`, `JumpMenu.test.tsx`, `useWide.test.tsx`, `sidebarModel` tests, `connection` tests.
- **Seam 4, browser (Playwright).** Per gx-playwright-e2es: no render-only flows, extend before adding, keep `maxFlows` and CATALOG.md in step. All behaviour below goes into existing flows on the same scenario; no new flow is planned. Ported flows are listed in Verification.
- Look checks use `gx-ui-check` against the fixture, driven from the Mock states list.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`). "The `workspaces` scenario" is the existing scenario (three workspaces, a live and a held Design, an implementation epic with agents, a chat, Fix agents, an Other section, one card per section); "`limits`" and "`spend`" are the existing seeded scenarios; flow names are those of `webui/e2e/CATALOG.md`. Look checks refer to the Mock states list at the end by state name.

1. Story 1: flow "Workspaces" (extended): clicking an implementation epic's row opens its page and leaves it collapsed; clicking the chevron expands it.
2. Story 2: flow "Workspaces" (extended): the chevron toggles the epic's agents in both directions; vitest on the row.
3. Story 3: flow "Workspaces" (extended): with the sidebar focused, Right/l expands and Left/h collapses the focused epic row (the existing keyboard step).
4. Story 4: flow "Workspaces" (extended): open an epic's page while its row is collapsed (stays collapsed); expand it, navigate to the page and back (stays as left).
5. Story 5: flow "Workspaces" (extended): clicking the Design row lands on the Design page with the composer, not the epic page. Mock states `sidebar-desktop` for the row look.
6. Story 6: flow "50. Design epic page" (existing) still passes: the Epics page Design card links to the epic page.
7. Story 7: flow "Workspaces" (extended): focus the chat row with the keyboard, press Enter, the chat opens and the prompt input has focus.
8. Story 8: flow "Workspaces" (extended): clicking the chat row opens it with the prompt not focused; at 390px tapping does not open the keyboard (prompt not the active element).
9. Story 9: flow "Workspaces" (extended): the chat row's "..." menu shows Stop while live; Stop ends it, the row reads "Exited" and Stop is gone. Mock states `row-menu-top` for the menu look on the phone.
10. Story 10: flow "Workspaces" (extended): Remove asks for the confirmation, the row leaves the sidebar and `gx agents` no longer lists the session.
11. Story 11: flow "Workspaces" (extended): removing the open chat from its row menu navigates away from the chat's page.
12. Story 12: vitest: nested agent rows render no menu button.
13. Story 13: flow "Workspaces" (extended): holding Alt shows numbers in the menu-button slot; the status words stay "Needs you"/"Working". Vitest: slot content only.
14. Story 14: flow "Workspaces" (extended): same step; the row's status words are identical before, during and after Alt.
15. Story 15: flow "Workspaces" (extended): with a text field (jump input) focused, Alt keydown changes nothing; Alt+1 does not navigate. Vitest: the Alt-held hook ignores key events while a text field has focus.
16. Story 16: flow "Workspaces" (extended): on the `workspaces` scenario, trace Alt keydown and keyup with a request log and a mutation observer on the main content (outside the sidebar). First step: reproduce the wider refresh before the fix and record what triggers it (record it in the ticket); after the fix, no `/v1/*` request and no DOM change outside the sidebar's number slots while Alt is held. Vitest: a render counter on a page component under the shell does not increase on Alt.
17. Story 17: flow "Workspaces" (extended): gear menu Sort submenu offers the three options, the active workspace's marked; choosing another reorders the sidebar; the item's label names the active workspace (for example "Sort · research") and changes when the workspace is switched. Vitest: the submenu is disabled until the active workspace resolves; a refused write shows the error toast and the radio stays on the stored value. Mock states `gear-menu` (item look) and `move-submenu` (submenu swap on the phone).
18. Story 18: flow "Workspaces" (extended): the Sort verb from the CLI, with a workspace name, changes that workspace's order without a reload and switching workspace switches the order; `<verb> <workspace>` prints that workspace's value, `<verb> <workspace> <value>` sets it, prints the new value and exits 0, and `workspaces list --json` shows each workspace's sort. An unknown value or an unknown workspace exits non-zero with the reason and changes nothing. Servertest: a restart keeps each workspace's value, a new workspace starts at the default, rename re-keys and delete drops the value, an old state file without the sort map loads as default, and the route refuses an unknown value (the Reason lists the valid values) and an unknown workspace (`ReasonUnknown`) and stores nothing.
19. Story 19: vitest on `buildSidebar`: ordering of top-level rows (cards and chats together), On hold rows, nested agents and the Other section for all three values, including rows with no activity time; a Design with a busy session and no ticket activity sorts by the session's `status_changed_at` under By last activity; so does an On hold row whose session is newer than its card (activity comes from the session placements, not from the row's displayed agents); alphabetical is case-insensitive with ties in server order.
20. Story 20: flow "Workspaces" (extended): with By last activity, Alt+1 opens the first row of the sorted list; with the default, Alt+1 opens the first Needs-you row. Vitest on the numbers.
21. Story 21: vitest: every former menu (row "...", "+ New", gear, workspace switcher, page header "...", Epics card "...") renders through the Menu component; flows "129. Restart server keyboard", "130. Restart dialog confirms", "75. Read a session" (header "..." menu) and "73. Agents sidebar" still pass (ported).
22. Story 22: flow "Workspaces" (extended) at 1280px: Move to workspace opens a submenu to the side of the item. Mock states `sidebar-desktop` (docked sidebar look; the submenu itself is not in a mock).
23. Story 23: Mock states `row-menu-top`, `row-menu-bottom`, `new-menu`, `gear-menu` (phone menu). Flow "88. Phone, Agents page" (ported): the footer "+ New" > "New chat" opens as an anchored popover, not a sheet, and the New chat form it opens keeps its layout. Vitest: flip above near the bottom, 8px inset, right edge on the button's.
24. Story 24: Mock states `row-menu-top`; flow "Workspaces" (extended) at 390px: tapping outside the open menu closes it.
25. Story 25: Mock states `move-submenu`; flow "Workspaces" (extended) at 390px: Move to workspace swaps in place with a "Back" row at the top; Back returns to the row menu; picking a workspace moves the item and closes the menu.
26. Story 26: vitest: arrow keys, Enter, Escape and focus return on the Menu; flow "Workspaces" (extended): open a row menu and the gear menu with the keyboard and choose an item without the mouse. Mock states `page-background` and `sidebar-background` (focus ring) for the ring on menu items.
27. Story 27: flow "Workspaces" (extended): the Move submenu lists every workspace including the Default workspace with the current one marked; choosing another moves the row to that workspace's list.
28. Story 28: flow "Workspaces" (extended): Move from a sidebar row, an Epics card and a jump menu row each work (the existing three-surface step, now through the submenu).
29. Story 29: flow "Workspaces" (extended): no "Remove from workspace" in any menu and no confirmation dialog for it. Vitest: the menu items of a non-Default member.
30. Story 30: `gx server workspaces remove <item>` exits 0 and the item is under the Default workspace (servertest / CLI parity test unchanged).
31. Story 31: flow "Workspaces" (extended): with the "research" workspace active, "+ New" > "New design" creates a Design; `gx server workspaces list --json` shows it in "research". The same on `newdesign.e2e.ts` ("26. New Design form") path from the sidebar.
32. Story 32: flow "Workspaces" (extended) and "85. Create session" (ported): with "research" active, "+ New" > "New chat" creates a chat that shows in "research"'s sidebar list.
33. Story 33: flow "Workspaces" (extended): the creates from the Epics page and the Agents page New session form land in the active workspace, as do the two sidebar creates; vitest: the request body carries the active workspace name from all three entry points.
34. Story 34: servertest and CLI: a create with no workspace (CLI, TUI path) lands in the Default workspace; an unknown name reads as Default. Servertest (design create): a promote (`FromSession`) with a workspace name still puts the Design in its chat's workspace, and reopening an existing Design with a different name leaves its membership unchanged.
35. Story 35: Mock states `gear-menu` (sidebar look); flow "Workspaces" (extended): the gear menu lists AFK, Vi mode, Sort, Restart server and "More settings..." and nothing else; "129. Restart server keyboard" and "130. Restart dialog confirms" (ported) still reach Restart; "108. Change password" reaches Settings.
36. Story 36: flow "Workspaces" (extended): "More settings..." opens the Settings page.
37. Story 37: vitest and Mock states `gear-menu`: no build string in the menu; the sidebar header shows the version.
38. Story 38: Mock states `sidebar-desktop` (tray between health meter and gear). Port the limits flows in `menu.e2e.ts` (a statusline report reaches the open popover; amount rises while open and a closed popover stops fetching; the popover flips to limits; narrow popover fits and updates live) from the gear menu to the limits tray popover, on `limits` and `spend`.
39. Story 39: new steps in the ported `menu.e2e.ts` flow on the `limits` scenario (`--shape fresh` and a shape past 80%): bars show C and X, the fill and the tone class follow the usage. Vitest: tone thresholds at 79, 80 and 100%, a sliver at 0%.
40. Story 40: Mock states `limits-popover`; the ported `menu.e2e.ts` flows (limits shapes `fresh`, `stale`, `past-reset`, `one-window`); on `spend` the spend shows in the popover and not on the bars.
41. Story 41: Mock states `sidebar-desktop`, `sidebar-phone` (background, hover, current row).
42. Story 42: Mock states `fix-agent-rows` on the `workspaces` scenario (Fix agents under `paused-epic` and under one-offs).
43. Story 43: Mock states `page-background`, `sidebar-background`; flow "Workspaces" (extended): Tab onto a sidebar row, a button, an icon button, a menu item, a text field and an Epics card; each shows the ring (computed style), including after a keyboard-opened menu.
44. Story 44: Mock states `sidebar-background` (inset ring on a row); the row's ring is not clipped by the sidebar.
45. Story 45: flow "Workspaces" (extended): a mouse click on a button or row shows no ring; the next Tab shows it.
46. Story 46: flow "Workspaces" (extended) at 390px and 1280px: the search button opens the jump menu; its tooltip names Cmd/Ctrl+K; flow "22. Back link count, then the phone layout" (ported) still passes. Mock states `sidebar-desktop` (search-button).
47. Story 47: flow "Workspaces" (extended) at 744px and 390px: a swipe left (dispatched through CDP touch events) closes the open sidebar; a vertical scroll of the list does not. Vitest on the gesture hook.
48. Story 48: vitest: with `navigator.standalone` or `display-mode: standalone` true, a rightward swipe starting at the left edge opens the sidebar. Real home-screen behaviour is checked by the person on the iPhone (not automatable); the verify ticket says so.
49. Story 49: vitest: with neither standalone signal, the same swipe opens nothing; flow "Workspaces" (extended) at 390px in the default browser context: the edge swipe does not open it.
50. Story 50: vitest: a movement under 40px, or mostly vertical, neither opens nor closes; a tap on a row still navigates.
51. Story 51: flow "Workspaces" (extended): block `/v1/events` with `page.route`, change state from the CLI (rename a workspace), then dispatch `visibilitychange` (visible) and `pageshow`; the sidebar shows the new name with no reload and the stream reconnects. Vitest: both events restart the stream and trigger the resync signal even when the stream looks alive.
52. Story 52: flow "Workspaces" (extended) at 390px: open the gear menu and the limits popover, close the sidebar by the toggle and by swipe; both are gone. A form or dialog opened from a row menu item stays open with the sidebar open behind it, and the sidebar cannot be closed meanwhile: a backdrop tap, a swipe and Cmd/Ctrl+B close only the dialog, and the sidebar is still open afterwards. A submit that navigates (New chat) closes the dialog and then the sidebar. "More settings..." navigates and closes the sliding sidebar, at 390px and again at 744px (tablet, where the sidebar also slides over). Vitest on the same.
53. Story 53: flow "Workspaces" (extended) on the `workspaces` scenario: "Fix it with an agent" on a ticket card closes the dialog and the page is the agent's page; the failed-migration strip's Fix agent does the same and the agent shows in the Other section. Vitest: on a refusal the dialog stays open and shows the reason.
54. Story 54: servertest and flow "Workspaces" (extended): `gx server workspaces list --json` shows the Fix agent's epic unchanged, the agent shows under the epic row, and a Fix agent of a one-offs ticket shows in the Other section without changing the active workspace. Vitest (active-workspace resolution): the agent's route is applied before its placement is in the snapshot, the route stays pending (the stored workspace does not win), and once the placement arrives the epic's workspace becomes active.
55. Story 55: vitest: a status change (a chat flips to Needs you) reorders the rows, and roving keyboard focus stays on the same row (same element), not the same position. Flow "Workspaces" (extended): focus a row with the keyboard, change that row's state from the CLI, the row moves and keeps focus. Vitest: focus follows the key into On hold; when the focused row leaves (moved to another workspace, removed, workspace switch) focus lands on the row now at the old index, clamped to the list, and on the list itself when it is empty; an open row menu repositions with its row as it moves and closes when the row leaves the list.

New flows or scenarios to write (these become implementation tickets):

- No new Playwright flow. Extend the existing "Workspaces" flow (`workspaces.e2e.ts`) with the steps above, and port `menu.e2e.ts`'s limits flows to the tray popover. Keep CATALOG.md and `maxFlows` in step.
- Add a limits seed to the `workspaces` scenario (the scenario struct already supports `Limits`), so the sidebar shows the limits tray beside the workspaces rows; or a combined scenario, the implementer names it. Needed for the Mock states `sidebar-desktop` and `sidebar-phone`.
- A step or helper for the Alt-flicker trace (request log plus a mutation observer on the main content) in the "Workspaces" flow.
- A helper that dispatches a swipe with CDP touch events and one that blocks and releases the event stream with `page.route`.
- Gallery entries (if the gallery lists menus): the Menu component on each device type, the limits tray and its popover, a Fix agent row.

**Mock states**

The 02 states `sidebar-desktop`, `gear-menu` and `limits-popover` are checked at 1280px (sidebar docked), comparing only the mock's panel parts listed. The 02 `gear-menu` sets the item look; the 04 `gear-menu` sets the phone placement.

- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/02-prototype-sidebar-look.html`, `sidebar-desktop`, 1280px, the `workspaces` scenario with a limits seed (see new scenarios), sidebar docked: sidebar, search-button, row-current, row-hover, footer, limits-tray, gear-button.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/02-prototype-sidebar-look.html`, `sidebar-phone`, 390px, the same scenario with the sidebar open: sidebar.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/02-prototype-sidebar-look.html`, `gear-menu`, 1280px, the `workspaces` scenario with the gear menu open: gear-menu-panel.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/02-prototype-sidebar-look.html`, `limits-popover`, 1280px, the `limits` scenario (shape `fresh`) with the limits tray open: limits-panel.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/02-prototype-sidebar-look.html`, `fix-agent-rows`, 560px, the `workspaces` scenario with `paused-epic` expanded and one Fix agent needing you and one working: fix-row-needs-you, fix-row-working.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/03-prototype-focus-ring.html`, `page-background`, 420px, the `workspaces` scenario on the Epics page, keyboard-focusing each control: sidebar-row, button, icon-button, menu, menu-item, text-field, epics-card.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/03-prototype-focus-ring.html`, `sidebar-background`, 420px, the `workspaces` scenario with the sidebar open, keyboard-focusing each control inside it: sidebar-row, button, icon-button, menu, menu-item, text-field.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/04-prototype-phone-menu.html`, `row-menu-top`, 390px, the `workspaces` scenario with the sidebar open and a row near the top menu open: sidebar, row-button, new-button, gear-button, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/04-prototype-phone-menu.html`, `row-menu-bottom`, 390px, the same with a row near the bottom (flips above): sidebar, row-button, new-button, gear-button, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/04-prototype-phone-menu.html`, `new-menu`, 390px, the same with "+ New" open (flips above): sidebar, new-button, gear-button, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/04-prototype-phone-menu.html`, `gear-menu`, 390px, the same with the gear menu open (flips above): sidebar, new-button, gear-button, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-nav-fixes/mocks/04-prototype-phone-menu.html`, `move-submenu`, 390px, the same with a row's Move to workspace submenu open (Back row on top): sidebar, row-button, new-button, gear-button, menu.

## Out of Scope

- Full keyboard control of the web UI: a later effort. This spec only lays groundwork (the global focus ring, the shared Menu component).
- Edge swipe to open the sidebar in a Safari tab: Safari's back gesture owns the left edge; the edge swipe works only in the home-screen app.
- The Epics page Design card link: only the sidebar Design row changes target.
- Removing the `gx server workspaces remove` CLI verb: it stays for scripts.
- Sorting anything outside the sidebar (Epics page, jump menu, Server queue).
- A new workspace or hold concept, a new pause kind, or any ticket or epic schema change.
- A tablet or phone menu variant other than the approved anchored popover; the menu bottom sheet is removed (forms keep their layout).
- A server-wide sort setting or a new last-activity wire field: the Sort is per workspace and reads existing times.

## Further Notes

- The approved prototypes are stored in the map's `mocks/` directory: sidebar look (variant B background, two-bar limits tray, wrench), focus ring (variant B, gap + blue ring) and phone menu (variant B, roomy popover). Where a mock and this spec differ, the mock wins on look and this spec wins on behaviour.
- This drops better-nav story 8's auto-expand ("the open page's epic starting expanded") and story 29's "Remove from workspace" on the web, and replaces story 12's gear menu contents. It also reverses better-nav story 55 ("no search button"): story 46 adds a search button. The better-nav spec is not edited.
- Rows reorder at once, even under a moving pointer or finger, so a row can slide under it just before a click. Only an open row menu is protected; mis-taps are accepted for now.
- Story 19 changes the GLOSSARY's Other section: its contents stay the same in every workspace, its order follows the active workspace's Sort. The impl epic updates GLOSSARY.md (see Vocabulary).
- The per-workspace Sort overrides the charting decision that made it server-wide like Vi mode. It is now stored per workspace, with the workspace store.
- The Alt flicker and the Design row target are bugs found while charting; the fix for the flicker is the cause found by the trace in story 16, not a row-level patch.
- Expect the swipe behaviour to need a person check on a real iPhone or iPad: the fixture cannot run the home-screen app.
