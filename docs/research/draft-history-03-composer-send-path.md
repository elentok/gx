# Draft history 03: composer send path and image storage

Ticket: `gx:draft-history/03`. All paths are relative to the repo root. Line numbers are from the
`draft-history-design` worktree at 94ef08bb8.

## Summary

- The chat composer and the design composer share one server route, `POST /v1/sessions/prompt`.
  The design composer also posts `POST /v1/sessions/answer` for a Round send, which ends in the
  same `promptSession`.
- The request carries text and image ids only. No session settings. Model, effort and harness live
  in the session meta on the server.
- gx persists the sent text nowhere. It types it into the harness. Only the byte count (run log)
  and the images (session folder) remain.
- The best capture point is the top of `promptSession`, after validation and before
  `sendPrompt`. It holds the meta (settings), the normalized text and the resolved image paths.
- Images are plain files, one per image, up to 10 per prompt and 20 MiB each, with no per-session
  or global cap.

## 1. How Send reaches the server

### Chat composer (Agents page)

- Component: `AgentComposer`, mounted at `webui/src/agents/SessionView.tsx:284`.
- Text and images are read from localStorage hooks: `useComposerDraft` (`webui/src/agents/AgentComposer.tsx:45`)
  and `useComposerImages` (`AgentComposer.tsx:46`).
- Send button and Cmd/Ctrl+Enter both run `sendText` (`AgentComposer.tsx:58-63`):
  `send({ text, ...images.payload }, onDone)`. `onDone` calls `onSent(text, readyIds)`, then
  `setText("")` and `images.clear()` (`AgentComposer.tsx:60-62`). This clears the localStorage
  draft only after the server accepted the send.
- `send` comes from `usePromptSend` (`webui/src/design/usePromptSend.ts:28-41`). It calls
  `post({ id, text: "", last_message, ...body })` (`usePromptSend.ts:33`).
- `post` defaults to `postPrompt` (`webui/src/design/prompt.ts:14-21`): `apiFetch("/v1/sessions/prompt",
  { method: "POST", body: JSON.stringify(body) })` at `prompt.ts:15`.
- Body is `wire.SessionPromptRequest` (`server/wire/sessions.go:291-310`): `id`, `text`,
  `last_message`, `continue`, `promote`, `images` (ids), `huh`.

### Design composer (Design chat page)

- Component: `Composer` in `webui/src/design/Composer.tsx`, mounted at `webui/src/design/ChatLog.tsx:101`.
- Same hooks: `useComposerDraft` (`Composer.tsx:95`), `useComposerImages` (`Composer.tsx:97`),
  `usePromptSend` (`Composer.tsx:108`).
- Free-text Send: `sendText` (`Composer.tsx:155-159`) is the same `send({ text, ...images.payload })`
  as the chat composer, so same route and same body.
- Continue button: `send({ continue: true })` (`Composer.tsx:190`). Server builds the text; no user text.
- Round send (answers to a Grilling round with the composer text as the note): `sendRound`
  (`Composer.tsx:120-146`) calls `answer({ id, source: CardSourceRound, card_id, answers, note:
  text.trim(), last_message, ...images.payload })` (`Composer.tsx:125-133`).
  `answer` is `postAnswer`, `webui/src/design/prompt.ts:8-13`, posting to `/v1/sessions/answer`.
  `onDone` clears text and images (`Composer.tsx:138-141`).

### Server handlers

- Routes: `server/sessions/prompt/routes.go:20` (`POST /v1/sessions/prompt`) and `:26`
  (`POST /v1/sessions/answer`). Both are `wire.WebWrite`. The web mux calls the `WebHandler` with
  `promptSourceWeb`; the CLI `Handler` uses `promptSourceCLI`.
- `promptHandler` `server/sessions/prompt/sessionprompt.go:27-41` decodes the body and calls
  `promptSession`.
- `answerHandler` `server/sessions/prompt/sessionanswer.go:23-37` calls `answerSession`
  (`sessionanswer.go:42`). For a Round card it builds `roundPrompt(...)` text plus `Note` and
  forwards a `SessionPromptRequest` to `promptSession` (`sessionanswer.go:73-77`). Permission,
  Codex, ticket-ask and claude-dialog answers do not reach `promptSession`.
- `promptSession` `sessionprompt.go:47-157`, in order:
  1. Locks `Watches.MetaMu` (`:48`), loads `dir, meta` via `SessionDir` (`:50`).
  2. Rejects Continue+Promote, Continue on a non-stepped session, harness with no chat
     (`:55-72`). Promote replaces `req.Text` with the skill command (`:67`).
  3. `wire.ValidatePromptText` (`:73`, `server/wire/prompttext.go:20-31`): trims, normalizes line
     ends, refuses a `!` prefix and text over 32 KiB (`MaxSessionPromptBytes`,
     `prompttext.go:9`). Empty text with no images is refused (`:80`).
  4. `resolveImages` (`:86`, `sessionimages.go:102-125`): at most 10 ids, each must exist in the
     session's `images/` folder and not already be marked sent.
  5. Web only: stale check against the last chat message id (`:92-100`).
  6. Refuses a session that is not Idle (`:101`).
  7. Builds the run-log event (`:104`) and, for a normal send, starts `sendPrompt` in the
     background (`:140`, `send.go:32`), applies `/rename` (`:141`), calls `notePersonTyped` (`:149`,
     `:160`) and `Conv.SaveHuh` (`:152`). Then appends the event (`:154`) and returns
     `SessionResult{Session: &info}` (`:158`).

