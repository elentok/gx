# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once, already signed in: the fixture finishes the web setup and `fixture.ts` adds the
`cookie_name` cookie to the browser. The `web-setup`, `web-login` and `web-logins` scenarios start
signed out or with extra web logins. `nextTotpCode()` returns a code for a new TOTP step, and
`start(scenario, { config: { "server.web.totp": "false" } })` runs without TOTP. It never reloads the page. The suite fails if a page reloads.
The `limits` scenario seeds the limits state: `start("limits", { shape })` takes `fresh` (default),
`stale`, `past-reset` or `one-window`. The `spend` scenario seeds the budget ledger
(`start("spend", { shape })` takes `normal` (default), `short` or `empty`) and holds one fake agent
running; append an assistant usage line to `fixture.agentTranscript` (create its directory first)
and the ledger, polled every second, adds its cost. The `limit-burn` scenario seeds `claude-limits.json` and the limit history
(`start("limit-burn", { shape })` takes `menu` (default), `few-days`, `stale` or `past-reset`) and holds
one Codex ticket running. To make the usage scan save a new Codex sample, append a `token_count` line
(`{"timestamp":<ISO>,"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":N,"window_minutes":10080,"resets_at":<epoch>}}}}`)
to the `rollout-*.jsonl` under `fixture.codexSessionsDir` (`codex_sessions_dir` in the fixture output). `fixture.cliInput(input, ...args)` and
`fixture.cliEnvInput(extra, input, ...args)` are `cli`/`cliEnv` with `input` on stdin.
The `triage` scenario seeds follow-ups of both categories and a Triage in the state its shape names
(`start("triage", { shape })`; default `c-few`). Each shape is its own server start, because the sidebar badge sums every project of a
workspace. Counts: `c-zero` (no open follow-up), `c-few` (3: 2 bugs and 1 idea across `demo` and `side`), `c-many` (45: 30 bugs, 15 ideas),
`c-live` (a Triage waiting for Approve, 12 rows left), `c-named` (a named workspace, "triage ws", holding both projects). Triage pages:
`verify` (45 rows, 31 verdicts saved, session running), `b-questions` (Approve, three open questions), `b-review` (Approve, no open
question, five action groups, three rows changed), `writing` (session running, two failed rows: one before its move, one after), `result`
(Done, two kept rows, a ready epic and a map epic), `stopped` (started from the CLI: no session, stopped at Plan). `journey` has
follow-ups and no record, for the Triage journey flow. Records are written with the `triage` package; the `verify` and `writing`
sessions are started with `gx server triage open`. Data: `testutil/gxfixture/spec/triage.go`.
The `launch-held` scenario queues one ticket whose worktree has a tracked `package-lock.json`, with a fake `npm`
on `PATH`: the ticket stays `claimed` at "installing dependencies" while `<gate_dir>/npm-hold` exists
(`<gate_dir>/npm-started` marks the wait), and removing the hold lets the launch go on. The `epic-at-cap` scenario
queues four independent tickets under `execution-queue.max-agents-per-epic=2` with held workers, so the third
and fourth read `concurrency cap` (`epic cap 2/2`). To lower the cap below the live count, set the key in the
JSON file at `config_file` and run `gx server reload-config`; explain then reads `epic cap 2/1`. Neither has a flow yet.
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Before you write a flow, read "How the fixture behaves" in `skills/gx-ui-check/SKILL.md`: how a fake
worker outcome maps to ticket status, why fixture CLI writes land late, how to set an epic summary,
and "read `test-results/**/error-context.md` first on a locator failure".

The suite is capped by `maxFlows` in `playwrightguard/budget_test.go`, which equals the number of
`test(` calls in `*.e2e.ts` (127). Before you add a flow, add a step to a flow on the same scenario
(see `skills/gx-playwright-e2es`) and keep this file and `maxFlows` in step with the code.

