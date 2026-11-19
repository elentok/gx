# Draft history

## Problem Statement

A person writes a long prompt on a Chat draft page, a Design draft page or in a composer. They press Start or Send, and something goes wrong: the start fails, the draft is removed by mistake, or the agent misreads the prompt. The text is gone. gx deletes a draft at Start and keeps a sent message nowhere, so the person has to rewrite it or dig it out of a harness transcript.

## Solution

The server keeps three lists of the latest 50 entries: Chat drafts, Design drafts, and messages sent from the chat and design composers. These three lists are the Draft history. Each entry keeps the text, the settings and copies of the images. A History button on each draft page and in each composer, or Leader then `h`, opens the History popup. The popup shows the open page's list, newest first, with a filter. Picking an entry fills the open page with it. The text that was on the page is saved to the history first, so a pick never loses text.

## User Stories

### Saving entries

1. As a person starting a chat from a Chat draft page, I want the draft saved to the Chat drafts list when I press Start, so that I can get it back if the start fails or the chat goes wrong.
2. As a person starting a Design from a Design draft page, I want the draft saved to the Design drafts list when I press Start design, so that I can get it back if the Design goes wrong.
3. As a person removing a non-empty draft, I want the draft saved before it is deleted, so that a mistaken Remove can be undone.
4. As a person who removes a draft from the CLI (`gx designs drafts delete` or the new `gx agents drafts delete`), I want the same save, so that the CLI is no way to lose text.
5. As a person, I want empty-draft cleanups (the sweep, remove-if-empty, leaving an empty draft page) to save nothing, so that the lists do not fill with blank entries.
6. As a person sending from a chat composer (a chat is a session with no flow, no ticket and no fix-of, and not a Queue agent), I want the message saved to the Sent messages list, so that I can resend or adapt it later.
7. As a person sending from a Design composer (a Design is a session with flow `design`), I want the same, so that Design messages are not lost.
8. As a person answering a Grilling round with a note in the Design composer, I want the note I typed saved, not the formatted round text, so that the entry is what I wrote.
9. As a person, I want Continue and Promote sends skipped, so that server-built commands do not fill the Sent messages list.
10. As a person, I want sends from the CLI and from sessions that are neither a chat nor a Design (Fix agents including project-level ones, ticket agents, Queue agents, other flows) skipped, with no History button in their composers, so that the list holds only what I wrote in a chat or Design composer.
11. As a person, I want an entry for a send that the server accepted even if the harness then fails to take it, so that a failed send never loses my text.
12. As a person, I want an empty draft or message (no text and no images) never saved, so that the lists hold only real writing.
13. As a person, I want an entry that repeats the newest one of its list (same text, same settings, byte-identical images) skipped, so that a failed Start followed by a second Start does not make a copy.
14. As a person, I want an entry with images and no text to be kept, so that an image-only message is not lost.
15. As a person, I want each list to hold the latest 50 entries across all projects and sessions, so that I find a draft wherever I wrote it.
16. As a person, I want the 51st entry to push out the oldest, so that the history needs no cleaning.

### What an entry keeps

17. As a person, I want a Design draft entry to keep its text, project, slug and images, so that I can restore the whole draft.
18. As a person, I want a Chat draft entry to keep its text, project, name, harness, model, effort, folder and images, so that a row tells me which draft it was.
19. As a person, I want a composer entry to keep its text, images, the session name (chat) or slug (Design), the project and, for a chat, the harness, model and effort, so that a row tells me where I sent it.
20. As a person, I want an entry to keep its own copies of the images, so that it survives the removal of the session or the draft.
21. As a person, I want the images of one list to take at most 500 MiB in total, so that the history cannot fill my disk.
22. As a person, I want the oldest entries (text and images) dropped when a new entry pushes a list over 500 MiB of images, so that the newest work is always kept.
23. As a person, I want a single entry whose images alone are over 500 MiB to keep its text and drop its images, and its row to say "images not kept", so that I still get the text back.

### Opening the History popup

