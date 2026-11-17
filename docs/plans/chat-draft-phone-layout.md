# Chat draft page: layout D, real defaults, images

Mock: `/private/tmp/claude-501/-Users-david-dev-gx-main/c673e4bd-6261-40aa-8bfc-8b919aba2629/scratchpad/chat-draft-mocks.html` (variant D).

Goals, from the user:

- The Folder control is the searchable combobox from the New chat dialog (`WorktreePicker`).
- On a phone the brief is the biggest element and the page does not overflow.
- Images upload like on the Design draft page.
- Title is editable and is the chat's Name. No separate Name field.
- The chat draft page shows `ChatDraftIcon`, not the Design `DraftIcon`.
- Harness, Model and Effort are chips inside the input card. A chip shows the value that will be used, never "Default".

## Decisions

- **Default values.** The server reports the effective default model and effort per harness. A chip with no override shows that value. If the server cannot resolve it, the chip shows its plain label (`Model`, `Effort`), never "Default".
- **Effort needs a model.** Keep the existing refusal. When the person picks an effort while the model is unset, the page also saves the resolved default model id, so the draft is valid.
- **Chat draft images.** A draft keeps uploaded images in its own folder. Start copies them to a stable folder and appends an `Images:` list of absolute paths to the first prompt, as `createMap` does for designs.

## Server

- [x] `commandlist.Model` gets `IsDefault` (Codex `isDefault`).
- [x] `SessionModelsResponse` gets `default_model` (resolved id) and `default_effort`. Claude: the `default` row's resolved id; no effort default known, so empty. Codex: the `isDefault` row and its `DefaultEffort`.
- [x] Regenerate `webui/src/generated/*` (`make` target in Makefile line 37).
- [x] Chat draft images: upload, delete and serve routes under `/v1/chats/drafts/images` (or the existing chat drafts route prefix), same caps as `designs/draftroutes.go`. Draft JSON lists image ids.
- [x] Start copies images and appends the `Images:` list to the brief sent to `CreateChat`. A missing draft image is skipped.
- [x] Remove deletes the images folder.
- [x] Go tests for each, run capped with `scripts/proc-cap`.

## Web UI

- [x] `ChatDraftPage` rebuilt as variant D: title row, project / folder breadcrumb, composer card, footer.
- [x] Editable title bound to `settings.name`, placeholder `Untitled chat`.
- [x] `ChatDraftIcon` in the title row.
- [x] Project pill and Folder combobox reuse `WorktreePicker` (live filter, "+ Create worktree" for the worktree folder mode).
- [x] Harness, Model, Effort chips show the resolved value; picking Effort with no model also saves the default model.
- [x] Image paste, file input, `DraftImages` strip and paperclip, via a chat-draft image API client.
- [x] Vitest for the chips (resolved value, fallback label), title edit and image attach.
- [x] `cd webui && pnpm format` on changed files; changed-file `gx check`.

## Follow-up: existing worktrees, cleanup, 390px check

- [x] Folder choice `dir` plus a `dir` save field, validated against the project's root and worktrees; Start launches there.
- [x] Folder combobox lists the live worktrees (trunk and root stay the two named rows).
- [x] Started-images folders older than 30 days are pruned when the store is built (no hook ties them to session removal).
- [x] Fix: the chat draft image GET used `WebHandler`, which a `WebRead` route ignores, so every image read panicked. It uses `Handler` now, with a web-mux test.
- [x] Fix: the chip toolbar overflowed at 390px; chips and gaps are tighter.
- [x] 390x844 check on the `chat-draft` fixture: no page overflow, brief largest, folder filter live, title edit saves, image attach shows a thumbnail, chips show values. Screenshot: scratchpad `chat-draft-390-final.png`.

## Follow-up tweaks

- [x] Paperclip moved from the card toolbar to the footer's bottom-left (`DraftFooter` `canAttach`).
- [x] On a phone (`useWide` false) the three chips become one summary pill ("Claude Opus · max") that opens a "Chat settings" bottom sheet with Harness, Model and Effort and a Done button. Wide screens keep the three chips.
- [x] Vitest: wide shows three chips and the paperclip in the footer; phone shows one pill and the sheet changes the effort.

## Follow-up: duplicates, effort, popup

- [x] Model options are unique by display name. The default row (value "") comes first; a tier that names the same model is dropped, and a selected pinned value keeps its row.
- [x] Effort never shows the bare word "Effort" as a value. The server reads Claude's `effortLevel` from `settings.json` / `.claude.json` (`harness.ClaudeEffortLevel`) for `default_effort`; with none set the chip reads "Auto".
- [x] The phone "Chat settings" is a centered `Modal`, not a bottom sheet.