A flow's title starts with its plan number. A flow that absorbed others lists their numbers in its
title (for example `"16. Needs you (+63, 28, 29, 30)"`). Flows with no number are named by what
they check. The plan numbers are not unique across files (72 to 78 and 117 to 121 were reused), so
run a single flow with `-g` on its full title.

Each entry below is one `test(` and says what the flow covers. The file holds the steps and
assertions; the shared helpers are in `fixture.ts`, `agentsKit.ts` and `restart-helpers.ts`.

## Epics and design (`epics`, `design`, `design-page`, `newdesign`, `united`, `dirty-trunk`)

### `epics.e2e.ts`

- **2. Epic looks right:** `mixed-epic` card, epic page ring, stats line and ticket cards before and after queueing; then the `summary-notes` ticket's "Latest summary" block (and no such text on its Epics card) and the `map-evolve` evolve ticket as a top-level one-off card.
- **7. Epic completion:** the epic page when the last ticket finishes.
- **6. Queue an epic, and outside changes show up:** Queue button disabled while `/v1/queue/add` runs, then the card moves to Implementing; changes made outside the page appear.

### `design.e2e.ts`

- **First step, ids, answer, Continue and a fresh round:** the first design step and an answered round; then the session reports flow `design` and its Agents page shows no Triage label or `/triage/` link (the shared stepped-session model changed nothing).
- **1440px: the Progress region and the ring show each step in turn.**
- **Cancel stops the send and Continue stays.**
- **An all-defaulted round:** one "Accept these defaults" card and a grilling Actionable.
- **Shows the step and ticket, then follows the step report.**
- **Auto-advance:** shows while working and idle; off, then on starts the countdown and a fresh round follows; then the Auto-queue row is enabled and off, flips on, the Branch step shows "Queuing <epic> in Ns" with Cancel auto-queue, and the epic is queued at the end; after the auto-advanced round the Progress step and the Design ring are unchanged.
- **Stopped session card:** live asks first, a stopped card moves to Ready to design, the menu and the banner start a new session.
- **Delete design:** appears when the session stops with the page open, and Delete removes the design.
- **50. Design epic page.**
- **Header shape:** the Design and Epic pages share the ring's and title's left edge and row at 1440px; neither scrolls sideways at 375px.
- **Epic menu and Copy ID (Epic):** in flow 2, the ⋯ menu holds Copy epic ID and Hold, and Copy epic ID puts `demo:mixed-epic` on the clipboard.
- **128. Copy ID (Design):** the Design page's ⋯ menu Copy design ID puts `demo:design-epic` on the clipboard.
- **Epic description popover:** click, Enter and Space open the full summary and Escape closes it; at 375px it stays inside the viewport.
- **`gx designs open` and `gx designs delete`:** open refuses an existing epic; delete removes an uncharted design and refuses a charted one.

### `design-page.e2e.ts`

- **Two live sessions:** popover under ⋯ at 1440px, the list shows the shown one, switches and stops; the live Design session reports flow `design` with no Triage label or `/triage/` link.
- **A move starts at the title:** rows stay put, the popover sits above Send.
- **Chat, prompt, choice, titles, body and meta text** (text size).

### `newdesign.e2e.ts`

- **26. New Design form.** Opened from the Epics page button at 1280px (Epic slug has focus). Start, then the draft journey: autosave ("Saved") and reload, close and reopen on the same draft, `gx designs drafts` and `gx server epics`, a second context seeing the draft, conflict (Load theirs, Keep mine), a second draft with switching, naming, the row trash and Discard, and the last project picked on both devices. Then the image journey: a pasted PNG and an Add image file, the "2 of 10 images" line, Start's reasons, a held and a failed upload, the same thumbnails on the second context, the viewer, Discard with an image, Start refused with the images kept, a good Start that moves the files (`Images:` paths in the map ticket, draft folder gone, bare skill command), and the read-only strip in the Design chat.
- **94. New Design launches at the strongest tier.**
- **27. Design a not-started Design.**

### `workspaces.e2e.ts`

