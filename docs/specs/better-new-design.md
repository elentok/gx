## Problem Statement

The New Design form is a one-draft, one-browser form. Its draft lives in the browser's localStorage, so a person who starts writing a Design on a laptop cannot continue on a phone. There is room for only one draft: a person who starts writing, then realizes something else comes first, must throw the text away or leave it blocking the form. The form does not remember which project the person used last, and a person cannot paste or add a screenshot to the idea, though the design agent would read it. On a phone the Start design button stretches to twice the width of Discard, which looks wrong.

## Solution

The work is web UI plus the gx server. The New Design form keeps its drafts on the gx server, so any browser talking to that server sees them.

- A person keeps several drafts, can name them, and switches between them from a slim title row above the form's header. The title shows the draft's label and a menu arrow, with a dim "Saving..." or "Saved" on the right. The menu lists the drafts, offers "New draft" and "Name this draft", and has a trash button per draft.
- The form opens on the most recently edited draft, or on a blank "New draft" when there is none.
- The form starts a new draft on the last project used. The Epics page filter wins when it is on exactly one project. A saved draft keeps its own project.
- A person pastes images into the idea or adds them with an Add image tile. A thumbnail row shows each image with its upload state. Clicking a thumbnail opens the existing image viewer. At Start the images join the new Design, and the design agent receives them.
- The footer holds Discard on the left and Start design on the right at its natural width, on phone and desktop.
- A second browser that changed the same draft raises an amber banner with Load theirs and Keep mine. No text is lost silently.

The approved layout is A1, "Title above", of `gx:better-new-design/04`.

## User Stories

