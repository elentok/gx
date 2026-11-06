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
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Before you write a flow, read "How the fixture behaves" in `skills/gx-ui-check/SKILL.md`: how a fake
worker outcome maps to ticket status, why fixture CLI writes land late, how to set an epic summary,
and "read `test-results/**/error-context.md` first on a locator failure".

The suite is capped by `maxFlows` in `playwrightguard/budget_test.go`, which equals the number of
`test(` calls in `*.e2e.ts` (124). Before you add a flow, add a step to a flow on the same scenario
(see `skills/gx-playwright-e2es`) and keep this file and `maxFlows` in step with the code.

A flow's title starts with its plan number. A flow that absorbed others lists their numbers in its
title (for example `"16. Needs you (+63, 28, 29, 30)"`). Flows with no number are named by what
they check. The plan numbers are not unique across files (72 to 78 and 117 to 121 were reused), so
run a single flow with `-g` on its full title.

Each entry below is one `test(` and says what the flow covers. The file holds the steps and
assertions; the shared helpers are in `fixture.ts`, `agentsKit.ts` and `restart-helpers.ts`.

## Epics and design (`epics`, `design`, `design-page`, `newdesign`, `united`, `dirty-trunk`)

### `epics.e2e.ts`

- **2. Epic looks right:** `mixed-epic` card, epic page ring, stats line and ticket cards before and after queueing.
- **7. Epic completion:** the epic page when the last ticket finishes.
- **6. Queue an epic, and outside changes show up:** Queue button disabled while `/v1/queue/add` runs, then the card moves to Implementing; changes made outside the page appear.

### `design.e2e.ts`

- **First step, ids, answer, Continue and a fresh round:** the first design step and an answered round.
- **1440px: the Progress region and the ring show each step in turn.**
- **Cancel stops the send and Continue stays.**
- **An all-defaulted round:** one "Accept these defaults" card and a grilling Actionable.
- **Shows the step and ticket, then follows the step report.**
- **Auto-advance:** shows while working and idle; off, then on starts the countdown and a fresh round follows.
- **Stopped session card:** live asks first, a stopped card moves to Ready to design, the menu and the banner start a new session.
- **Delete design:** appears when the session stops with the page open, and Delete removes the design.
- **50. Design epic page.**
- **Header shape:** the Design and Epic pages share the ring's and title's left edge and row at 1440px; neither scrolls sideways at 375px.
- **Epic menu and Copy ID (Epic):** in flow 2, the ⋯ menu holds only Copy epic ID and it puts `demo:mixed-epic` on the clipboard.
- **128. Copy ID (Design):** the Design page's ⋯ menu Copy design ID puts `demo:design-epic` on the clipboard.
- **Epic description popover:** click, Enter and Space open the full summary and Escape closes it; at 375px it stays inside the viewport.
- **`gx designs open` and `gx designs delete`:** open refuses an existing epic; delete removes an uncharted design and refuses a charted one.

### `design-page.e2e.ts`

- **Two live sessions:** popover under ⋯ at 1440px, the list shows the shown one, switches and stops.
- **A move starts at the title:** rows stay put, the popover sits above Send.
- **Chat, prompt, choice, titles, body and meta text** (text size).

### `newdesign.e2e.ts`

- **26. New Design form.**
- **94. New Design launches at the strongest tier.**
- **27. Design a not-started Design.**

### `united.e2e.ts`

- **16. Needs you (+63, 28, 29, 30).**
- **18. Joined card.**

### `dirty-trunk.e2e.ts`

- **58. Dirty trunk on the Epic page (and 60. Merge again).**
- **59. Stash, merge and restore.**

## Pause, queue and Codex (`pause`, `codex`)

### `pause.e2e.ts`

- **Every pause reason recovers.**
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

- **`jump.e2e.ts` 22. Back link count, then the phone layout.**
- **`phone.e2e.ts`:** a card's meta sits below its title and the answer and fix buttons are full-width tap targets; **41. Phone, New Design sheet**; **24. Phone, Design page.**
- **`modes.e2e.ts`:** the Epic page gets the wide layout and a popover jump menu (and 62. Card columns); the Design page gets the phone layout.
- **`menu.e2e.ts`:** a statusline report reaches the open menu; the amount rises while open and a closed menu stops fetching; the open menu flips to limits and its output matches the no-server run; the narrow menu fits and updates live.

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