### What is persisted today

- Text: not stored by gx. `sendPrompt` types it into the harness terminal
  (`server/sessions/prompt/send.go:32-120`). It survives only in the harness transcript.
  One exception: a failed send raises an Actionable that holds the text and image ids
  (`send.go:43-58`, `Action: []string{text}`, `Images: imgs.ids`) in `meta.json`.
- Run log: one `session-prompted` event with `Bytes: len(text)`, source and action
  (`sessionprompt.go:104-107`, `runlog.AppendServerEvent` at `:154`). Not the text.
- Meta: `AutoSends` reset and a derived Actionable cleared (`notePersonTyped`, `:160-176`).
  Continue also clears `Actionable` and writes meta (`:121-126`).
- Huh records for a round (`huhItems`, `sessionprompt.go:131-139`), saved through `Conv.SaveHuh`.
- Images: already on disk from the earlier upload (section 3). Send adds an empty `.sent-<id>` marker
  next to each image once it was pasted (`sessionimages.go:87-92`, `markImagesSent` `:127-135`).
- Browser: localStorage keys `gx.draft.composer.<session>`, `gx.draft.images.<session>`
  (`webui/src/drafts.ts:5-7`). The images key holds `{key, id}` pairs, ids only.

## 2. Where the server can take a Draft history copy at Send time

The request has no session settings. Everything needed is already in scope in `promptSession`.

| Item | Where it is available | Reference |
| --- | --- | --- |
| Text | `text` after `ValidatePromptText`; for a Round send this is the `roundPrompt` text built from answers plus note. Use `req.Text` before validation only if the raw text is wanted | `sessionprompt.go:73` |
| Images | `req.Images` ids (validated by `resolveImages`) and absolute `paths` of the files in `<session dir>/images/` | `sessionprompt.go:86-90`, `sessionimages.go:102` |
| Settings | `meta` from `SessionDir`: `Harness`, `Tier`, `Model`, `Effort`, `SwitchedByHand`, `AutoAdvanceOff`, `AutoQueue`, `Flow`, `Project`, `Dir` | `sessionprompt.go:50`, `server/sessions/sessiontypes.go:47-126` |
| Which list | `meta.IsStepped()`/`IsDesign()` tells design from chat | `sessionprompt.go:58` |

Recommended capture point: just before `p.sessions.Watches.Start(func() { p.sendPrompt(...) })`
(`sessionprompt.go:140`), in the `else` branch of `req.Continue`. At that point every refusal has
passed, so a history entry exists only for a send the server accepted. The handler still holds
`MetaMu`, so the copy of text and settings is consistent with the meta. The `Continue` and
`Promote` sends type server-built commands; skip them (or record them) deliberately.

Consequences to decide in the next ticket:

- Round sends reach this point through `answerSession` (`sessionanswer.go:73`). The text there is
  the formatted round, not what the person typed in the composer. The composer note is
  `req.Note`. A hook in `promptSession` would see only the formatted text. To keep the typed note
  instead, capture in `answerSession` before it builds the prompt, or pass a field.
- The send runs in the background (`Watches.Start`). A hook before it is synchronous and cannot
  see `sendPrompt` failures. A failed send is surfaced as an Actionable (`send.go:43-58`), so the
  entry may exist for a send that never landed.
- The session images are marked sent but stay in `<session dir>/images/` until the session is
  removed. A history entry can either reference those files or copy them (see section 3 for cost).
  A reference dies when the session is removed (`server/sessions/sessions.go:680`, `os.RemoveAll(dir)`).
- Both `Handler` (CLI) and `WebHandler` use `promptSession`. `source` distinguishes them
  (`sessionprompt.go:21-24`). A web-only history needs a `source == promptSourceWeb` check.

## 3. How pasted images are stored

### Shared rules (`server/imagestore/imagestore.go`)

- Accepted types: PNG, JPEG, GIF, WebP, decided from the bytes with `http.DetectContentType`
  (`imagestore.go:26-33`, `:53-56`). Name and MIME type from the upload are ignored.
- Per-image cap: `MaxBytes = 20 << 20` (20 MiB) (`imagestore.go:23`). Checked in `Check`
  (`:61-64`).
- Stored unchanged, no re-encoding or thumbnail. File name `img-<n>-<id><ext>` with `<id>` 8 hex
  characters (`imagestore.go:90-91`, `transcript/imagename.go:8-17`). `<n>` is the image count of the
  folder plus one.
- Upload body cap: `MaxBytes + 64 KiB` in all three upload routes
  (`server/sessions/prompt/sessionimages.go:23`, `server/chatdrafts/images.go:122`,
  `server/designs/draftroutes.go:196`).