1. As a person on a desktop, I want Start design on the right at its natural width, so that the footer looks finished.
2. As a person on a phone, I want Start design on the right at its natural width and not stretched, so that the button looks like the desktop one.
3. As a person, I want Discard on the left of the footer on phone and desktop, so that the two buttons do not sit together.
4. As a person who writes on one device, I want my draft saved on the gx server, so that I can continue it in another browser.
5. As a person, I want the draft to save while I type, without pressing anything, so that I lose nothing.
6. As a person, I want to see "Saving..." and "Saved" beside the draft title, so that I know my text is safe.
7. As a person, I want closing the form to keep the draft, so that I can come back to it.
8. As a person who starts a second idea, I want to keep several drafts, so that I can set one aside and write another.
9. As a person, I want a title row above the form showing the draft's label and a menu arrow, so that I see which draft I am in.
10. As a person with several drafts, I want a menu that lists them with project, image count and when each was edited, so that I can pick the one I want.
11. As a person, I want the draft I am editing marked "editing now" in the list, so that I know where I am.
12. As a person, I want "New draft" in the menu, so that I can start another without losing this one.
13. As a person on a blank draft, I want "New draft" to refocus the slug field instead of making a second blank draft, so that I do not collect empty drafts.
14. As a person, I want "Name this draft" in the menu, so that I can label a draft in my own words.
15. As a person, I want an unnamed draft labeled by its slug, then by the first words of its idea, then "Untitled draft" (a new blank draft reads "New draft"), so that every draft has a readable label.
16. As a person, I want the form to open on the draft I edited last, so that I continue where I stopped.
17. As a person with no drafts, I want the form to open on a blank "New draft", so that I can type at once.
18. As a person, I want a blank draft that I never typed in to leave nothing behind, so that the list holds only real drafts.
19. As a person, I want a trash button per draft, so that I can remove a draft I no longer need.
20. As a person, I want the trash to ask "Keep / Discard" on the row when the draft has text or images, and to delete at once when it is empty, so that I never lose work by a slip.
21. As a person deleting the draft I am editing, I want the next most recently edited draft to open, or a blank one, so that the form is never empty-handed.
22. As a person, I want Discard to ask "Discard this draft and its images?" when the draft has text or images, so that I do not lose them by a slip.
23. As a person on two devices, I want to be told when the draft changed on the other device, with Load theirs and Keep mine, so that neither device overwrites the other silently.
24. As a person whose open draft was deleted or started on another device, I want a banner "This draft was deleted or started on another device." with Keep as new draft, so that my text is not lost.
25. As a person, I want Start design disabled while a conflict banner shows, so that I cannot start from text I have not resolved.
26. As a person with an old localStorage draft, I want it moved to the server once, so that I lose nothing when the update ships.
27. As a person, I want the form to start on the last project I used, so that I do not pick it again.
28. As a person, I want the last project used to be the project I last picked in the form or last started, whichever is newer, so that the rule is predictable.
29. As a person with the Epics page filtered to one project, I want a new draft to start on that project, so that the form matches the page behind it.
30. As a person reopening a saved draft, I want it to keep its own project, so that switching drafts never changes a project under me.
31. As a person whose draft's project no longer exists, I want the draft to move to the new-draft project with a note "<old> no longer exists; moved to <new>", so that Start does not fail and I know why the project changed.
32. As a person on any browser, I want the last project used to be the same, so that it follows me between devices.
33. As a person, I want to paste an image into the idea box, so that I can show a screenshot.
34. As a person, I want an Add image tile, so that I can pick files when I cannot paste (on a phone).
35. As a person, I want a thumbnail row with an upload spinner, a retry tile on failure and a remove button, so that I see the state of each image.
36. As a person, I want a line below the thumbnails showing "N of 10 images", the upload or failure state, or the accepted formats and limits, so that I know the limits.
37. As a person, I want PNG, JPEG, GIF and WebP up to 20 MB and 10 per draft, the same as the session composer, so that the rules are the same everywhere.
38. As a person, I want a refused image (too large, wrong type, past the count) to say why, so that I can fix it.
39. As a person, I want to click a thumbnail to see the image large, on phone and desktop, so that I can check it.
40. As a person, I want to see the same thumbnails on my other browser, so that images follow the draft.
41. As a person, I want Start disabled while an upload runs or has failed, with a one-line reason, so that I do not start without an image I meant to include.
42. As a person, I want Start to need idea text and a slug, with a reason when one is missing, and images alone not to count as an idea.
43. As a person switching drafts during an upload, I want the upload to finish into its own draft, so that nothing is blocked or lost.
44. As a person, I want removing a thumbnail to delete its file, so that no stray images stay on the server.
45. As a person, I want Discard to delete the draft's images with it, so that nothing is left behind.
46. As a person, I want Start to move the images into the new Design and delete the draft, so that the Design owns its images.
47. As a person, I want the design agent to read my images when it charts, so that the screenshots shape the Design.
48. As a person, I want the Design's Initial prompt to list the image paths, so that a later session (a re-chart, a fresh run after `/clear`) finds the images.
49. As a person on Codex, I want the path list in the Initial prompt to reach a later Codex session, so that images work on both harnesses.
50. As a person whose Start fails, I want the draft and its images kept, so that I can try again.
51. As a keyboard user, I want Tab to cycle inside the form, so that focus never leaves the dialog, using the one shared dialog trap.
52. As a person whose save fails (server down, network error), I want the status to read "Not saved" and the form to retry, so that I know my text is not yet safe and it is not lost.
53. As a person who works from the terminal, I want `gx designs drafts` verbs (list, show, delete, image add and rm), so that I can inspect and clean up drafts without the web UI.
54. As a person in the Design chat of a Design started with images, I want a strip of the idea's thumbnails at the top, and a click to open one large, so that I can see what the agent was given.

## Implementation Decisions

### Draft model and server storage