- **`agents-sidebar.e2e.ts` 73. Agents sidebar.**
- **`agents-create.e2e.ts`:** **85. Create session**; **88. Phone, Agents page.**
- **`agents-tiers.e2e.ts`:** **95. New Agents tab session tiers**; **97. Session model label follows the handoff.**
- **`agents-session.e2e.ts`:** **75. Read a session**; **103. Rate-limit hold**; **104. Usage gate and override.**
- **`agents-composer.e2e.ts`:** **76. Send and draft (and Promote to Design)**; **78. Send refused**; **91. Removed session draft**; **93. Shared draft.**
  - **76.** Scenario: `agents-promote`. After the send steps: on a Claude and a Codex session with one message, type a draft and click Promote to Design. Pass: the fake agent receives `/gx-promote-to-design` (Codex: `$gx-promote-to-design`), the draft stays, no dialog opens. The button's disabled states and its absence on Design and Queue agents are in `AgentsPage.promote.test.tsx`.
- **`composer-images.e2e.ts`:** **193. Paste images** (`agents`, 1280 px: one PNG, then three images in one paste; thumbnails in order, "4 of 10 images"); **183. Drop images** (two files dropped on the composer); **194. Add image button** (390 px: Add shows before any image, a multi-file pick adds thumbnails, a busy session still takes images); **194. Add image button, design composer** (`design`: the picked thumbnails show on the Agents page of the same session); **187. Remove image** (the thumbnail leaves and the file leaves `<DataDir>/sessions/<id>/images`); **188. Image limits** (11 picked, ten show, "Max 10 images").
- **`composer-images-upload.e2e.ts`:** **195. Upload states** (`agents`, uploads steered with `page.route`: a spinner while held, "1 failed · 1 uploading", Send off until a retry makes every image ready); **186. Upload failures** (21 MB, SVG, HEIC and a renamed text file are refused with their message; a refused upload shows "Upload failed, tap to retry" and retrying readies it; a held-then-failed upload; the good images send once the failed one is removed); **192. Image draft** (a second tab shows the ready thumbnails; an upload still held in the first tab shows "Upload cut off" in a new tab, and turns ready there once released).
- **`chat-images.e2e.ts`:** **189. Send images** (`agents`, 1280 px: one image with no text, then three images and text with Cmd+Enter; the chat shows grids and no `[Image #N]` or path, `gx agents chat` lists the messages; the same on a Codex session); **189. Send images, design composer round** (`design`: "Send answers" delivers both pasted images to the agent); **190. Send images with a slash command** (a `/compact foo` prompt with two images pastes both images before the command); **191. Chat images on the agents-images scenario** (ten images show three thumbnails and "+7"; the viewer opens at "2 of 3" with Prev, Next and Close, Next reaches "10 of 10"; the outside-folder line shows a placeholder; a second tab and a resumed session still show the thumbnails).
- **`agents-keys.e2e.ts`:** **156. Key bar and Type field in the screen view**; **156. Type refuses !x**; **156. Error-blocked session, Type and Enter get out**; **160. Phone, key input and Blocked notices.**
- **`agents-commands.e2e.ts`:** **149. Command list freshness and failure, refresh**; **152. Phone, command menu.**
- **`agents-commands-send.e2e.ts`:** **153. Send a command, typed once or nudged, and /clear**; **154. Commands in a design session** (two flows: `/clear` and `/compact` with keys, Type and Close dialog; Continue with the countdown off).
- **`agents-commands-output.e2e.ts`:** **155. Command output in the chat, cut at 12 lines**; **155. design page, with 159. /exit.**
- **`agents-permission.e2e.ts`:** **157. Close dialog, Agents page and design page (+158 design Allow once and Deny)**; **158. Allow once and Deny, readable Claude prompt; refused; ignored Allow key; Codex hook and screen.**