- Max images per prompt or draft: `wire.MaxPromptImages = 10` (`server/wire/sessions.go:271-272`).
  The web UI checks it too (`webui/src/design/imageLimits.ts:1-2`, `checkImages` `:41-57`).
- Worst case today: 10 x 20 MiB = 200 MiB per prompt or draft. A typical screenshot is far smaller.

### Composer (chat and design composers)

- Web upload: `uploadSessionImage` posts raw bytes to `/v1/sessions/images?id=<session>`
  (`webui/src/design/sessionImages.ts:6-15`). Handler `imageUploadHandler`
  (`server/sessions/prompt/sessionimages.go:35-53`) goes through `saveSessionImage` (`:56-62`) and
  `imagestore.Save`. The session must be Idle or Working (`:75-83`).
- Location: `<SessionsDir>/<session id>/images/` (`chat.ImagesDir = "images"`,
  `server/sessions/chat/cache.go:80`). `SessionsDir` defaults to `<data>/sessions`
  (`server/server.go:222-227`).
- Per-draft cap: none on the server for a session. The 10-per-prompt limit is enforced at send time
  (`resolveImages` `:103`) and in the web UI. The server does not count the images already in the
  folder when saving, so uploads that are never sent, or sent images, stay. Only `images/delete`
  removes an unsent one (`sessionimages.go:171-189`); a sent image is refused with `ReasonImageSent`.
- Sent images are not deleted. They stay for the chat to show them. They go with the session
  folder when the person removes the session (`sessions.go:680`).
- Browser side keeps only `{key, id}` in localStorage (`webui/src/drafts.ts:10-14`,
  `useComposerImages.ts:42-`), so a reload finds the image again by id.

### Chat drafts (`server/chatdrafts`)

- Root `<stateDir>/chat-drafts/<id>/draft.json` plus `<id>/images/` (`chatdrafts/store.go:22-23`,
  `images.go:17-19`). The draft record lists image ids in `Images` (`server/wire/ops/chatdrafts.go:40-41`).
- Upload: `POST /v1/agents/drafts/images?id=<draft>` (`images.go:122-137`) to `Store.AddImage`
  (`:36-68`). Refuses past 10 images (`:49-51`, `ReasonImageTooMany`).
- Draft settings: `Harness`, `Model`, `Effort`, `Folder`, `Worktree`, `Name` (`wire/ops/chatdrafts.go:22-44`).
- On Start: `CopyImages` copies each file to `<chat-drafts root>/started-images/<draft id>/` (mode
  0600, `images.go:94-117`) because the draft is deleted afterwards (`start.go:60`, `finishStart`
  `start.go:83-`). The chat's first prompt then copies them again into the new session's `images/`
  (`server/sessions/sessions.go:766-790`, `saveFirstImages`).
- Cleanup: `started-images` folders older than 30 days are pruned when the store is created
  (`startedImagesMaxAge`, `images.go:170`, `pruneStartedImages` `:174`, called from
  `store.go:38`). It is age-based and startup-only. This is the only existing time-based image
  cleanup, and a candidate pattern for the history store.
- Removing a draft removes its folder and images (`Store.Delete` is for design drafts; chat drafts
  use `remove` (`chatdrafts/store.go:215`), called by `finishStart` (`start.go:83`)).

### Design drafts (`server/designs/drafts`)

- Root `<stateDir>/design-drafts/<id>/` with `images/` (`drafts/store.go:26`, `:66`). Record has
  `Slug`, `Idea`, `Images`, `Revision` only (`server/wire/ops/designdrafts.go:19-29`): no
  harness, model or effort.
- Upload: `POST /v1/designs/drafts/images?id=<draft>` (`server/designs/draftroutes.go:193-212`) to
  `Store.AddImage` (`store.go:321-345`). 10-image cap at `store.go:334`.
- On Start: images are not moved; `createMap` copies them into the new epic's `images/` folder
  (`server/designs/designs.go:481-499`, `:547-580`), then the draft folder is deleted
  (`designs.go:502`, `Store.Delete` `store.go:380`). A Start record is written
  (`RecordStart`, `store.go:534`).
- Remove and DeleteIfEmpty delete the folder and its images (`store.go:380`, `:402-420`).

### Disk-cap inputs for 150 history entries

- Three lists of 50 entries each hold up to 150 entries (GLOSSARY.md, "Draft history").
- Per entry, copy cost is the sum of its image file sizes. Worst case 200 MiB per entry, so
  150 entries could reach 30 GiB. A realistic entry is a few images of 0.1 to 3 MiB.
- No existing cap limits total bytes anywhere (images, session folders, drafts). Only counts (10)
  and per-file size (20 MiB) exist. A history cap must be new: total bytes, a count of images per
  entry, or reference-not-copy.
- Reference-not-copy is unsafe for the composer: session folders vanish with the session
  (`sessions.go:680`) and chat-draft started images are pruned at 30 days. Design-draft images go
  with the draft. So a history entry that outlives its source needs its own copy.
- Files are stored unchanged (no compression), and images cannot be deduplicated by name (ids are
  random). Dedup would need a content hash, which does not exist today.