- A draft is a server-side record in its own store in the gx state dir, one JSON file per draft. It is not a map epic: it never shows on the Epics page, in `gx tickets` or in `gx server epics`; `gx designs drafts` lists it. It becomes a map epic only at Start. Glossary: a draft is not a Not started Design.
- Fields: project, epic slug (may be empty until Start), idea, image ids, optional name, a revision number, and an edited-at time. The name is separate from the slug. The implementer picks the field names, the id format, the revision format and the folder name.
- Label: the name, else the slug, else the first words of the idea, else "Untitled draft". A blank new draft shows "New draft".
- Drafts are fetched when the form opens and when the person switches drafts. There is no live server event.
- The server alone owns a draft's image list. The upload and remove routes change it under a lock and never bump the text revision. A text save carries only project, slug, idea and name, never images, so an upload finishing during a save raises no conflict. Load theirs replaces those fields and re-reads the server's image list.
- Every text save carries the draft's revision. The server refuses a save against an old revision, and a save against a draft that no longer exists. The form then shows the amber banner above the idea: "This draft changed on another device." with Load theirs and Keep mine (Keep mine saves over it), or "This draft was deleted or started on another device." with one button, Keep as new draft, which saves the text as a new draft; its images are added again. No text is lost silently.
- Start is disabled while either banner shows, until the person resolves it. The form stays editable, and autosave pauses until the banner is resolved. Keep mine then saves the latest local text, and Load theirs drops it.
- A save that fails for another reason (server down, network error, 5xx) shows "Not saved" in place of "Saved". The form keeps the text in memory and retries on the next keystroke and every few seconds. Closing the form or the page while unsaved keeps the text in a fallback key, `gx.newDesign.unsaved`, a map from draft id to {revision, project, slug, idea, name}. The old `gx.newDesign.draft` key is read only by the one-time move. On the next open each fallback entry saves to its draft with its revision; a revision refusal raises the conflict banner and a gone refusal raises the gone banner, so nothing is created silently. An entry is removed on a successful save, on Start and on Discard. The retry timer is 5 seconds.
- A blank draft shows no status. "Saving..." and "Saved" appear from the first save of a non-empty draft.
- Saving runs 800 ms after the last keystroke, and at once on close, on switching drafts and on leaving the page. "Saving..." and "Saved" show beside the title.
- A draft with no name, slug, idea or image is never saved. It is deleted when the form closes or switches away from it. A draft with any image counts as non-empty.
- Start deletes the draft after the map epic exists. If the map write fails, the draft stays. Discard deletes the draft and its images, asking first when it has text or images. Close keeps the draft.
- There is no limit on the number of drafts. The switcher list scrolls.
- The form opens on the most recently edited draft, or on a blank "New draft" when none exists. "New draft" in the menu is the only other way to a blank one; on a blank draft it refocuses the slug field and creates nothing.
- The trash on a list row follows the Discard rule. With text or images it asks Keep or Discard on the row. With neither it deletes at once. Deleting the draft being edited opens the next most recently edited draft, or a blank one. Deleting a draft deletes its image folder.
- Old localStorage draft (the existing draft key): the first time the form opens, if it has text, it creates a server draft from it and deletes the key. If the server refuses, the key stays and the move retries on the next open. A key with no text is deleted. How the client marks the move is the implementer's choice.
- New server routes (list, read, save with revision, delete, the draft-image routes and the last-used record) follow the repo's "Adding a server route" checklist, including CLI parity and the web allowlist. The CLI verbs call the same routes as the form, so the revision, gone and image-list rules hold for them unchanged. The CLI verbs are `gx designs drafts` (list, show, delete, and `image add` and `image rm`). The implementer picks the last-used verb and any extra verbs the parity test needs. The implementer picks the route names, except the image route below. The wire types go through the generated types.

### Images

