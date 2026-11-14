## Problem Statement

New design opens a dialog over whatever page the person is on. The dialog holds one draft at a time, with a switcher to reach the others. A person who starts a second idea cannot see the first one except by opening the switcher. Drafts belong to no workspace, so they never show next to the Designs, epics and chats the person is working on. The dialog is also a poor place to write a long prompt: it is a layer over the page, it has its own close and Tab handling, and on a phone it is a sheet.

## Solution

New design creates a Design draft at once and opens its Design draft page. No dialog asks for anything first.

- Each Design draft is a row in the sidebar of the workspace it was created in, with its own icon (a dashed page outline in the Design purple) and the status "Draft". It sits in the mixed list with the Design, epic and chat rows and follows the sidebar's Sort.
- The Design draft page is the old dialog's layout as a page: a header with the label, a Rename pencil (it edits the slug) and the saved state; a project and slug bar; the Prompt; the image strip; and a footer with Attach, Remove, the Start design reason and Start design.
- Start design starts the design session as today and removes the draft. The new Design row appears in the draft's workspace. Remove deletes the draft.
- A draft with nothing in it (no prompt, no images, no slug) leaves nothing behind: it is removed when the person leaves its page inside the app, and New design from that page reuses it.
- The sidebar, the jump palette, the command palette, the chord `d`, the Epics page button and the phone sidebar all use the same action. The dialog and its draft switcher go away.
- Every browser on the same gx server sees the rows at once: the connection snapshot carries them and a `design-drafts-changed` event refreshes them.

## User Stories