24. As a person on a draft page, I want a History button in the title row beside the "Saved" status, so that I find the history where I write.
25. As a person in a composer, I want a History button between Add and Note, so that it sits with the other composer tools.
26. As a person on a phone, I want the History button to be icon only (44 px in a composer), so that the row of buttons fits.
27. As a keyboard user, I want Leader then `h` to open the same popup on a draft page or on the page of a chat or a Design (by the session rule above), so that I do not need the mouse.
28. As a person, I want the Chord box to list `h  History` after the existing rows on those pages, and not on other pages, so that the box shows only what works.
29. As a person, I want the popup to show only the open page's list: Chat drafts on a Chat draft page, Design drafts on a Design draft page, and Sent messages in both composers, so that I see what fits.

### The popup

30. As a person, I want a centered dialog, 640 px wide and at most 480 px tall, with the title "<list name> · newest first", so that it looks like the Jump menu.
31. As a person, I want a filter field with focus on open, so that I can type at once.
32. As a person, I want the filter to be a case-insensitive substring match on the text and on the row's label, project or session name, so that I find an entry by what I remember.
33. As a person, I want each row to show the text on one ellipsized line, and below it a pill, the details, the time and the image count, so that I tell entries apart.
34. As a person, I want the pill to be the slug (Design draft), "project / name" (Chat draft) or "Chat · name" or "Design · slug" (composer), so that the row names its source.
35. As a person, I want the details to be the project (Design draft, composer) or "harness · model · effort" (Chat draft), so that I see the settings.
36. As a person, I want an image icon with a count on rows that have images, and "images not kept" on a row whose images were dropped, so that I know what a restore brings.
37. As a person, I want an images-only entry to show "(images only)" as its text, so that the row is not blank.
38. As a person, I want "No match for "<filter>"" when the filter finds nothing, so that an empty list is explained.
39. As a keyboard user, I want Up and Down to move the highlight, Enter to restore, and Esc to close, with a hint line that shows the keys and "N of 50", so that I can drive it from the keyboard.
40. As a mouse user on a wide screen, I want a click on a row to restore it at once, so that one click does the job.
41. As a person on a phone, I want the popup to be a bottom sheet (full width, top 90 px) with Close and Restore buttons, where a tap marks a row and Restore restores it, so that I do not restore by a stray tap while scrolling.

### Restoring

42. As a person, I want a pick on a Design draft page to restore the text, the images and the slug, and keep the page's project, so that I do not change the target by accident.
43. As a person, I want a pick on a Chat draft page to restore the text, the images and the name, and keep the page's project, folder, harness, model and effort, so that the launch settings stay as I set them.
44. As a person, I want a pick in a composer to restore the text and the images, and nothing else, so that the session keeps its own settings.
45. As a person, I want a composer entry restored unchanged into a session of another harness, so that I can edit any `/` or `$` command myself.
46. As a person, I want a restored Design slug that is already taken by an epic or a Design to be cleared, with a notice, so that Start does not fail on it.
47. As a person, I want image files that are gone to be skipped, with the notice "N images missing", so that the rest of the entry still restores.
48. As a person, I want the text that was on the page saved to the page's own list before a pick fills it, so that a pick never loses text.
49. As a person, I want a pick of an entry equal to what is on the page to change nothing and add no copy, so that the history does not grow from idle picks.
50. As a person, I want the images on the page replaced by the entry's images, so that the page holds the entry as it was.
51. As a person, I want the popup to close after a pick, with the page's prompt focused, so that I can edit at once.
52. As a person, I want a restored draft to save like any typed edit, so that a reload keeps it.
53. As a person who pauses typing and then picks, I want the pick to cancel the page's pending autosave and adopt the draft the server returns, so that a late autosave never overwrites the entry I picked. If the draft changed elsewhere since my page loaded, I want the pick refused with the page's usual revision conflict and nothing saved, so that I do not lose the other edit.

## Implementation Decisions

Decided in the map and its tickets:

- The server keeps three lists in the gx state dir: Chat drafts, Design drafts and Sent messages (chat and Design composers together). Each holds at most 50 entries, newest first, across all projects and sessions.
- Draft entries are saved at Start and at every explicit delete of a non-empty draft, whether the caller is the web UI or the CLI. A Start that deletes the draft after it saves must make one entry, not two. Empty-draft cleanups save nothing. A failed chat launch keeps the draft and its entry, and a second Start of it is a repeat that is skipped.
- Composer entries are saved on the server in the prompt send path, just before the send starts, with no second client call. Only web sends from chat sessions and Design sessions are saved. One rule on both sides: a Design is a session with flow `design`. A chat has an empty flow, an empty ticket and an empty fix-of, and is not a Queue agent. Project-level Fix agents and Queue agents are not chats. The web UI reads the same rule from the session info it already has, so the History button, the Leader then `h` row and the server capture agree. Continue and Promote are skipped. A Round answer is saved from the answer path, with the note the person typed. A Round send whose note is empty and has no images saves nothing.
- A send that the server accepted leaves an entry even if the harness then fails to take it.
- A save is skipped when the entry is empty (no text and no images) or equals the list's newest entry. Equal means the same text, the same settings and byte-identical image files.
- Entry settings. Design draft: project and slug. Chat draft: project, name, harness, model, effort and folder. Composer entry: the session name or slug, the project, and for a chat the harness, model and effort. The ticket 03 research says the Design draft record has no settings besides slug. That is wrong: the record has a project too, and the entry reads it from the record.
- Each entry owns copies of its image files, made at save time, with no sharing between entries. Reference-only entries are unsafe because session folders and draft folders disappear.
- Cap. The images of one list total at most 500 MiB, on top of the 50-entry count cap. When a new entry pushes a list over, the server drops the oldest entries (text and images) until it fits. A single entry that is over 500 MiB alone keeps its text, drops its images and is marked, so its row says "images not kept". The 500 MiB value is one constant in the server, not a setting.
- A pick, in one server call per page. The call carries the page's current text and, for a draft page, its revision (a composer sends its text and image ids as before). The server saves that text, with the page's settings and images, to the page's own list (a draft page's list, or Sent messages for a composer), then replaces the page's text and images with the entry's. On a draft page the server writes the picked entry with the next revision and returns the new draft. The page cancels its pending autosave and adopts the returned draft. A stale revision is refused with the existing revision-conflict reason, and nothing is saved or changed. Images are copied from the entry into the draft or session image folder, and the images that were on the page are removed from it. For a draft page it also restores the name or slug as described in stories 42 and 43, and clears a taken Design slug. The call returns the notices for the page to show. Text that the browser alone holds (a composer's unsent text and image ids, which live in localStorage) goes up in this call.
- A restore never changes the page's project, folder, harness, model or effort.
- Image limits stay as they are: 10 images per draft or session prompt, 20 MiB each. A restore never brings more than 10.
- The CLI gets one new verb, `gx agents drafts delete`, which deletes a chat draft and its images and saves the entry like the server's delete route (the chat draft side of story 4). `gx designs drafts delete` already exists and gains the same save. The new verb's flags follow `gx designs drafts delete`; the implementer picks them.
- Only the web UI is in scope for the popup. The popup filter is a plain substring filter, not the fuzzy ranking of the Jump menu.

Look, from the approved mock `02-history-popup-look`:

- Variant A, the Palette dialog: centered, 640 px wide, at most 480 px tall, over a scrim, like the Jump menu. Title row "<list name> · newest first", a filter field with a search icon, two-line rows, and a hint line (up and down pick, Enter restore, Esc close, "N of 50").
- Phone: a bottom sheet with the same title, filter and rows, plus Close and Restore buttons.
- History button on a draft page: in the title row beside the "Saved" status, with a clock icon and the text "History", icon only on a phone. In a composer: between Add and Note, icon only on a phone (44 px, like Note).
- Chord box: a row `h  History` after the existing rows.

Implementer's picks (the map does not decide them):

- The storage layout, entry ids and the file format of the lists, with writes under one lock per list so that two sessions cannot lose an entry.
- The server route names and wire types for list and pick, and the name of the cap constant.
- How the 500 MiB cap is tested: the history store takes the cap as an argument that defaults to the constant. Tests pass a small cap. The constant is not a config key, there is no environment knob, and no settings page entry is added.
- The wording of the notices, the "(images only)" text and the empty-list text, as long as they say what stories 37, 46 and 47 say.
- The time text in a row. The mock shows "2 min ago", "yesterday", "Mon 14:02" and "Nov 12". The implementer reuses the web UI's existing relative-time helper if one fits.
- The name of the new fixture scenario. This spec calls it `draft-history`.
- The marker on an entry that says its images were dropped.
- The count in the hint line is the number of rows shown over the cap of 50 ("6 of 50").
- The place the History popup lives in the web UI source, and whether the draft pages and composers share one component. They must share one popup component, since the map says it is the same popup.

## Testing Decisions

- A good test checks behavior from outside: what the lists hold after Start, Remove, Send and a pick, what the page shows, what is on disk. It does not check function calls or file layout.
- Server rules are Go tests at the history store and the send and answer paths, since they need many entries, big files or a failing send: the 50 count cap, the byte cap and eviction, the over-cap single entry, the repeat rule, the empty rule, an images-only entry, concurrent saves, and the pick route (including a stale revision). Popup filter, key handling, "(images only)" and the no-match text are vitest. Web e2e is kept small: one save step added to each of three existing flows, plus one new flow, "Draft history popup".
- A Go test at the prompt send path covers the capture rules: chat and Design sessions saved by the one session rule, other sessions and CLI sends not, Continue and Promote skipped, a refused send not saved, a send accepted and then failed by `sendPrompt` still saved, a Round answer saved with its note. A Go test of the new CLI verb covers its save, its exit codes and an unknown id.
- Prior art. The existing draft flows: `chat-draft.e2e.ts` (Chat draft page, Start and Remove), `newdesign.e2e.ts` (Design draft page, images, Remove), `agents-composer.e2e.ts` and `composer-images.e2e.ts` (composers), `chords.e2e.ts` (Chord box), `phone.e2e.ts` (phone), `jump.e2e.ts` (Jump menu). Go tests for the draft stores live beside `server/chatdrafts` and `server/designs/drafts`.

## Verification

Flows come from `webui/e2e/CATALOG.md`. Scenarios `chat-draft`, `drafts`, `agents` and `design` exist. "Save step" means one step added to an existing flow. "Popup flow" means the one new flow, "Draft history popup", on scenario `draft-history`. Go tests and vitest run in the epic's own checks. Story entries that name a Mock states line use its look check and do not repeat it. Checks never list the storage layout on disk; they go through the popup, the CLI or Go tests.

New fixture scenario `draft-history` (implementer's pick of name): it seeds Chat drafts, Design drafts and Sent messages entries, each list with some entries with images and some without. One Design entry has a slug that now belongs to an existing Design. One entry has an image file deleted from disk. One entry is marked "images not kept". One entry is images only. It also seeds a chat session, a Design session and a Fix agent session with idle composers.

New work that becomes implementation tickets: the `draft-history` scenario, the Popup flow, the three save steps, the new verb `gx agents drafts delete` with its Go test, and the Go tests and vitest named below.

Entries:

1. Save step in `chat-draft.e2e.ts`: Start a chat draft. The Chat drafts popup on a new Chat draft page shows it as the newest row. Go test: a failed launch keeps the draft and its entry, and a second Start adds no second entry.
2. Save step in `newdesign.e2e.ts`: Start design on a draft with text. The Design drafts popup on a new draft page shows the entry. Go test: Start makes exactly one entry.
3. Save steps in `newdesign.e2e.ts` and `chat-draft.e2e.ts`: Remove a non-empty Design draft from the footer, from the sidebar row menu and with Leader then `x`, and a Chat draft from its page. Each adds one entry with the draft's text.
4. `gx designs drafts delete <id>` and `gx agents drafts delete <id>` on non-empty drafts exit 0, and the matching popup then shows the entry. The new verb on an unknown id exits non-zero and adds no entry. Go tests: the new verb (save, exit codes) and the server delete route for both draft kinds.
5. Go test at the draft stores: remove-if-empty, the empty-draft sweep and leaving an empty draft page add no entry.
6. Save step in `agents-composer.e2e.ts`: Send a message with text from a chat composer. The Sent messages popup shows it as the newest row with the pill "Chat · <session name>". Go test at the send path: a session with empty flow, ticket and fix-of that is not a Queue agent is saved.
7. Go test at the send path: a send to a session with flow `design` is saved to Sent messages. The Popup flow shows a seeded Design entry with the pill "Design · <slug>" in the Design composer.
8. Go test at the answer path: a Round answer with a note saves the note, not the formatted text. A round with no note and no images saves nothing.
9. Go test at the send path: Continue and Promote add no entry, and a send from the CLI to a chat adds none.
10. Go test at the send path: sends to a Fix agent (project-level too), a ticket agent, a Queue agent and another flow add no entry. Vitest on the session rule: History is offered for a chat and a Design only. The Popup flow checks the seeded Fix agent page: no History button, no `h` row in the Chord box.
11. Go test at the send path: a send is accepted, then `sendPrompt` fails, and the entry stays. A refused send (busy session, empty text, bad text) leaves no entry.
12. Go test at the history store: an entry with no text and no images is not saved.
13. Go test at the history store: same text, same settings and byte-identical images is a repeat and is skipped. The same text with an added image, or different settings, is a new entry.
14. Go test at the history store: an images-only entry is saved. Vitest: its row shows "(images only)" on line 1.
15. Go test at the history store: with 50 entries, the 51st entry drops the oldest, and the list is newest first.
16. Go test at the history store: entries of mixed projects and sessions share one list, and none is dropped by project or session.
17. Same as entry 15 (the push-out is the 51st entry).
18. Go test at the history store: a Design entry holds project, slug, text and images. The Popup flow shows a seeded Design row with its project and slug.
19. Go test at the history store: a Chat draft entry holds project, name, harness, model, effort, folder, text and images. The Popup flow shows a seeded Chat draft row with them.
20. Go test at the send path: the saved composer entry holds the session name or slug, the project and, for a chat, the harness, model and effort of the session meta.
21. Go test at the pick route: after the source session or draft is removed, the entry still restores with its images.
22. Go test at the history store with a small cap: entries with images are saved until the sum passes the cap. The oldest entries go (text and images) and the list's image bytes stay at or under the cap.
23. Go test at the history store with a small cap: one entry whose images alone pass the cap keeps its text, keeps no images and is marked. The Popup flow shows the seeded marked row with "images not kept".
24. Mock states `design-draft-wide` and `chat-draft-wide`. The Popup flow also checks the History button sits in the title row beside "Saved".
25. Mock state `chat-composer-wide`. The Popup flow checks the button sits between Add and Note in the chat composer and in the Design composer.
26. Mock states `design-draft-phone` and `design-composer-phone`. The button is icon only, and 44 px in the composer.
27. Popup flow: Leader then `h` opens the popup on each of the four pages. Mock state `chord-box`. Existing flow "Chords" in `chords.e2e.ts` still passes with the new row.
28. Mock state `chord-box` for the `h  History` row. Popup flow: on the Epics page and on a Queue agent and a Fix agent page the row does not show and `h` does nothing. Vitest on the chord's `available` rule.
29. Popup flow: the popup on each of the four pages shows only that page's list. Both composers show the same Sent messages rows.
30. Mock states `design-draft-wide`, `chat-draft-wide` and `chat-composer-wide` for size, scrim and the title row. The Popup flow at 1280 px checks the 640 px width and the 480 px max height.
31. Popup flow: the filter has focus on open and typing works with no click.
32. Vitest on the filter: a word in the text, the same word in upper case, a project name, a slug and a session name each narrow the rows, and the hint count follows.
33. Mock states `design-draft-wide` and `chat-composer-wide`: the `history-row` part. Vitest: a long text is one ellipsized line.
34. Mock states `design-draft-wide`, `chat-draft-wide` and `chat-composer-wide` (pill text for each of the three kinds).
35. Mock states `design-draft-wide`, `chat-draft-wide` and `chat-composer-wide` (details text).
36. Mock state `design-draft-wide` (image icon and count). The Popup flow shows "images not kept" on the seeded marked row in place of the count.
37. Vitest: the images-only row shows "(images only)".
38. Vitest: a filter with no match shows "No match for "<filter>"".
39. Vitest: Down and Up move the highlight, Enter restores, Esc closes with the page unchanged. Mock state `design-draft-wide` for the `hint-line` part.
40. Popup flow at 1280 px: one click on a row restores it and closes the popup.
41. Popup flow at 390 px: a tap marks a row and restores nothing, and Restore restores. Mock states `design-draft-phone` and `design-composer-phone` for the sheet, Close and Restore.
42. Popup flow, Design draft page: pick a Design entry. Text, images and slug come back and the project is unchanged (`gx designs drafts show <id>`, `gx designs drafts list`).
43. Popup flow, Chat draft page: pick a Chat draft entry from another harness. Text, images and name come back. Harness, model, effort, folder and project stay as on the page.
44. Popup flow, composer: pick a Sent messages entry from another session. Text and images come back. The session settings are unchanged (`gx agents list`).
45. Popup flow: pick a Claude composer entry that holds a `/` command into a Codex session. The text is unchanged.
46. Popup flow: pick the seeded Design entry whose slug is taken. The slug field is empty after the pick and the notice shows.
47. Popup flow: pick the seeded entry with a deleted image file. The other images restore and a notice with the count of missing images shows (wording is the implementer's pick).
48. Go test at the pick route: the page's current text is saved to the page's own list before the entry replaces it, and for a composer the unsent text lands in Sent messages. The Popup flow checks it by opening the popup after a pick.
49. Go test at the pick route: picking an entry equal to the page's content adds no entry.
50. Go test at the pick route: the page's images are removed from its folder and the entry's images are copied in. The Popup flow checks the "N of 10 images" line after a pick.
51. Popup flow: after a pick the popup is closed and the prompt box has focus.
52. Popup flow: after a pick, reload the page. The restored text, images and name or slug are still there. This is the flow's one declared reload.
53. Go test at the pick route: a pick with the current revision writes the entry with the next revision and returns the new draft. A pick with a stale revision is refused with the revision-conflict reason and saves and changes nothing. The Popup flow types, picks within the autosave delay, and the picked text is what a reload shows.

**Mock states**

- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · design-draft-wide · 860 px · `draft-history` on a Design draft page with the Design drafts popup open · history-button, history-popup, filter-field, history-row, hint-line
- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · chat-draft-wide · 860 px · `draft-history` on a Chat draft page with the Chat drafts popup open · history-button, history-popup, filter-field, history-row, hint-line
- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · chat-composer-wide · 860 px · `draft-history` in the chat composer with the Sent messages popup open · history-button, history-popup, filter-field, history-row, hint-line
- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · design-draft-phone · 390 px · `draft-history` on a Design draft page with the Design drafts sheet open · history-button, history-popup, filter-field, history-row
- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · design-composer-phone · 390 px · `draft-history` in the Design composer with the Sent messages sheet open · history-button, history-popup, filter-field, history-row
- `$(gx tickets root)/draft-history/mocks/02-history-popup-look.html` · chord-box · 860 px · `draft-history` on a Design draft page with the Chord box open · history-button, history-chord

The mock shows the phone sheet's Close and Restore buttons but gives them no part name. The phone states check them by their text. The mock's Chord box rows (`n`, `d`, `h`, `j`) are a sketch. The real box keeps its existing rows and adds `h  History` after them.

New scenario for every Mock states line: `draft-history`, as described above.

## Out of Scope

- Other prompt boxes (answer boxes, deny note, start-over reason, schedule prompt).
- The TUI, and any CLI verb to list or restore history. The only new CLI verb is `gx agents drafts delete`; the CLI otherwise only triggers saves through delete.
- Deleting single history entries. The count cap is enough for the first version.
- Per-session composer lists, and picks across kinds (a Chat draft entry into a Design draft page).
- Saving sends from the CLI, from Fix agents, ticket agents, Queue agents and other flow sessions.
- Fuzzy ranking in the popup.
- A setting for the 50-entry or the 500 MiB cap.

## Further Notes

- GLOSSARY.md already defines Draft history and History popup. The spec uses those terms.
- Disk use is bounded at about 1.5 GiB for all three lists (500 MiB per list), plus the text.
- The cap counts image bytes only. Text is small and bounded by the 32 KiB prompt limit and the 50-entry cap.
- Research: `docs/research/draft-history-03-composer-send-path.md` has the send path and the image storage facts.