- An upload still running when its draft is deleted or started elsewhere is refused with the gone reason; the tile shows failed with "Draft is gone", and after Keep as new draft the person adds the image again.
- Draft images live in a per-draft folder in the state dir, next to the draft record. A draft-image upload route, `/v1/designs/drafts/<id>/images`, reuses the session image route's checks: PNG, JPEG, GIF and WebP, decided from the file's first bytes. The draft record lists the image ids, so every browser shows the same thumbnails.
- Limits and thumbnails reuse the composer's as they are: 10 images per draft, 20 MB each, the shared limit checks and the thumbnail row with uploading, failed and ready states. The form adds an Add image tile and paste support in the idea box only.
- A refusal for size, type or count behaves as the composer's does (flow 186): it becomes a failed tile with its reason and a remove button, and blocks Start. Retry is offered only for transport failures. The implementer copies the composer's rules exactly so they match everywhere.
- Clicking a thumbnail opens the existing image viewer, on phone and desktop.
- A file is deleted when the person removes its thumbnail. The folder goes on Discard and on deleting the draft. An empty draft deleted on close takes its folder with it.
- An upload finishes into the draft it began on. Switching drafts is never blocked. A failed upload shows as a failed tile when the person returns.
- At Start the files move into the new map epic's `images/` folder in the ticket store, next to `mocks/`, then the draft folder is deleted.
- The agent sees the images two ways. The map's `## Initial prompt` ends with a fixed lead line "Images:" and then the image paths, one per line, as absolute paths under `$(gx tickets root)/<project>/<slug>/images/`, so every later session finds them.
- The first prompt stays the bare gx-design skill command; no images are pasted into it and the session-create request gains no field. The design agent reads the images from the `Images:` list: one line is added to the gx-design skill's chart step, "Read every image listed under `Images:` in `## Initial prompt` before charting." This is an implementation task.
- The Design chat shows a read-only thumbnail strip of the map's images at the top. A small new read route serves only the map's `images/` folder: no arbitrary paths, no map frontmatter field and no schema change. The strip extends the existing thumbnail strip of the composer (`ComposerImages` and its private `Thumb` tile, which the New Design idea box also uses) with a read-only mode: no remove, no Add, no upload states. The implementer extends that component and does not build a second strip. The existing `ImageViewer` opens on a thumbnail click; it takes a session id and chat image list today, so the implementer gives it a source that also serves the map folder.
- Start is off while an upload runs or has failed (the composer's Send rule). Start needs idea text and a slug. Images alone are not an idea.

### Last project used

- The last project used is kept on the gx server in a small record next to the drafts, so every browser shares it.
- "Used" means the project last picked in the form's project select or the project of the last Start, whichever is newer. Editing text does not change it.
- A new draft starts on the Epics page filter when the filter is on exactly one project. With several projects selected or none, it starts on the last project used. A fresh server uses the first project.
- A saved draft keeps its own project when reopened, even with no text; the filter never overrides it. Changing the select on a draft changes that draft and updates the last project used. If the draft's project no longer exists, the new-draft rule applies. The form moves the draft to that project in memory and saves it with the next save, without bumping edited-at (the same rule as any open that changes nothing). A one-line note under the project select reads "<old> no longer exists; moved to <new>".
- The last-used record is not cleared when a draft is deleted.

### Form layout (from the approved mock)

- One slim row above the project and slug header: the draft label as plain bold text with a menu arrow, and a dim "Saving..." or "Saved" on the right. No box, no count, no separate New button.
- The menu (a full-width dropdown on a phone) has one row per draft (label, project, image count, "editing now" or "edited N h ago", a trash button), then "New draft", then "Name this draft". Choosing Name shows a name field under the title row with a Done button.
- Under the idea sit the thumbnail row (72 px tiles, a remove button, a spinner while uploading, a red tile with "tap to retry" when failed) and the Add image tile. A line below says "N of 10 images", the upload or failure state, or the accepted formats and limits.
- Footer: Discard on the left, Start design on the right at its natural width, on phone and desktop. Discard with text or images shows "Discard this draft and its images?" with Keep and Discard. Start is disabled with a one-line reason for: no idea, no slug, upload running, failed image. Desktop shows it beside the button, as the mock does. On a phone it shows as a one-line hint above the footer, with Start still at natural width (not drawn in the mock's 390 states).
- UI the mock does not draw reuses drawn patterns and adds no new mock: the row trash's inline Keep and Discard reuse the pair from `discard-confirm`; the "deleted or started on another device" banner reuses the amber banner of `changed-on-another-device`; "Saving...", "Not saved" and a refused-image message reuse the dim status text and the failed tile. The impl review checks these reuses.
- A width of an iPad mini follows the desktop layout.
- The form's own Tab trap is removed; the shared Modal already traps Tab and closes the topmost layer on Escape.
- New reusable UI (the draft switcher, the thumbnail row) gets a gallery entry and a render test, as `webui/README.md` requires. The form still shows a Spinner, not skeletons, while the drafts load.

## Testing Decisions

- A good test drives the form and asserts what a person sees or what the server stores. It does not assert component structure, style props or private helpers.
- Seams, highest first:
  - Go tests of the server rules through the server routes: revision refusal, gone refusal, empty-draft deletion, image checks and limits, deletion of files on remove, Discard and Start, the move to the map `images/` folder, the path list in Initial prompt, the first-prompt paste, last-used record rules.
  - Web tests of the form with injected fetchers: the footer shape, label rules, switching, naming, the trash rules, the banners, Start reasons, the project rules, the localStorage move, paste and Add image, the thumbnail states. Prior art: the existing form's test file and the composer image tests.
  - Playwright for the real round trip only. By the flow budget (`maxFlows` in `playwrightguard`; the e2e skill says extend before adding) the draft and image journey extends flow 26 "New Design form", and the phone footer extends flow 41 "Phone, New Design sheet". No new flow is written.
- Existing tests that assert the old localStorage draft or the stretched phone footer change with this epic.

## Verification

Each entry runs against a temporary gx server (`go run ./testutil/gxfixture <scenario>`, see the gx-ui-check skill). Flows are cited by number and title from `webui/e2e/CATALOG.md`. Scenario `ready` has the projects demo and scratch and is used unless stated.

1. Start right at natural width, desktop: run flow 26 (New Design form); look check: Mock states one-draft.
2. Start right on a phone: run flow 41 (Phone, New Design sheet). New assertion: at 375px Start design's width equals its content width and its right edge is at the footer's right edge; look check: Mock states one-draft.
3. Discard left: run flow 41 and flow 26. New assertion: Discard's left edge is at the footer's left edge at both widths.
4. Drafts on the server: extend flow 26 with a second browser context on the same server that opens the form and sees the draft typed in the first. CLI check: `gx designs drafts list` shows the draft and exits 0; `gx designs drafts show <id>` prints its idea; `gx server epics` does not list it.
5. Auto save: extend flow 26: type, wait for "Saved", reload the page, see the text; look check: Mock states one-draft.
6. Saving status: extend flow 26: "Saved" shows beside the title after typing; look check: Mock states one-draft.
7. Close keeps: extend flow 26: close with Escape, reopen, see the same draft.
8. Several drafts: extend flow 26: create a second draft, switch, see each draft's text; look check: Mock states several-drafts.
9. Title row: look check: Mock states several-drafts.
10. List: extend flow 26: open the menu and see label, project, image count and time per row; look check: Mock states switcher-open.
11. Editing now: look check: Mock states switcher-open.
12. New draft: extend flow 26: choose New draft, see a blank "New draft"; look check: Mock states switcher-open and no-drafts.
13. New draft on a blank draft: web test (a second blank draft is never created; the slug field has focus).
14. Name: extend flow 26: Name this draft, type, Done, the title reads the name; look check: Mock states naming.
15. Labels: web table test of the label rules: the first 5 words of the idea, cut at 40 characters with "…"; times read "just now", "N min ago", "N h ago", "N d ago".
16. Opens on last edited: extend flow 26: close, reopen, the most recently edited draft shows.
17. Opens blank: look check: Mock states no-drafts.
18. Empty draft leaves nothing: Go test of the empty-draft rule; web test that closing a blank draft saves nothing.
19. Trash: extend flow 26: trash on a row; look check: Mock states switcher-open.
20. Trash rule: web test of the inline Keep/Discard on a row with text or images, and the immediate delete of an empty row.
21. Next draft after delete: web test (the next most recently edited opens, or a blank one).
22. Discard confirm: extend flow 26: Discard with text and an image shows "Discard this draft and its images?", Discard removes the draft; look check: Mock states discard-confirm. Go test that the image folder is gone.
23. Conflict banner: extend flow 26: edit the draft in a second context, save in the first, see "This draft changed on another device." with Load theirs and Keep mine; both buttons work; look check: Mock states changed-on-another-device.
23b. Banner behaviour: web test that, while a conflict or gone banner shows, the form stays editable, autosave sends nothing, Keep mine saves the latest local text and Load theirs drops it. Go test that an upload during a text save raises no conflict, and that Load theirs re-reads the server's image list.
24. Gone banner: web test and Go test of the gone refusal, and a Go test that an upload to a gone draft is refused (the tile reads "Draft is gone"); the banner reads "This draft was deleted or started on another device." with Keep as new draft; images need adding again.
25. Start disabled during a banner: web test.
26. localStorage move: web test (a stored draft with text becomes a server draft and the key is deleted; a refusing server keeps the key; a key with no text is deleted).
27. Last project used: extend flow 26: pick scratch, close, reload and open a new draft, it starts on scratch.
28. Newer of pick and start: Go test of the last-used record and web test of the select.
29. Epics filter: web test with `filterProject` on one project for a new draft, and with none or several.
30. Saved draft keeps its project: web test, including a filter on another project.
31. Missing project: web test (the draft takes the new-draft project in memory, shows "<old> no longer exists; moved to <new>", and its edited-at time is unchanged after the save).
32. Shared across browsers: extend flow 26: the second context's new draft starts on the project picked in the first.
33. Paste: extend flow 26: paste a PNG into the idea box (a `DataTransfer` event), a thumbnail appears; look check: Mock states images-ready.
34. Add image: extend flow 26: set files on the Add image input; look check: Mock states images-ready.
35. Thumbnail states: extend flow 26 with a held upload (steered with `page.route`, as flow 195 does); look check: Mock states images-uploading-failed.
36. Limits line: look check: Mock states no-drafts, images-ready and images-uploading-failed.
37. Limits: Go table test of type, size and count checks on the draft route; web test that the form shows the refusal.
38. Refusal text: web test of each refusal message. A limit refusal is a failed tile with a remove button and no retry; a transport failure offers retry (as flow 186).
39. Viewer: extend flow 26: click a thumbnail, the image viewer opens at both widths (run at 375px in flow 41).
40. Same thumbnails elsewhere: extend flow 26: the second context shows the ready thumbnails.
41. Start reasons: extend flow 26: no idea, no slug, upload running and failed image each show their reason and keep Start off; look check: Mock states no-drafts (desktop start-hint) and images-uploading-failed. Flow 41 at 375px asserts the one-line hint above the footer with Start still at natural width.
42. Images alone: web test.
43. Switch during an upload: web test with a held upload.
44. Remove deletes the file: Go test through the route.
45. Discard deletes images: covered by entry 22.
46. Start moves images: extend flow 26: after Start, CLI check `gx tickets root` plus the map's `images/` folder holds the files and the draft folder is gone from the state dir.
47. Agent reads images: Go test that Start writes the `Images:` lines. Go test that the new read route serves only the map's `images/` folder and refuses any other path. Extend flow 26: the fake agent's first prompt is still the bare skill command. The skill line is checked by a text test of the gx-design skill.
48. Initial prompt path list: extend flow 26: `gx tickets show demo:<slug>` ends `## Initial prompt` with the line "Images:" and one absolute path per line under `$(gx tickets root)/demo/<slug>/images/`; the test asserts each file exists at its listed path.
49. Codex: a manual gx-ui-check step run once at the verify step, not a Playwright flow. Run the same Start on the `codex-design` scenario, or on a new Codex scenario if it cannot start a new Design (the one allowed new scenario). Check that both the first and a later Codex session read the `Images:` list. If Codex does not, report it as a defect to the epic.
50. Start fails: extend flow 26: the existing "already has an epic" refusal keeps the draft and its images.
51. Tab trap: web test (the form has no key handler of its own); flow 41 and flow 26 keep focus inside the dialog.
52. Not saved: web test (a failing save shows "Not saved" and retries on the next keystroke and every 5 s; closing while unsaved keeps an entry in `gx.newDesign.unsaved`; the next open saves it with its revision, a refusal raises the matching banner). Ghost case: Start while "Not saved", reopen, no draft comes back. Two-draft case: a failed save on A, a switch to B, and A's entry is still there.
53. CLI drafts: CLI checks as in entry 4, plus `gx designs drafts image add` then `image rm` on a draft and `gx designs drafts delete` removing the draft and its image folder; `TestRouteTableHasCLIVerbs` passes; after `image add` from the CLI, the open form's next text save raises no conflict; a CLI `delete` of a draft open in a browser raises the gone banner there.
54. Chat strip: extend flow 26: after Start with images, open the Design chat and see the thumbnail strip at the top with no remove, Add or upload states; click a thumbnail, the image viewer opens. Web test of the read-only mode of the composer's strip.

New flows and scenarios to write: none. Every journey extends flow 26 or flow 41. The extensions that create a second draft run after flow 26's existing "reopened modal is empty" step, or delete that draft first. If the extension makes flow 26 too long to read, the implementer splits it by moving steps to vitest, not by adding flows. The `ready` scenario is enough; the Codex check may need a scenario (entry 49).

Existing flows to update: 26 (New Design form: the "reopened modal is empty" step stays, since Start deletes the only draft) and 41 (Phone, New Design sheet).

**Mock states**

Every state is in `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`. Each state is drawn at both widths; the fixture scenario is `ready` throughout, with the state driven by the flow step.

- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `no-drafts`, width 390 and 720, fixture scenario `ready` (no draft yet; the look check skips the mock's "Saved" text, since a blank draft shows no status), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button, and start-hint at 720.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `one-draft`, width 390 and 720, fixture scenario `ready` (one saved draft), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `several-drafts`, width 390 and 720, fixture scenario `ready` (three saved drafts), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `switcher-open`, width 390 and 720, fixture scenario `ready` (three saved drafts, menu open), parts: add-image-button, close-button, discard-button, draft-item, draft-title, footer, header, idea-textarea, image-line, image-row, name-draft-item, new-draft-item, project-select, saved-status, slug-input, start-button, switcher-menu.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `naming`, width 390 and 720, fixture scenario `ready` (one saved draft, Name this draft chosen), parts: add-image-button, close-button, discard-button, draft-name-input, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `discard-confirm`, width 390 and 720, fixture scenario `ready` (one saved draft, Discard clicked; the look check ignores the prompt text, which reads "Discard this draft and its images?" in the spec; the row trash reuses this pair), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, keep-button, project-select, saved-status, slug-input.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `images-ready`, width 390 and 720, fixture scenario `ready` (one draft with ready images), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `images-uploading-failed`, width 390 and 720, fixture scenario `ready` (uploads steered with `page.route`: one held, one failed), parts: add-image-button, close-button, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, saved-status, slug-input, start-button, and start-hint at 720.
- `/Users/david/.local/share/gx/tickets/gx/better-new-design/mocks/04-draft-switcher.html`, `changed-on-another-device`, width 390 and 720, fixture scenario `ready` (a second browser context saved the draft; the gone banner reuses this amber banner), parts: add-image-button, close-button, conflict-banner, discard-button, draft-title, footer, header, idea-textarea, image-line, image-row, project-select, slug-input, start-button.

## Out of Scope

- Images in the New session form's First prompt and in the Promote to Design brief. They can reuse this work later.
- Pasting images into the first prompt, and a `Images` field on session create. The path list in the Initial prompt is how the agent gets images.
- Keeping the Epics page project choice in the URL (the rest of `gx:webui-pages/08`).
- The shared localStorage helper module (`gx:webui-sessions/01`).
- Syncing drafts between two gx servers on different machines. "Another device" means another browser on the same gx server.
- A live server event for draft changes; the form fetches on open and on switching.
- Showing drafts in epic listings: the Epics page, `gx tickets` and `gx server epics`.
- Images in the slug or name fields; only the idea accepts paste.

## Further Notes

- After this epic ships, the verify step closes or annotates the New Design parts of `gx:webui-pages/08` (which project the form starts on) and `gx:webui-dialogs/01` (the duplicate Tab trap). `gx:webui-dialogs/02` and `gx:webui-dialogs/04` describe markup the shared Modal already replaced and stay in their own epic.
- Map: `gx:better-new-design`. Tickets: 01 (draft model), 02 (images), 03 (last project), 04 (mock), 05 (round-1 answers).
- Image limits and the Prompt image term come from `gx:webui-paste-image`.

## Self-decided design questions

1. Question: Can a draft be saved with a slug that matches an existing epic?
   Choice: Yes. Only Start checks the slug, and the server's existing "already has an epic" refusal keeps the draft.
   Reason: this is today's behaviour, and drafts are not epics.
2. Question: Where may a person paste an image?
   Choice: Only into the idea box. The Add image tile covers the rest.
   Reason: the map scopes images to the idea.
3. Question: Does Load theirs replace the images too?
   Choice: Yes. It replaces the whole draft (fields and image list) with the server's copy.
   Reason: a partial replace would leave the thumbnails and the text from different revisions.
4. Question: Does "Keep as new draft" on a gone draft keep the old name?
   Choice: Yes, it keeps name, slug, idea and project; the image list starts empty.
   Reason: the old image folder went with the old draft.
5. Question: Does editing the name count as "last edited" for ordering?
   Choice: Yes, any saved change updates the edited-at time.
   Reason: one rule is simpler than a list of exceptions.
6. Question: Does opening a draft without changing it update the edited-at time?
   Choice: No.
   Reason: otherwise browsing the list would reorder it.