- **Workspaces** (scenario `workspaces`: "gx bugs" holds a live and a held Design, "research" an implementation epic and a held epic; the Default workspace holds a chat, a paused epic with two Fix agents (one Idle, so Needs you, one Working) and an epic with an Investigate agent; the scenario also seeds Claude limits, so the sidebar footer shows the limits tray; a second project "side" (own repo) holds a Design and a chat, and a flow can raise the health warning, paused and offline indicators together with `setHealth`, `gx server queue pause` and `blockEvents`; Other holds a project-level Fix agent and the One-off agents). The sidebar follows the active workspace; clicking an epic row only navigates, the chevron alone expands its agents (a page visit leaves rows as left, no auto-expand), Enter on a chat row focuses its prompt while a click or tap does not; Move from a sidebar row, an Epics card and a jump row (opening an item in another workspace switches the sidebar, a held one does not), Move is a submenu to the side listing every workspace with the current one marked (no Remove anywhere), and at 390px a tap outside closes the menu and Move swaps in place with a Back row; a chat shows in its workspace; Hold from a card, sidebar row, page header and jump row, the "Hold and end sessions" confirmation for a live Design, and the collapsed "On hold (n)" section; the Other section (same in every workspace, its Fix agents have a menu without Move or Hold and the Queue agent has none, opening a row keeps the workspace, a ticket's Fix agent nests under its epic); tree rows (a chat row and an epic row share a title x; an expanded epic's nested agent row shares its parent's title x, status column and menu button x and has no icon unless it is a Fix agent's wrench; a Queue agent has no menu button; at 1280px the button shows on its own row's hover or focus and not its parent's, and in a touch context every row, nested included, shows it; a nested Fix agent's menu has the Agent block, Stop and Remove, and Remove from the open agent's row confirms, drops the row and the session from `gx agents list` and leaves the page for the agents list); focus after a real menu closes ("+ New" then New workspace…, the switcher's New workspace…, New chat and New design (opened four times) focus Workspace name, Name and Epic slug at 1280px; at 390px New chat and New workspace leave the panel focused); the New chat and New design forms start on the project of the open Design or chat page, then on the Epics filter when it picks one project, then on the last used project (a chat created in "side" makes the Stats page forms show it); the Manage workspaces dialog (order, rename, reorder, delete naming its members, no controls on Default); CLI rename, reorder and delete reaching the open page without a reload, deleting the active workspace dropping to Default. Keyboard: rows show numbers while Alt is held, Alt+1 opens the first row, Alt+Shift+2 picks the second workspace, Cmd/Ctrl+B hides and restores the sidebar with focus on a row, j/k/arrows move, l/h expand and collapse, Enter opens. At 744px and 390px a swipe left closes the open sidebar, a vertical drag does not, and (390px, no home-screen app) the edge swipe does not open it. At 744px picking a row closes the sidebar, the jump menu is a popover, and Epics, Epic, Ticket, Design, Agents, Stats and Settings do not scroll sideways; at 390px an item outside the active workspace opens through the switcher. Footer: Epics, Follow-ups, Stats and the settings button are one size with centered icons (25px at 1280px, 34px at 390px), the Epics and Follow-ups badges share size, font and corner offset, the settings icon is three paths and no circle, and with the health, paused and offline indicators all shown no box shrinks and the 280px footer does not overflow.

### `united.e2e.ts`

- **16. Needs you (+63, 28, 29, 30).**
- **18. Joined card.**

### `dirty-trunk.e2e.ts`

- **58. Dirty trunk on the Epic page (and 60. Merge again).**
- **59. Stash, merge and restore.**

## Pause, queue and Codex (`pause`, `codex`)

### `pause.e2e.ts`

- **Every pause reason recovers.** Also starts the `hung-pane-twice` scenario: the pause says the pane stopped taking keys twice, offers Retry and Start over, and Retry ends it.
- **Unknown skill.**
- **A model-mismatch pause resumes on the worker tier.**
- **Copy, start, and refuse a second agent.**
- **Retry lifts the pause.**
- **Names the Claude app.**

### `codex.e2e.ts`

- **117. Default harness setting.**
- **118. Codex queue rows,** and **118. codex missing from PATH.**
- **120. Codex rate-limit wait.**
- **121. Codex error pause.**
- **122. Codex design run.**
- **119. Codex approval card.**

## Layout and navigation (`jump`, `phone`, `modes`, `menu`)

- **`jump.e2e.ts` 22. Back link count, then the phone layout:** the sidebar footer's Epics badge counts every card that needs you on any page and follows the server; the page title is not a button; Cmd/Ctrl+K opens the jump menu on the Epics and Agents pages; the Command palette step: Cmd/Ctrl+E opens it (filter focused, centered) on the Epics, Epic, Agents, Stats, Settings and Design pages (chat box included) and a second press closes it, the eight rows show by name, Next question lands on a Needs you card, Toggle AFK flips the gear menu's AFK switch, and on a phone the palette is a full-screen sheet; on a phone the sidebar slides in over the page and keeps the badge.
- **`phone.e2e.ts`:** a card's meta sits below its title and the answer and fix buttons are full-width tap targets; **41. Phone, New Design sheet** (the dialog panel has focus and no text field; 88% bottom sheet with grab handle, a "Close" text button at the top bar's right end and no "×"; at 375px the paperclip left of Discard and Start design at the right edge, each at natural width, with no tile, no limits line and no row for an empty draft; with the idea empty the hint sits above the footer and Start keeps its width; a file picked through the paperclip, its thumbnail click opens the viewer and returns without starting a design; Close still at the right end at 744px; Tab stays inside the dialog); **24. Phone, Design page** (Send is dimmed until an answer exists and named "Send" on a phone; at 390px the footer is one row of Add, Note, Auto, Send with 44px Add and Note, the paperclip at the left, no separator and 6px/8px padding; a visual viewport 300px shorter with the question box focused hides the title block and images and leaves the tabs with the ⋯, and restoring it brings them back; the ⋯ menu shows the model row and "Open design epic" reaches the epic page; Auto menu shows Advance and Queue and Queue flips; at Branch the Queuing countdown strip sits above the footer with a Cancel at least 40px tall; Cmd/Ctrl+K opens a full-screen jump sheet).
- **`modes.e2e.ts`:** at 760px the Epic page gets the wide layout and Cmd/Ctrl+K opens a popover jump menu (and 62. Card columns); at 390px the Design page's jump menu is a full-screen sheet.
- **`menu.e2e.ts`:** the sidebar footer's limits tray (between the health meter and the gear; the bars read C and X, fill and tone follow the 5h usage) and its popover: a statusline report reaches the open popover; the amount rises while it is open; the open popover flips to limits and its output matches the no-server run; the narrow popover fits and updates live. The tray keeps its bars fresh while closed, so a closed popover still fetches.