1. As a person, I want New design to create a Design draft and open its page at once, so that I can start writing without answering a dialog first.
2. As a person, I want the Design draft to appear as a row in the active workspace's sidebar, so that I can see it next to my Designs, epics and chats.
3. As a person, I want the draft row to have its own icon (a dashed page outline in the Design purple), so that I tell it from a Design, an epic and a chat at a glance.
4. As a person, I want the draft row to show the status "Draft" in muted text, so that I never mistake it for work that needs me or is running.
5. As a person, I want the draft row labelled with its slug, else the first five words of its prompt cut at 40 characters, else "Untitled design", so that every draft has a readable label.
6. As a person, I want the draft row to sort by the active Sort like any other row (by last edit for Activity, group "Draft" under By status, by its project under By project), so that I do not have to learn a special place for it. Under Activity another tab's order can lag until the next event or snapshot refetch, because ordinary prompt saves publish no event. This is accepted.
7. As a person, I want the draft row to take one of the numbers 1 to 9 like any own row, so that Alt+n opens it.
8. As a person, I want a row menu with Rename, Move to workspace and Remove, so that I can manage a draft without opening it. Rename opens the chat Rename popup holding the slug input.
9. As a person, I want the draft page to have a Prompt field with vim mode, an image strip, a project picker and an epic slug field, so that I can write the whole idea in one place.
10. As a person, I want the project and slug to be fields on the page and the slug allowed to stay empty until Start design, so that I am not forced to name the epic before I know what it is.
11. As a person, I want a new draft to start on the project of the open Design or chat page, else the Epics page's single-project filter, else the last project used, so that the project is usually right. The project of a non-empty draft's own page comes first, like an open Design or chat page.
12. As a person, I want my text to save while I type and to see "Saving..." and "Saved" beside the label, so that I know my words are safe.
13. As a person, I want a Rename pencil in the page header that focuses the slug field, so that I can give the draft a slug, which wins over the prompt in the label. An empty slug clears it and the label falls back.
14. As a person, I want to paste or attach images to the draft, up to the composer's limits, so that the design agent receives them at Start.
15. As a person, I want Start design to be off with a reason ("Write a prompt first", then "Name the epic to start.", then the image and upload reasons), so that I know what is missing.
16. As a person, I want Start design to create the Design as it does today, so that the design session works exactly as before.
17. As a person, I want the new Design to start in the same workspace as the draft, so that it does not jump to another workspace.
18. As a person, I want the draft row to leave the sidebar and the Design row to appear after Start design, so that the sidebar shows one item for the work.
19. As a person whose start fails, I want the Design page with the retry, as today, so that a failed start loses nothing.
20. As a person, I want Remove on the page footer, in the row menu, in the bulk bar and on the chord `x`, so that I can delete a draft wherever I am.
21. As a person, I want Remove on a non-empty draft to ask first (the footer's Keep and Remove pair on the page, the Keep/Remove modal "Remove <label>? Its prompt and images are deleted." in the row menu), so that a slip does not lose work.
22. As a person, I want Remove on an empty draft to act at once from the page button, the row menu and `x`, so that clearing a blank draft costs no click.
23. As a person, I want the action called Remove everywhere and never Discard, so that the word means one thing.
24. As a person, I want navigating away inside the app from an empty draft's page to delete it and its row, so that blank rows do not pile up. A reload or a tab close deletes nothing; the lease and the sweep cover a closed tab.
25. As a person, I want New design pressed on an empty draft's page to reuse that draft and focus its Prompt, so that I do not get a second blank row. A new draft also opens with the Prompt focused.
26. As a person, I want New design pressed from any other page to create a new draft, so that each press gives me somewhere to write.
27. As a person, I want a draft with a slug to count as non-empty, so that giving it a slug keeps it. Clearing the slug can make the draft empty again.
28. As a person, I want a reload of the draft page to keep the same draft and its text, so that a refresh loses nothing. The reload sends no remove-if-empty.
29. As a person, I want a draft that a page has open to survive the server's sweep, so that the server never deletes a draft I am looking at.
30. As a person, I want the server to sweep empty drafts at start and when a draft is created, so that abandoned blank drafts (a closed tab) go away.
31. As a person whose page was swept, I want to land on the Epics page with a short note that the draft was removed, so that I know what happened.
32. As a person, I want to move a draft to another workspace with Move to workspace, so that it sits with the rest of its work.
33. As a person, I want a draft to have no On hold and no Archive, so that Start design and Remove are its only exits.
34. As a person, I want drafts that exist on the server today to appear in the Default workspace, so that old drafts are not lost.
35. As a person deleting a workspace, I want its drafts to move to the Default workspace like its other members, so that nothing disappears. The delete dialog names a draft member by its label.
36. As a person, I want the chords `r` (Rename, which focuses the slug field) and `x` (Remove) to work on a draft page, so that the keyboard reaches the same actions as on a chat.
37. As a person, I want to select drafts in the sidebar's bulk mode and Move or Remove them, so that I can clean up fast.
38. As a person, I want Hold, Archive and Stop in the bulk bar to skip drafts, so that a mixed selection does not fail.
39. As a person, I want the bulk Remove to always ask, even for empty drafts, because a selection is deliberate. A mixed selection of chats and drafts gets one confirmation listing both, so that nothing goes without my yes. A draft there reads "Remove <label>? Its prompt and images are deleted."
40. As a person, I want Cmd/Ctrl+K to list Design drafts in their own "Design drafts" section (the active workspace's by default, other workspaces' only while searching under "Other workspaces"), so that I can jump to a draft.
41. As a person, I want a draft's jump row to show the sidebar's label, status, icon and row menu and to open its page, so that it behaves like the sidebar row.
42. As a person, I want the jump search to match a draft by label (which covers the slug) and project, so that I find it by whatever I remember.
43. As a person, I want "New design" in the command palette, the chord `d`, the sidebar footer and the Epics page to run the same action as the sidebar button, so that every entry point behaves alike.
44. As a person, I want the command palette to have no Remove or Start design entry for drafts, so that destructive actions stay on the page and the row menu.
45. As a person on a phone, I want the same page in the same stack, and the sidebar to open full width with the draft rows, so that I can write and reach drafts on a phone.
46. As a person, I want opening a draft from another workspace (jump result or link) to switch the sidebar to that workspace, so that the row is in view.
47. As a person with two browsers, I want a draft created, given a new slug or project, moved, started or removed in one to show in the other's sidebar without a reload, so that the sidebar is the same everywhere.
48. As a person with a draft page open in a second tab, I want a redirect to the Design page after Start and to the Epics page, with a note, after Remove, so that the tab never shows a dead draft. The open page learns this from the `design-drafts-changed` event and re-reads at once; the lease ping is the fallback.
49. As a person who bookmarked a draft URL, I want it to redirect to the Design page after Start for at least 7 days, and to the Epics page with the "removed" note when the draft is unknown, so that old links still land somewhere useful.
50. As a person, I want the "changed on another device" banner (Load theirs, Keep mine) to stay on the page, so that two devices editing one draft lose no text. A slug, project or workspace change made outside the open page does not trip the banner; the page picks up the new slug and project from the refetch.
51. As a person, I want text that could not be saved kept in the browser per draft and saved on my next visit, so that a server hiccup loses nothing.
52. As a person with a draft in the old single-draft browser storage, I want it moved into a server draft before my next New design creates a draft, so that the old text is not lost.
53. As a person, I want the page title to show the draft's label, so that the browser tab names the draft.
54. As a person, I want Rename in the row menu to open the chat Rename popup with the slug input, using the same slug rules as the page's slug field, so that I can set or clear the slug without opening the draft.
55. As a person, I want every way a draft is deleted (Remove, sweep, remove-if-empty, Start) to drop its workspace membership, and the workspace prune to drop members whose draft is gone, so that no dead entry stays in a workspace.

## Implementation Decisions

### Draft record and server rules

- The server's draft store keeps its record shape minus the name: one record per draft with project, epic slug, prompt, image ids, a revision and an edited-at time. `Name` is removed from the wire types (`ops.DesignDraft`, `DesignDraftItem`, the create and save requests), the store's create path, the routes and the `gx designs drafts` verbs. Rename edits the slug.
- A one-time step runs at server start, before the first sweep, for each stored draft that still has a name: when the slug is empty and the slugified name passes slug validation, the name becomes the slug; otherwise the name is added as the first line of the prompt. Then the name field is dropped. A name-only draft is therefore never swept as empty by the upgrade. A draft is still not a map epic: it is not on the Epics page, in `gx tickets` or in `gx server epics`.
- A draft is now created empty on purpose. The store's create and save paths no longer delete an empty draft. Deleting an empty draft is a separate, explicit rule below.
- "Empty" means no prompt, no images and no slug. The project never counts, because every new draft has one. Clearing the slug can make a draft empty again.
- A new draft starts on the project chosen by today's rule order: the open Design, chat or non-empty draft page's project (New design pressed on a non-empty draft's page starts the new draft on that draft's project), then an Epics page filter on exactly one project, then the last project used.
- The page leaving rule: only in-app navigation away from an empty draft's page sends a "remove if empty" request. Reload and tab close send nothing; the lease and the sweep cover a closed tab. The server deletes the draft only while it is still empty, so text typed on another device in the meantime keeps it. The row disappears through the sync event.
- Reuse: New design pressed on the page of an empty draft creates nothing and focuses that page's Prompt. From any other page, New design creates a draft, and the new draft's page also focuses the Prompt.
- Rename edits the slug. The page's Rename pencil and the chord `r` focus the slug field. The row menu's Rename opens the chat Rename popup holding the slug input, with the same slug validation as the page's slug field. The popup takes a session or a draft target. For a draft it is titled "Rename draft", labels its field "Slug", starts with the current slug, allows an empty submit (which clears the slug and the label falls back) and shows no auto-name line. There is no separate name field and no `name` change event.
- The sweep: the server deletes empty drafts at server start and each time a draft is created. It skips a draft with a live page lease. The open page pings a lease route about every 30 seconds; a lease lasts about 2 minutes after the last ping. At server start no lease is live. A page whose draft was swept takes the "removed" redirect. An open page learns its draft is gone from the `design-drafts-changed` event and re-reads at once; the lease ping is the fallback. The implementer picks the ping interval, the TTL and the lease route.
- Remove deletes the draft, its image folder and its workspace membership. Remove of a non-empty draft asks in the UI (below); the server route does not ask.
- Every draft delete path (Remove, sweep, remove-if-empty, Start) drops the workspace membership.
- Start design creates the map epic as today and then deletes the draft. The new Design takes the workspace of the draft: the server reads the draft's own workspace before it deletes the draft, and does not take the workspace from the request. Images move into the map's `images/` folder as today. A start failure keeps today's behaviour: the map exists, the draft is deleted, and the Design page offers the retry.
- Start writes a small persisted record from the draft id to the Design address. The old draft URL redirects to the Design page through it. The server prunes a record after 7 days. A draft id with no draft and no record is "removed".
- The revisioned text save carries the prompt and the image ids only. The slug and the project have their own write, which never touches the revision. The page's slug field, its project picker, the Rename popup and the row menu all use that write. So a stale autosave never writes an old slug or project back after an outside change.
- The revision-conflict rules stay: a text save carries the revision; a stale revision gets the "changed on another device" refusal. The "deleted or started on another device" gone banner and its Keep as new draft button are dropped; a save against a gone draft instead takes the redirect.
- Slug, project and workspace changes made outside the open page (the row menu, another device) do not bump the text revision and do not trip the "changed on another device" banner. The page picks up the new slug and project from the `design-drafts-changed` refetch.
- The last project used is still recorded by the server; a project pick on the page, and the project of the last Start, update it.

### Workspace membership

- A draft is a workspace member through a membership key. The implementer picks the key format. It must not clash with an epic address or a session id, and the store's membership rules apply to it: the join check that today accepts only epic and chat keys gets a case for draft keys.
- A draft created today lands in the active workspace. Drafts that exist before the upgrade have no membership; the store treats an unknown key as the Default workspace, so no migration step is written.
- Move to workspace, bulk Move and workspace deletion work on drafts as on other members (a deleted workspace's drafts move to the Default workspace).
- Every draft delete path (Remove, sweep, remove-if-empty, Start) drops the membership. The workspace prune (`pruneEpicMembers`) also drops draft keys whose draft folder is gone.
- The Manage workspaces delete dialog names a draft member by its label.
- A draft cannot be held or archived: the Hold, Archive and Stop actions and their routes do not apply to draft keys.

### Snapshot and events

- The connection snapshot gains a draft field next to the workspaces and placements fields. Its rows carry what the sidebar and the jump palette need: the id, the computed label, the project, the slug, the edited-at time and the workspace. They carry no prompt text and no image list. The implementer picks the wire name and the fields.
- Label rule: the epic slug, else the prompt's first 5 words cut at 40 characters, else "Untitled design". The server computes the label and ships it in the snapshot rows and in the save result. The sidebar, the jump palette and the page title read it, so the rule lives in one place.
- The draft store publishes a new `design-drafts-changed` event on create, slug change, project change, move to another workspace, and on every delete path (Remove, sweep, remove-if-empty and Start). A prompt save publishes it only when the row's label changes (no slug, and the first words differ). Other autosaves publish nothing, so under the Activity sort another tab's order can lag until the next event or snapshot refetch. This is accepted.
- The client refetches the snapshot on the event, so every tab and device updates. After the stream drops and returns, the snapshot refetch brings the rows back; there is no separate catch-up and no polling.
- The generated wire types and the route-table and CLI parity rules (the repo's "Adding a server route" checklist, including the web allowlist) apply to every new route and the new event. The CLI gets only what those parity tests force; the existing `gx designs drafts` verbs keep working. The implementer picks route and verb names.

### Sidebar and rows

- The sidebar model gets a draft row kind. It has the draft icon, the label, the status "Draft" (muted), and the draft's edited-at time as its activity time. It is never "Needs you" or "Working".
- It joins the mixed list and follows the Sort: Needs you (a draft never needs you), Activity (by edited-at), Name (by label), By project (the draft's project), By status (group "Draft").
- It takes a number from 1 to 9 like any own row. Alt+n opens its page.
- Row menu: Rename (opens the chat Rename popup with the slug input), Move to workspace, Remove. Remove on a non-empty draft opens the same Keep/Remove modal with the text "Remove <label>? Its prompt and images are deleted."; on an empty draft it acts at once. The page button and `x` follow the same rule.
- Bulk mode: select, Move and Remove include drafts. Bulk Remove always asks, even for empty drafts, because a selection is deliberate. A mixed selection of chats and drafts gets one confirmation listing both and calls both removes. A draft there reads "Remove <label>? Its prompt and images are deleted." Hold, Archive and Stop skip drafts.
- Opening a draft that sits in another workspace (jump palette result, direct link) switches the sidebar to that workspace.
- The row icon is a new gallery entry beside the Design, epic and chat icons.

### The Design draft page

- A new route renders the page for one draft id. The implementer picks the path. The page reuses the old form's internals (prompt field with vim mode, image strip, project select, slug field, save loop) and drops the dialog shell, its own Tab trap and the draft switcher.
- Layout, from the approved mock: a header (draft icon, label, Rename pencil that focuses the slug field, saved state); a bar with the project picker and the slug field; the Prompt with its vim mode line; the image strip; a footer with Attach at the left, then Remove, the Start design reason and Start design.
- The field is called "Prompt". The blocker texts: "Write a prompt first" first, then "Name the epic to start." Image and upload reasons are unchanged. The reason sits beside Start on desktop and above the footer on a phone, as today.
- Remove in the footer on a non-empty draft swaps in the pair "Remove this draft and its images?" with Keep and Remove. On an empty draft it removes at once.
- Saving runs 800 ms after the last keystroke, and at once when the page is left. The failed-save "Not saved" retry rules (every 5 seconds, and on the next keystroke) stay.
- The unsaved-text browser fallback stays and is keyed by draft id. The one-time move of the old single-draft browser key runs before each New design creates a draft; it puts the old text into a draft of its own.
- The page pings the lease while open, and when it learns its draft is gone it redirects: to the Design page when a Start record exists, else to the Epics page with the short note "This draft was removed."
- The page title is the draft's label.
- Chords: `r` focuses the slug field and `x` removes, with the same rules as the page button.

### Entry points

- One shared "new design" action creates (or reuses) a draft and navigates to its page. The sidebar footer's New design, the command palette's New design, the chord `d` and the Epics page's New design button all call it. The New Design dialog (`DesignDialog` in the new-item host) and the draft switcher are deleted. On a phone the action does the same.
- The command palette has New design only. It has no Remove and no Start design entry for drafts.

### Jump palette

- The jump palette gets a "Design drafts" section, separate from "Designs". With an empty filter it lists the active workspace's drafts. While searching it also lists drafts of other workspaces, in "Other workspaces", with the workspace name as the subtitle.
- A row shows the draft icon, the label, the status "Draft" and the sidebar's row menu, and opens the draft's page. The search matches the label (which covers the slug) and the project.

### Wording

- The glossary terms are Design draft and Design draft page. The action is Remove everywhere: row menu, chord, bulk bar and page button. "Discard", "New Design dialog" and "New Design draft" go from the UI text and from the tests' names.

## Testing Decisions

- A good test drives the product from outside: a server route, a rendered page or sidebar with injected fetchers, or a browser flow. It asserts what a person sees or what the server stores, not component structure or private helpers.
- Seams, highest first:
  1. Go tests through the server routes (prior art: the drafts store and route tests, the workspaces route tests): empty create and save keep the draft, remove-if-empty refuses a draft that gained text, the sweep with and without a live lease, a lease that expires, the slug counting as content and clearing it making the draft empty again, a slug change not bumping the text revision, Start reading the draft's workspace before deleting it, the Start record and its 7-day pruning, the redirect result for started, removed and unknown ids, membership join for draft keys, Default workspace fallback for an unknown key, workspace delete, membership dropped on every delete path, the prune dropping draft keys whose folder is gone, Hold/Archive/Stop refusing draft keys, the snapshot field and `design-drafts-changed` on exactly the listed changes (and not on other autosaves).
  2. Web unit tests (vitest; prior art: the old form's tests and the sidebar model tests): the label rule table, `buildSidebar` for the draft kind under every Sort, number shortcuts, the page's states (empty, filled, Remove pair, blocker texts, conflict banner, redirects), the jump palette sections and search, the one shared new-design action (create versus reuse), the localStorage fallback and the old-key move.
  3. Playwright for real round trips only, by extending existing flows: 26 (the draft journey, rewritten from the dialog to the page), 41 (phone), 22 (jump and phone sidebar), and the Workspaces, Chords, Bulk actions and Sidebar rows flows. No new flow is added (`playwrightguard` flow budget).
- Existing tests that assert the dialog, the draft switcher, the "deleted or started on another device" banner, Discard wording, or a store that deletes empty drafts on create and save change with this epic.

## Verification

Each entry runs against a temporary gx server (`go run ./testutil/gxfixture <scenario>`, see the gx-ui-check skill). Flows are cited by number or name from `webui/e2e/CATALOG.md`. Scenario `workspaces` is used unless stated; it has the Design, epic and chat rows, the Default and named workspaces, and projects. Its seed gets one non-empty draft labelled by its slug with no workspace membership; it serves the "other workspace" row look and the unmembered-draft check. Scenario `bulk-actions` seeds one non-empty and one empty draft. "Mock states" refers to the lines at the end of this section.

1. Story 1: flow 26 (rewritten): press New design on the Epics page, the URL is the draft page and the Prompt has focus (also for a reused empty draft); no dialog is in the DOM. CLI check: `gx designs drafts list` lists the new draft and exits 0; `gx server epics` does not list it. Look check: Mock states `desktop-empty`.
2. Story 2: flow Workspaces: after New design the active workspace's sidebar has a draft row and another workspace's does not. Look check: Mock states `desktop-empty`.
3. Story 3: look check: Mock states `desktop-empty` and `phone-sidebar` (the sidebar-draft-row icon beside the design, epic and chat rows). Vitest: the gallery entry renders.
4. Story 4: flow Sidebar rows: the draft row's status reads "Draft" in the muted style, never "Needs you" or "Working". Look check: Mock states `desktop-empty`.
5. Story 5: Go table test of the label rule the server computes (slug, first 5 words cut at 40 characters, "Untitled design"); vitest that the sidebar, jump palette and page title show the shipped label. Flow 26: type a prompt, the row and the page title show its first words; set a slug, the row shows the slug.
6. Story 6: vitest `buildSidebar` for every Sort. Flow Workspaces: under By status the row is in group "Draft"; under By project it sits in its project's group. The Activity lag in another tab is accepted and has no check.
7. Story 7: flow Sidebar rows: holding Alt numbers the draft row and Alt+n opens its page.
8. Story 8: flow Sidebar rows: the row menu shows Rename, Move to workspace and Remove, in that order; Rename opens the chat Rename popup with the slug input.
9. Story 9: flow 26: the page has the header, project and slug bar, Prompt with the vim mode line, the image strip and the footer. Look check: Mock states `desktop-filled`.
10. Story 10: flow 26: Start design stays off with the slug empty and shows "Name the epic to start." once there is a prompt. Look check: Mock states `desktop-empty`.
11. Story 11: Go test for the rule order, including a non-empty draft's page first. Flow 26: with the Epics page filtered to one project, the new draft's project is that project; New design pressed on a non-empty draft's page makes a draft on that draft's project.
12. Story 12: flow 26: type, see "Saving..." then "Saved", reload, see the text. Look check: Mock states `desktop-filled`.
13. Story 13: flow 26: the Rename pencil focuses the slug field; a new slug updates the row and the title; clearing it brings the prompt label back. Look check: Mock states `desktop-filled` (rename part).
14. Story 14: flow 26 (the image journey kept): a pasted PNG and an Add image file show thumbnails and "2 of 10 images"; a held and a failed upload; the thumbnail click opens the viewer. Look check: Mock states `desktop-filled` (images part).
15. Story 15: flow 26: no prompt shows "Write a prompt first", a prompt with no slug shows "Name the epic to start.", an upload running and a failed image show their reasons and keep Start off. Flow 41 at 375 px: the one-line hint sits above the footer and Start keeps its natural width. Look check: Mock states `desktop-empty` and `phone-page`.
16. Story 16: flow 26: a good Start opens the Design page; `gx tickets show demo:<slug>` ends `## Initial prompt` with the `Images:` line and absolute paths that exist; the draft folder is gone from the state dir; the draft row has left the sidebar.
17. Story 17: flow Workspaces: create a draft in a named workspace, Start, and the Design row is in that workspace. Go test: Start reads the draft's workspace, not the request's.
18. Story 18: flow 26 and flow Workspaces: after Start the draft row is gone and one Design row is present.
19. Story 19: flow 26: the existing "already has an epic" refusal keeps the draft and its images; a forced map-write failure lands on the Design page with the retry (Go test through the route for the second case).
20. Story 20: flow 26 (footer Remove), flow Sidebar rows (row menu), flow Chords (`x`), flow Bulk actions (bulk Remove): each removes a draft. Look check: Mock states `desktop-remove`.
21. Story 21: flow 26: Remove on a draft with a prompt shows "Remove this draft and its images?" with Keep and Remove; Keep leaves the draft; Remove deletes it and its image folder. Flow Sidebar rows: the row menu Remove on a non-empty draft shows the Keep/Remove modal "Remove <label>? Its prompt and images are deleted." Look check: Mock states `desktop-remove`.
22. Story 22: flow 26 (page button), flow Sidebar rows (row menu) and flow Chords (`x`): Remove on an empty draft acts with no confirmation.
23. Story 23: shell check: the command `rg -n -i "discard" webui/src/design webui/src/shell -g '!*.test.*'` returns nothing. The flows read "Remove" in the row menu, bulk bar, footer and chord box.
24. Story 24: flow 26: create a draft, navigate to the Epics page inside the app, the row is gone and `gx designs drafts list` is empty. Reload the empty draft's page instead: the draft stays. Go test: remove-if-empty on a draft that gained text keeps it.
25. Story 25: flow 26: on an empty draft's page press New design; the row count stays one and the Prompt has focus.
26. Story 26: flow 26: from the Epics page with an empty draft already left behind (second context), New design creates a new draft; vitest for the create-versus-reuse rule.
27. Story 27: flow 26: set a slug on an otherwise empty draft, leave the page, the draft stays; clear the slug and leave, the draft is removed. Go test: a slug makes the draft non-empty for remove-if-empty and the sweep, and clearing it makes it empty again.
28. Story 28: flow 26: reload the draft page, the same draft and text return and no new row appears.
29. Story 29: Go test: the sweep skips a draft with a live lease and removes it after the lease expires. Flow 26: with the page open, a second New design from another context (which runs the sweep) leaves the open empty draft in place.
30. Story 30: Go test: the sweep at server start (a restart case) and on create removes empty drafts.
31. Story 31: Go test: a lease-expired draft is swept and its id then reads "removed". Vitest: a page given a "removed" read redirects to the Epics page with the note "This draft was removed."
32. Story 32: flow Workspaces: Move to workspace on a draft moves the row to the other workspace's list.
33. Story 33: vitest and flow Sidebar rows: the draft row menu and the page have no Hold or Archive. Go test: Hold, Archive and Stop refuse a draft key.
34. Story 34: Go test: a draft record with no membership resolves to the Default workspace. Flow Workspaces: a draft seeded without membership shows in Default. Go test of the one-time name step: a name-only draft with a valid slugified name gets that slug, an invalid one moves into the prompt's first line, and the first sweep then keeps both.
35. Story 35: flow Workspaces: delete a workspace holding a draft; the dialog names the draft by its label and the draft shows in Default.
36. Story 36: flow Chords: on a draft page `r` focuses the slug field and `x` removes (with the Remove pair for a non-empty draft).
37. Story 37: flow Bulk actions: select a draft with a Design, Move moves both, Remove removes the draft.
38. Story 38: flow Bulk actions: with a draft and a Design selected, Hold, Archive and Stop act on the Design and list the draft as skipped.
39. Story 39: flow Bulk actions: bulk Remove of the empty draft still shows the confirmation; a mixed selection of a chat and a draft shows one confirmation listing both, and Remove deletes both.
40. Story 40: flow 22 (jump menu step): Cmd/Ctrl+K with an empty filter shows a "Design drafts" section with the active workspace's drafts; a draft in another workspace is absent until a search, then shows in "Other workspaces" with the workspace name as subtitle.
41. Story 41: flow 22: the draft jump row has the icon, label and status "Draft"; picking it opens the page; its row menu has Rename, Move to workspace and Remove.
42. Story 42: flow 22: search by the label (a slug-labelled draft) and by the project name each finds the draft.
43. Story 43: flow 26 (Epics page button), flow Chords (`d`), flow 22 (command palette: the New design row runs the action and no dialog appears; the palette has no Remove or Start design entry for drafts), flow Workspaces (sidebar footer button). Each creates one draft and opens its page.
44. Story 44: flow 22: the command palette listing has "New design" and no draft Remove or Start design row.
45. Story 45: flow 41 (rewritten, 375 px): the page stack matches the desktop order, the paperclip sits left of Remove and Start design at the right edge at natural width; flow 22's phone step: the sidebar opens full width and lists the draft row. Look check: Mock states `phone-page` and `phone-sidebar`.
46. Story 46: flow Workspaces touch: at 390 px opening a draft from another workspace (jump result) switches the active workspace. Flow Workspaces: a direct link to a draft in another workspace does the same.
47. Story 47: flow Workspaces and flow 26: with a second browser context, create, change the slug, change the project, move, Start and Remove in the first; the second's sidebar follows without a reload. Go test: the event publishes on exactly those changes (including a sweep and a remove-if-empty, once each) and an ordinary prompt autosave publishes nothing.
48. Story 48: flow 26: the draft page open in the second context redirects to the Design page after Start in the first, and to the Epics page with the note after Remove, at once on the event.
49. Story 49: flow 26: after Start, open the old draft URL, it lands on the Design page; open an unknown draft id, it lands on the Epics page with the note. Go test: the record is pruned after 7 days (fake clock).
50. Story 50: flow 26: two contexts edit one draft; the conflict banner with Load theirs and Keep mine shows and both work; no "gone" banner or Keep as new draft button exists. Change the slug from the row menu in the second context: the first page shows the new slug and no banner. Go test: a slug, project or workspace change leaves the text revision unchanged, and a text save after an outside slug change keeps the new slug. Flow 26: after the second context's Rename, type in the first page and the new slug stays.
51. Story 51: vitest: a failing save shows "Not saved", retries and keeps the entry keyed by draft id; the next visit saves it with its revision.
52. Story 52: vitest: the old single-draft key with text becomes a draft before New design creates one, and the key is removed; a key with no text is removed.
53. Story 53: flow 26: the page title equals the draft's label and updates after a slug change (page title helper from the page-titles flow).
54. Story 54: flow Sidebar rows: row menu Rename opens the chat Rename popup with the slug input; an invalid slug is refused with the page field's message; an empty value clears the slug and the row label falls back. Vitest: the popup and the page field share one validation. The popup for a draft is titled "Rename draft", labels its field "Slug" and accepts an empty submit.
55. Story 55: Go tests: each delete path (Remove, sweep, remove-if-empty, Start) leaves no membership for the draft key; `pruneEpicMembers` drops a draft key whose folder is gone.

New flows and scenarios to write: no new Playwright flow and no new scenario. Every journey extends flow 26 (`newdesign.e2e.ts`), flow 41 (`phone.e2e.ts`), flow 22 (`jump.e2e.ts`), or the Workspaces, Workspaces touch, Sidebar rows, Chords and Bulk actions flows (all scenario `workspaces` except Bulk actions, which uses `bulk-actions`). The implementer splits any flow that grows too long by moving steps to vitest, not by adding a flow. The Go tests and vitest tests above are new implementation tickets. Existing flows to update: 26 and 41 (dialog and switcher steps become page steps), and the Workspaces, Chords, Bulk actions and Sidebar rows flows (a draft row joins their seeds or steps).

**Mock states** (all in `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, fixture scenario `workspaces`; the state is driven by the flow step, and a second draft row stands for "other" in the mock):

- `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, `desktop-empty`, width 1120, fixture scenario `workspaces` (New design just pressed, nothing typed), parts: sidebar, sidebar-design-row, sidebar-epic-row, sidebar-chat-row, sidebar-draft-row, sidebar-draft-row-other, sidebar-new-design, page-header, rename, fields, project-field, slug-field, prompt, images, footer, attach, remove-button, start-blocker, start-button. The state has no saved-state text.
- `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, `desktop-filled`, width 1120, fixture scenario `workspaces` (prompt and slug set, Saved shown, Start design on), parts: sidebar, sidebar-design-row, sidebar-epic-row, sidebar-chat-row, sidebar-draft-row, sidebar-draft-row-other, sidebar-new-design, page-header, rename, saved-state, fields, project-field, slug-field, prompt, images, footer, attach, remove-button, start-button.
- `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, `desktop-remove`, width 1120, fixture scenario `workspaces` (a non-empty draft, Remove clicked; the look check ignores the prompt text, which reads "Remove this draft and its images?" in the spec), parts: sidebar, sidebar-design-row, sidebar-epic-row, sidebar-chat-row, sidebar-draft-row, sidebar-draft-row-other, sidebar-new-design, page-header, rename, saved-state, fields, project-field, slug-field, prompt, images, footer, remove-confirm (with its Keep and Remove pair).
- `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, `phone-page`, width 390, fixture scenario `workspaces` (a draft page with a prompt), parts: phone-topbar, page-header, rename, saved-state, fields, project-field, slug-field, prompt, images, footer, attach, remove-button, start-button.
- `$(gx tickets root)/design-drafts-sidebar/mocks/05-draft-page-mock.html`, `phone-sidebar`, width 390, fixture scenario `workspaces` (the sidebar open full width with draft rows), parts: sidebar, sidebar-design-row, sidebar-epic-row, sidebar-chat-row, sidebar-draft-row, sidebar-draft-row-other, sidebar-new-design.

## Out of Scope

- Drafts on the Epics page. They are not epics and would need a new card kind.
- Drafts in the TUI, and CLI verbs beyond what the route-and-verb parity tests force.
- On hold and Archive for drafts.
- A dialog that asks for the project and slug before the draft is created.
- Syncing drafts between two gx servers on different machines.
- Changes to Start design's map creation, the image limits or the design agent's image reading.

## Further Notes

- Map: `gx:design-drafts-sidebar`. Tickets 01 to 08 hold the decisions and ticket 09 (spec round 1) overrides them where they differ (the draft has no name, its title is the slug, and the slug counts as content for "empty", for example). Review round 1 overrides tickets 04 and 09, which mention a name.
- This spec builds on `docs/specs/better-new-design.md` (server drafts, images, revision conflicts) and drops its draft switcher, its "gone" banner and its rule that a draft with no content is never stored.
- Terms: Design draft, Design draft page (GLOSSARY.md). A Design draft is not a map epic and not a Not started Design, and not the ticket status Draft.
- Choices the implementer picks: the draft page's route path; the workspace membership key format for a draft (it must not clash with epic addresses or session ids); the snapshot field's wire name and which fields its rows carry (not the full prompt or the image list); route and verb names; the lease ping interval (about 30 s) and TTL (about 2 min).