- **`cleanup.e2e.ts` 198. Cleanup:** scenario `cleanup`. The gear menu lists More settings, Cleanup and Tips and Tips opens its page; Cleanup shows the project and its groups, sizes fill in; Remove on the unmerged idle worktree asks first, Cancel removes nothing, Confirm removes the folder and the row leaves the group without a reload, the branch stays; "Start cleanup agent" files a one-off whose prompt names only the unresolved branches and done-but-unmerged epics.

## Stats, health, disk and checks (`stats`, `health`, `disk`, `check-lock`, `checks`)

- **`stats.e2e.ts`:** **184. Live token ledger (and 182. Stats limit burn, phone)**; **57. Stats, Grilling section, phone.**
- **`health.e2e.ts` 81. Tray popover (and 82. Tray offline).**
- **`disk.e2e.ts`:** **76. Disk banner and Clean now (and 78. Clean now fails)**; **77. Disk hold.**
- **`check-lock.e2e.ts`:** the waiting label and holder show, then clear.
- **`checks.e2e.ts` 67. Set up checks, save to repo (and 68. save to project.json, 69. refused commit, 70. phone).**

## Editor (`vi-editor`)

- Cmd-Enter in the agent composer's editor closes it and sends.
- 1280px mouse: an 860 by 680 modal; mode text left of the footer; Cmd-K over it; 1280px touch is at most 680px high.
- 390px: `j` moves one screen line inside a wrapped paragraph; a click resets the column `j` keeps.
- Escape in the editor over the question sheet closes neither.
- Tab and Shift-Tab never leave the editor; Tab leaves no tab character; touch cycles editor, Esc, Done.
- Escape closes the start-over dialog, Tab stays in it, and the editor over it keeps both open.
- Flipping the setting with the editor open shows and hides vi at once.

## Auth and settings (`setup`, `login`, `reload`, `settings`)

- **`setup.e2e.ts`:** **99. First-run setup**; **100. Setup from the QR link**; **101. Setup and login with TOTP off.**
- **`login.e2e.ts` 102. Login (+103 rate limit, +104 certificate help).**
- **`reload.e2e.ts`:** one allowed reload passes and is counted.
- **`settings.e2e.ts`:** **106. Sign out another device**; **107. Sign out of this device (+111 CLI revoke)**; **108. Change password**; **109. Reset authenticator**; **110. Web reset from the CLI**; **112. Add the authenticator after a CLI reset.**

## Restart and migration (`restart`, `restart-run`, `migration`)

- **`restart.e2e.ts`:** **129. Restart server keyboard**; **130. Restart dialog confirms**; **134. unreadable version**; **137. running agents**; **138. dry run.**
- **`restart-run.e2e.ts`:** **139. Restart with a failed migration**; **141. tab reloads**; **142. other tab**; **143. server does not return**; **145. two tabs**; **146. launchd binary.**
- **`migration.e2e.ts`:** **169. Migrating state holds scheduling**; **171. Writes wait for the migration**; **173. Failed start and Migration block**; **161. Server migrates on start.**

## Agents tab (`agents-*`, shared helpers in `agentsKit.ts`)

- **`agents-sidebar.e2e.ts` 73. Agents sidebar.** Scenario: `agents`, 1280 px. The shared sidebar is the only `complementary` and lists the agents; the Agents page has no session list of its own. A stopped session's row reads "Exited" with no reload; a removed one leaves and a newly created one arrives.
- **`agents-create.e2e.ts`:** **85. Create session** (first the Agents page "New session" button, which focuses Name; then the sidebar footer "+ New" > "New chat"); **88. Phone, Agents page** (the page alone with the sidebar slid away; "Show sidebar" and a row open a session; the footer "+ New" is an anchored popover that flips above, and its "New chat" opens the form as a bottom sheet).
- **`agents-tiers.e2e.ts`:** **95. New Agents tab session tiers**; **97. Session model label follows the handoff.**
- **`agents-session.e2e.ts`:** **75. Read a session** (the header has no standalone Promote button; its "..." menu holds "Promote to Design"; the shared sidebar has no Unread mark, so viewing a session is checked as the server's unread flag clearing; on `agents-tools` the Working row reads `running shell: <command>`, the newest of several calls plus `(+N more)`, and that button's popover opens on hover, focus and a click at 390px, listing the older calls as kind and label; flow **14. Working row** also checks the design chat's row on that scenario); **103. Rate-limit hold**; **104. Usage gate and override.**
- **`agents-composer.e2e.ts`:** **76. Send and draft (and Promote to Design)**; **78. Send refused**; **91. Removed session draft**; **93. Shared draft.**
  - **76.** Scenario: `agents-promote`. After the send steps: on a Claude and a Codex session with one message, type a draft and click Promote to Design. Pass: the fake agent receives `/gx-promote-to-design` (Codex: `$gx-promote-to-design`), the draft stays, no dialog opens. The button's disabled states and its absence on Design and Queue agents are in `AgentsPage.promote.test.tsx`.
- **`composer-images.e2e.ts`:** **193. Paste, drop and add images** (`agents`, 1280 px: one PNG, then three images in one paste; thumbnails in order, "4 of 10 images"; two files dropped on the composer; then at 390 px the Add button takes a multi-file pick, and a busy session still takes images); **194. Add image button, design composer** (`design`: the picked thumbnails show on the Agents page of the same session); **187. Remove image and image limits** (the thumbnail leaves and the file leaves `<DataDir>/sessions/<id>/images`; 11 picked, ten kept and two failed tiles with Remove, "Max 10 images").
- **`composer-images-upload.e2e.ts`:** **195. Upload states** (`agents`, uploads steered with `page.route`: a spinner while held, "1 failed · 1 uploading", Send off until a retry makes every image ready); **186. Upload failures** (21 MB, SVG, HEIC and a renamed text file are refused with their message; a refused upload shows "Upload failed, tap to retry" and retrying readies it; a held-then-failed upload; the good images send once the failed one is removed); **192. Image draft** (a second tab shows the ready thumbnails; an upload still held in the first tab shows "Upload cut off" in a new tab, and turns ready there once released).
- **`chat-images.e2e.ts`:** **189. Send images** (`agents`, 1280 px: one image with no text, then three images and text with Cmd+Enter; the chat shows grids and no `[Image #N]` or path, `gx agents chat` lists the messages; the same on a Codex session; a `/compact foo` prompt with two images pastes both images before the command); **189. Send images, design composer round** (`design`: "Send answers" delivers both pasted images to the agent); **191. Chat images on the agents-images scenario** (ten images show three thumbnails and "+7"; the viewer opens at "2 of 3" with Prev, Next and Close, Next reaches "10 of 10"; the outside-folder line shows a placeholder; a second tab and a resumed session still show the thumbnails).
- **`agents-keys.e2e.ts`:** **156. Key bar and Type field in the screen view** (the screen settles after Type and each key with no Refresh and no extra read, "Screen unchanged", Refresh is one plain read); **156. Type refuses !x**; **156. Error-blocked session, Type and Enter get out** (typed text shows as a marked row); **160. Phone, key input and Blocked notices** (settled rows with "Screen unchanged" at 390px).
- **`agents-commands.e2e.ts`:** **149. Command list freshness and failure, refresh**; **152. Phone, command menu.** (The New session form opens from the footer "+ New" > "New chat".)
- **`agents-commands-send.e2e.ts`:** **153. Send a command, typed once or nudged, and /clear**; **154. Commands in a design session** (two flows: `/clear` and `/compact` with keys, Type and Close dialog; Continue with the countdown off).
- **`agents-commands-output.e2e.ts`:** **155. Command output in the chat, cut at 12 lines**; **155. design page, with 159. /exit.**
- **`agents-permission.e2e.ts`:** **157. Close dialog, Agents page and design page (+158 design Allow once and Deny)**; **158. Allow once and Deny, readable Claude prompt; refused; ignored Allow key; Codex hook and screen.**
- **`triage.e2e.ts`:** **199. Triage journey** (`triage` shape `journey`, part 1: the Follow-ups card in Ready to design with Triage, the page opens and the card moves to Designing, a second click opens the same Triage; the Verify strip counts verdicts, the countdown chains a second session, Plan ends with no Continue and no countdown; the card is in Needs you with "Waiting for you" and "Review plan"; the sidebar badge shows the count, amber at Approve, and its popover line opens the Triage page; part 3, with one declared reload: Done lists the ready, map and merged groups with links, the card keeps the kept count and no Finished card appears, `gx server queue items` lists nothing before the Queue button, which sits left of the ring and queues the ready epic; after a reload and in a second tab the button is gone; a second Triage abandoned at Plan changes no ticket, the card shows a plain Triage button, and the page lists two past Triages newest first (abandoned, done); the Agents page session links to the Triage page; at 390 px a third Triage at Approve has no Progress panel and no sideways scroll, the view switch and Approve bar work, and the sidebar drawer shows the badge and popover).
