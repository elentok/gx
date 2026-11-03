## Problem Statement

The Design page and the Epic page head their content in different ways, and both waste space. On a Design, the title row holds a ring, a title with a visible "⌘K" hint, three agent pills (`Claude`, `Worker`, `claude-sonnet-5-5 · medium`) and a boxed 44px "⋯" button with misaligned dots, no hover state and a dull look. On a phone these pieces wrap badly. The Epic page has no menu at all, and its title row looks nothing like the Design's.

Agent details are noisy. The tier, the harness dot and the raw model ID all show, though the person mostly wants the model. Raw IDs such as `claude-sonnet-5-5` also appear wherever a model shows as a label (the Agents tab, ticket cards, recovery text, the new-session tier picker). They read as code, not as names.

## Solution

This epic is web only. The server does not change.

Both pages share one header: a ring beside the title with its jump caret and one line of text (a Design shows its step, an Epic shows its description), then a hairline and a meta bar. There is no visible "⌘K" hint; the shortcut still works. A round ghost "more actions" button with three centred dots sits at the right of the title row. It fills on hover, shows a border while its menu is open and shows a focus ring on keyboard focus.

- A Design's meta bar shows the model, joined with the harness, and the effort on desktop (`Claude Sonnet 5.5 · medium`), and only the model on a phone. It never shows the tier or a harness dot.
- An Epic's meta bar keeps the stats line on the left and the Open design, Open implementation epic, Queue and Merge buttons on the right. On a phone the buttons wrap to a full-width row of 44px buttons.
- Whatever the header hides goes in the menu. The menu opens as a popover on desktop and as a bottom sheet on a phone.
- The Design menu has an Agent block (Tier on desktop; Tier and Effort on a phone; a "switched by hand" note when it applies), New session, live sessions, Copy design ID and Delete design.
- The Epic phone header hides nothing, so the Epic menu has no data block. It has only Copy epic ID, which copies `<project>:<name>`. The pause rows come from `queue-and-epics`, which fills this menu later. If a later change hides something in the Epic header, it goes in the menu.
- The Epic description stays visible on one truncated line. Clicking it opens a popover with the full text.
- Everywhere a model shows as a label it reads as a pretty name joined with the harness, such as "Claude Sonnet 5.5" or "Codex GPT-6.1-Sol". One pure web function formats it.

The approved mock is direction A2 of `gx:better-epic-header/01`.

## User Stories

1. As a person on the Design page, I want the title, ring and step on one tidy block, so that I see where I am without clutter.
2. As a person on the Epic page, I want the same title, ring and one-line layout as the Design page, so that both pages feel like one product.
3. As a person on a phone, I want the header to fit the narrow screen with the ring, title and menu button aligned and no wasted rows, so that more of the chat or the tickets is visible.
4. As a person on a desktop, I want the ring to match the height of the title plus its line, so that the header lines up.
5. As a person using the jump menu, I want the title to show only a caret and no "⌘K" text, so that the title stays clean.
6. As a keyboard user, I want Cmd/Ctrl+K to still open the jump menu, so that I lose no speed.
7. As a person on the Agents tab, I want the session switcher title to lose its "⌘K" text too, so that every title looks the same. (The empty state's "press ⌘K to search" is help text and stays.)
8. As a person on a Design, I want a round ghost "more actions" button with three centred dots, so that the button looks finished and its dots line up.
9. As a mouse user, I want the menu button to fill on hover, so that I see it is clickable.
10. As a keyboard user, I want a visible focus ring on the menu button, so that I know where focus is.
11. As a person with a menu open, I want the button to keep a border while the menu is open, so that I know which button owns the menu.
12. As a phone user, I want the menu button to be at least as large as a touch target, so that I can tap it.
13. As a person on a Design, I want the header to show the model with its effort on desktop, so that I see the most useful agent detail at a glance.
14. As a person on a Design on a phone, I want the header to show only the model, so that it fits.
15. As a person on a Design, I want the header to never show the tier or a harness dot, so that the header stays quiet.
16. As a person on a Design, I want the tier (and the effort on a phone) in the menu's Agent block, so that I can still find what the header hides.
17. As a person whose model was changed by hand, I want the menu to say so, so that I know the session is not on its launch model.
18. As a person on a Design or Agents session whose agent has not replied yet, I want the label to read "<Harness> CLI default" (with the effort on desktop in the Design header), so that I am not shown a guess.
19. As a person on a Design with no session yet, I want a header with no meta bar, so that I see nothing that is not there.
20. As a person on a Design, I want the Start button and any failed-start message to stay visible in the title row, so that I can still start the session.
21. As a person on a Design, I want the step line on desktop as well as on a phone, so that the header says what the Design is doing.
22. As a phone user, I want tapping the ring or the step line to still open the progress steps sheet, so that I lose no feature.
23. As a person on a Design, I want New session, the live sessions and Delete design in the menu as today, so that nothing moves away.
24. As a person on a Design, I want Copy design ID in the menu to copy `<project>:<name>`, so that I can paste the address into a CLI command.
25. As a person on a Design with no session and no delete right, I want the menu to still open with Copy design ID, so that the menu always has at least one item.
26. As a person on a phone, I want the menu to open as a bottom sheet with a grab handle and 44px rows, so that I can use it with a thumb.
27. As a person on a desktop, I want the menu to open as a popover under the button, so that it stays near the pointer.
28. As a person on an Epic, I want a more-actions menu with Copy epic ID, so that I can copy the epic address.
29. As a person on an Epic, I want Copy epic ID to copy `<project>:<name>`, so that the copy matches the Design menu.
30. As a person on an Epic, I want the menu to make room for the pause items of `queue-and-epics`, so that the menu does not need a redesign when they arrive.
31. As a person on an Epic, I want to see the description on one line under the title, so that I still know what the epic is.
32. As a person on an Epic, I want to open the description with a click, Enter or Space and close it with Escape, so that long descriptions are readable by mouse and keyboard.
33. As a person on an Epic with no description, I want the header to show just the ring and title, so that no empty line wastes space.
34. As a person on an Epic, I want the stats line and the Queue, Merge and Open design buttons to stay visible in the row under the title, so that the main actions are one click away.
35. As a phone user on an Epic, I want those buttons in a full-width row at 44px, so that I can tap them.
36. As a person on a Design epic page (a map), I want the same header and menu as any Epic, so that the page is not a special case.
37. As a person reading the Agents tab, I want a session's header to read one joined name such as "Claude Sonnet 5.5", with no tier pill, so that I do not decode `claude-sonnet-5-5`.
38. As a person on the Agents tab (on any session, queue sessions included), I want the tier and "switched by hand" in the session's more-actions menu, so that the header stays quiet like the Design header.
39. As a person reading a running ticket card, I want the model on the card to read as the joined name with the tier and effort, such as "Claude Sonnet 5.5 · Worker · medium", so that cards match the headers.
40. As a person reading a model-mismatch recovery message, I want "Ran on Claude Opus 4, expected Claude Sonnet 4.", so that the two names are easy to compare.
41. As a person creating a session, I want the tier picker to read "Worker (Claude Sonnet)", so that the model name is readable.
42. As a person using Codex, I want "Codex GPT-6.1-Sol" in the same places, so that Codex rows read like Claude rows.
43. As a person whose session was saved with an alias such as `sonnet`, I want it to read "Claude Sonnet", so that it is readable before the first reply.
44. As a person whose model ID is unknown (a custom ID), I want to see it with its harness, so that nothing breaks or disappears.
45. As a person with a 1M-context model, I want the name to say "(1M)", so that I can tell the variants apart.
46. As a person reading older text such as `no model` in a mismatch message, I want it left as it is, so that the message stays true.
47. As a developer, I want a branded `ModelLabel` type that only `modelName` returns, so that `tsc` rejects a raw ID wherever a label is expected.

## Implementation Decisions

### Shared header

- One shared page-header component serves the Design page and the Epic page. It takes: the ring, the title control (the jump menu), one line of text, a menu, a meta bar slot and a right-hand slot for page-specific controls (the Design's Start button and failure text).
- Every "desktop versus phone" rule in this spec uses the existing `useWide` breakpoint.
- Ring: 56px on desktop and 44px on a phone, beside the title. It matches the height of the title plus one line, so an Epic with no description lines up with the same ring height. The Design ring keeps its pencil badge, its progress colour and its comet; the Epic ring keeps its own.
- The title keeps its condensed face and its jump caret. The title block is the same on both pages.
- The line under the title truncates to one line with an ellipsis. An Epic shows its description. A Design shows its step in the mock's two forms: long on desktop ("Resolve tickets · 08 Decide the guidelines … (8 of 16)"), short on a phone ("Resolve tickets · 8 of 16").
- The meta bar sits under a hairline. A Design shows the agent part; an Epic shows its stats line and its buttons, moved unchanged from the current Epic bar. Epic buttons on a phone wrap to a full-width row at 44px.
- A merge-blocked Epic keeps its merge-block notice under the bar.
- The Design page keeps its current step-sheet behaviour on a phone: the ring and the step line open the progress steps sheet.
- The Start design session button and the failed-start message stay in the title row, left of the menu button, whether or not a session exists.

### Jump menu hint

- The visible "⌘K" text is removed from the shared row-list title, so it goes from the epic jump menu and the Agents session switcher alike. The Cmd/Ctrl+K toggle and its `hotkey` switch stay. The Agents empty state's "press ⌘K to search" stays: it is help text.

### Menu button and menu

- A new shared "more actions" menu component replaces the Design-only actions menu trigger and is used by both pages. Its trigger is a round ghost button (36px on desktop, 40px visible on a phone with a touch area of at least 44px), three centred dots, accessible name "More actions". States: rest, hover (filled), keyboard focus (focus ring), open (filled with border).
- The menu content is supplied by the page as sections, so `queue-and-epics` adds its items without touching the shell. The shell owns: popover on desktop, bottom sheet with a grab handle on a phone, 44px rows on a phone, close on pick, the Escape and outside-click behaviour of the current menu.
- The menu always renders, because both menus carry a Copy ID item.
- Design menu, top to bottom: Agent block (key-value rows: Tier on desktop; Tier and Effort on a phone; a "switched by hand" row when the model was switched), a divider, New session (with its current subtitles), the live-sessions rows (when more than one is live), a divider, Copy design ID (subtitle `<project>:<name>`), Delete design (destructive, with today's confirm).
- Epic menu in this epic: Copy epic ID (subtitle `<project>:<name>`) only. The mock shows Soft pause, Hard pause, Resume, Remove from queue and Auto-merge greyed under "Added by queue-and-epics" for design only; nothing for them ships here.
- Copy ID uses the existing `copyText` helper from `webui/src/clipboard.ts`, with no extra failure UI. The copied text is always `<project>:<name>`.
- The Design menu has no pause items. The server never schedules a Design.

### Agent details in the Design header

- Header text: desktop `<Harness> <Model> · <effort>`, phone `<Harness> <Model>` (the joined name from `modelName`). The effort is left out when empty.
- With no model yet, the label reads `<Harness> CLI default` (desktop adds ` · <effort>`). The tier is never shown in the header. "<Harness> CLI default" is the no-model label in every place a model shows.
- The Agent block in the menu lists exactly what the header hides at that width.
- The header has no harness dot.

### Agents tab and ticket cards

- The Agents session header follows the Design header: one joined name, no tier pill. Desktop reads `<name> · <effort>`, a phone reads `<name>` only, with Effort in the Agent block on a phone. The tier and the "switched by hand" note move into the session's more-actions menu as an Agent block.
- The Agents session view always renders the shared more-actions menu and button with the Agent block, because queue sessions have no actions today (`SessionActions` returns null when `!canManage(session)`). Stop and Remove stay limited to `canManage` sessions. `SessionActions` drops its own boxed 44px button, so every "More actions" looks the same.
- Ticket cards have no menu. They keep the harness dot, then the joined name, the tier and the effort: "Claude Sonnet 5.5 · Worker · medium". With no model: "Claude CLI default · Worker".
- The pill form of the shared harness label goes away on the Agents session header; the plain form stays on ticket cards.

### Epic description

- The description (the epic summary) stays on one truncated line under the title at every width. The line is a button: click, Enter or Space opens a popover with the full text, rendered as the page renders the summary today (Markdown). Escape closes it. It always opens, even when the text fits. On a phone the popover fits inside the viewport. An epic with no summary shows no line.

### Model names

- One pure web function `modelName(id, harness?)` is the only code that formats a model. It returns a branded `ModelLabel`. Components that show a model label (`HarnessLabel`, the Design header, the Agents session header, the recovery text, the tier picker) take a `ModelLabel`, so `tsc` rejects a raw ID.
- Output is the harness joined with the pretty name: "Claude Sonnet 5.5", "Codex GPT-6.1-Sol".
  - Claude: strip `claude-` and a trailing `-YYYYMMDD`; capitalise the family; join the digits with `.`. An ID that does not match `claude-<family>-<digits>` (for example old-order `claude-3-5-sonnet-20241022`) passes through the unknown rule below.
  - Suffix: `[1m]` becomes ` (1M)` on IDs and aliases (`sonnet[1m]` reads "Claude Sonnet (1M)"). Any other `[...]` suffix is dropped.
  - Aliases are `opus`, `sonnet` and `haiku`. An alias reads "Claude Sonnet".
  - Codex: keep the ID's hyphens and capitalise each word (`gpt` becomes `GPT`), so `gpt-6.1-sol` reads "GPT-6.1-Sol".
- The harness is the optional second argument. With no harness, `modelName` infers it from the ID: `claude-*` and the Claude aliases read as Claude, `gpt-*` as Codex. The mismatch recovery text has no harness (`TicketDetail` sends only `model_ran` and `model_expected`, and the guard runs for Codex too), so it relies on the inference.
- Table rows (the `modelName` table test covers each):
  - `claude-sonnet-5-5` with harness Claude: "Claude Sonnet 5.5".
  - `claude-opus-5[1m]`: "Claude Opus 5 (1M)". `sonnet[1m]`: "Claude Sonnet (1M)".
  - `sonnet`, `opus`, `haiku`: "Claude Sonnet", "Claude Opus", "Claude Haiku".
  - `claude-3-5-sonnet-20241022` (old order): passes through (joined with the harness when known).
  - `gpt-6.1-sol` with no harness: "Codex GPT-6.1-Sol".
  - An empty ID with a harness: "<Harness> CLI default", such as "Claude CLI default".
  - An unknown ID with a harness (a custom ID): "<Harness> <id>", such as "Codex my-model".
  - Any text with no harness known, such as `no model` on the recovery text: returned as is, with no harness joined.
- The model name rule is a web rule only. The server sends no name field and reads no model catalog.
- Decision: rewrite the "Agents tab: model display" rule in `docs/specs/design-models.md` (line 187, "A reported ID is shown as is, for example `claude-sonnet-5-5`") in this epic, so that a reported ID reads as its `modelName`. The two specs then agree. All other rules in that section stay.

## Testing Decisions

- A good test drives the page and asserts what a person sees. It does not assert component structure, style props or private helper calls.
- Seams:
  - A pure table test of `modelName`: Claude IDs, dated IDs, `[1m]`, aliases, Codex IDs, `no model`, unknown text.
  - Web page tests of `DesignPage`, `EpicPage` and the Agents session view, with the injected fetchers and a connection snapshot. They cover the header text at both widths, the missing ⌘K text, the menu items per page, Copy ID, the description popover (keyboard too) and the model names.
  - The e2e flows of `webui/e2e/CATALOG.md` against a fixture server, for the real browser look.
- Prior art: `SessionControls.test.tsx` and the Design actions menu tests; `HarnessLabel.test.tsx`; `rows.test.ts`; `EpicPage.test.tsx`; `DesignPage.test.tsx`. Chakra menu tests follow the recipe in `webui/README.md`.
- Existing tests that assert raw IDs or the old menu button change with this epic: the harness-label, rows, epic page, Agents session view, design page, action card and new-session form tests, and the e2e flows listed in Verification.
- New reusable UI gets a gallery entry and a render test, as `webui/README.md` requires: the more-actions button (rest, hover, focus, open) and the shared header.

## Verification

Each entry runs against a temporary gx server (`go run ./testutil/gxfixture <scenario>`, see the gx-ui-check skill). Flows are in `webui/e2e/CATALOG.md`, cited by number and title because numbers are not unique there.

1. Design page block: run flow 64 (Design ring) and flow 24 (Phone, Design page); look check: Mock states design-desktop and design-phone.
2. Epic page matches Design: run flow 2 (Epic looks right) and flow 50 (Design epic page); look check: Mock states epic-desktop and epic-phone. New flow "Header shape" opens `#/d/` and `#/e/` for one epic at 1440px, and asserts the ring, the `h1` and the line share the left edge and the same row for both.
3. Phone fit: run flow 53 (Phone, epic page) and flow 24 (Phone, Design page); look check: Mock states design-phone and epic-phone. New assertion in "Header shape" at 375px: no sideways scroll.
4. Ring height: look check: Mock states design-desktop and epic-desktop.
5. No ⌘K text: run flow 20 (Jump menu opens and closes). New assertion: the title button's text has no "⌘K" at 1440px. Update flow 20.
6. Shortcut works: run flow 20 (Jump menu opens and closes; Ctrl+K opens the "Jump to epic" dialog) and flow 21 (Jump menu keyboard and picks).
7. Session switcher: run flow 84 (Session switcher). New assertion: the switcher title has no "⌘K" text and Ctrl+K still opens it. The empty state still says "press ⌘K to search".
8. Round ghost button: look check: Mock states menu-button-states (rest) and design-desktop.
9. Hover: look check: Mock states menu-button-states (hover). New flow "Menu button states": hover and see the filled background; check a computed style change.
10. Focus ring: look check: Mock states menu-button-states (focus). New flow "Menu button states": press Tab to the button and assert a visible outline.
11. Open state: look check: Mock states menu-button-states (open). New flow "Menu button states" asserts the border while the menu is open.
12. Touch target: run flow 53 (Phone, epic page), flow 72 (Design actions menu) and flow 75 (Read a session) at 375px. Update flow 72 (Design actions menu): the button is round, at least 44px in its hit area, and has no 1px square border at rest. The Agents session "More actions" button (`SessionActions`) uses the same shared button and passes the same check.
13. Desktop model and effort: run flow 96 (Session model label) and flow 98 (Session model label on an interactive session); look check: Mock states design-desktop. Update flow 96 (Session model label) and flow 97 (Session model label follows the handoff) to read the names the fixture gives. `design-model-tiers` launches the aliases `opus` and `sonnet` and the fake reports no model, so the headers read "Claude Opus · max" and "Claude Sonnet · medium" (flows 96, 97 and 98, and the resumed-session check of flow 95, which today asserts `opus · max`), with no tooltip on a tier pill. The design-desktop look check text is "Claude Sonnet · medium", while the mock shows "Claude Sonnet 5.5".
14. Phone model only: look check: Mock states design-phone. New assertion in flow 96 (Session model label) at 375px: the meta bar reads only the model.
15. No tier or dot in the header: run flow 96 (Session model label) and assert no "Worker" or "Strongest" text and no harness dot in the header; look check: Mock states design-desktop and design-phone.
16. Tier and effort in the menu: run flow 72 (Design actions menu); look check: Mock states design-menu-phone. New assertion: the Agent block reads Tier on desktop, and Tier and Effort at 375px.
17. Switched by hand: no new design fixture. Test the Design menu's switched row in `DesignPage.test.tsx` with `model_switched: true`. For e2e, run the switched `dotfiles` session of `default-harness` (flow 117, Default harness setting): the menu's Agent block shows the switched note and the header shows none.
18. CLI default: new fixture step in `design-model-tiers`: a Design session with an empty tier model. The header reads "Claude CLI default" with no tier. Update every assertion in `codex.e2e.ts`: test "117. Default harness setting" (lines 72-80) asserts "Worker" exact, the raw `${model} · ${effort}` and "switched by hand" in the header, so it now reads the joined name in the header and the tier and the switched note in the menu; the tier-without-model test (line 89-90) asserts "Claude" exact and "Worker · CLI default", so it now reads "Claude CLI default" in the header and the tier in the menu.
19. No session: run flow 4 (Start a design session) and flow 27 (Design a not-started Design). New assertion: no meta bar before Start.
20. Start and failure stay: run flow 4 (Start a design session), flow 44 (Failure banner New session) and flow 43 (New session on a stopped Design). Update them if a locator moves.
21. Step on desktop: run flow 9 (Design progress to the end); look check: Mock states design-desktop (step part). Assert the long form on desktop and the short form on a phone.
22. Steps sheet on a phone: run flow 24 (Phone, Design page; ring and step line open "Design progress steps").
23. Menu items kept: run flow 72 (Design actions menu), flow 45 (Delete design) and flow 43 (New session on a stopped Design). CLI check: `gx agents list` shows one exited session after Stop; `gx designs delete` per flow 46 (gx designs delete) still works.
24. Copy design ID: new flow "Copy ID": in scenario `design`, open the Design menu, click "Copy design ID", read the clipboard. Expected `demo:design-epic`. Look check: Mock states design-menu-phone.
25. Menu always renders: update flow 72 (Design actions menu), which today says "Before Start there is no More actions button": the button now exists and the menu holds only Copy design ID.
26. Phone sheet: run flow 72 (Design actions menu) at 375px (full-width bottom sheet with a grab handle); look check: Mock states design-menu-phone and epic-menu-phone.
27. Desktop popover: run flow 72 (Design actions menu) at 1440px (popover, no grab handle); look check: Mock states epic-menu-desktop.
28. Epic menu: new flow "Epic menu": in scenario `mixed`, open `#/e/demo:mixed-epic`, open "More actions", see only "Copy epic ID". Look check: Mock states epic-menu-desktop and epic-menu-phone.
29. Copy epic ID: new flow "Copy ID": in scenario `mixed`, click "Copy epic ID" on `#/e/demo:mixed-epic` and read the clipboard. Expected `demo:mixed-epic`.
30. Room for pause items: unit test of the shared menu shell with a second section; no e2e (the items ship in `queue-and-epics`).
31. Description visible: run flow 50 (Design epic page; the summary shows under the title); look check: Mock states epic-desktop and epic-phone.
32. Description popover: new flow "Epic description popover": set a long summary with `gx tickets section`, click the description, see the full text in a popover; Escape closes it; Enter and Space open it from the keyboard. Look check: Mock states epic-description-popover-desktop. Also check the popover stays inside the viewport at 375px.
33. No description: run flow 2 (Epic looks right) on an epic with no summary (`mixed-epic`); assert no empty line and the ring still aligns with the title.
34. Stats and buttons kept: run flow 2 (Epic looks right), flow 17 (Merge from the card) and flow 51 (Merge from the epic page); look check: Mock states epic-desktop. Update locators if needed.
35. Phone buttons: run flow 53 (Phone, epic page): every bar button below the stats line and at least 44px tall; look check: Mock states epic-phone.
36. Design epic page: run flow 50 (Design epic page) and flow 18 (Joined card: "Open design" and "Open implementation epic" buttons); look check: Mock states epic-desktop.
37. Agents tab model: run flow 98 (Session model label on an interactive session) and flow 75 (Read a session). Update them to read "Claude Sonnet 5.5" with the tier in the more-actions menu. No new fixture: the switched session of `default-harness` already reports the full ID `claude-haiku-5-5`, so flow 117 (Default harness setting) asserts "Claude Haiku 5.5" on that session.
38. Agents menu Agent block: run flow 75 (Read a session) and flow 117 (Default harness setting), open "More actions", see Tier and the switched note. This holds for a queue session too (the menu always renders there).
39. Ticket card model: run flow 2 (Epic looks right) and flow 118 (Codex queue rows). New assertion: a running card's chip keeps its harness dot and reads the joined name with the tier and effort, such as "Claude Sonnet 5.5 · Worker · medium" or "Codex GPT-6.1-Sol · Worker"; with no model "Claude CLI default · Worker".
40. Mismatch text: run flow 3 (Pause reasons and recovery), the model-mismatch case. Update `pause.e2e.ts` (line 202, today `claude-opus-4` ran and `claude-sonnet-4` expected) to expect "Ran on Claude Opus 4, expected Claude Sonnet 4.".
41. Tier picker: run flow 95 (New Agents tab session tiers). Update to expect "Worker (Claude Sonnet)".
42. Codex names: run flow 118 (Codex queue rows) and flow 122 (Codex design run). Expected "Codex GPT-6.1-Sol" for a `gpt-6.1-sol` ID. Update test "122. Codex design run" in `codex.e2e.ts` (line 217 asserts a tier word in the Design header, which story 15 forbids): no tier assertion in the header, the joined name instead.
43. Alias: unit table row: `sonnet` reads "Claude Sonnet".
44. Unknown model: unit table row: a custom ID with a harness reads "<Harness> <id>"; an empty ID reads "<Harness> CLI default".
45. 1M variant: unit table row `claude-opus-5[1m]` reads "Claude Opus 5 (1M)".
46. `no model` passthrough: unit table row (no harness known, so returned as is); flow 3 (Pause reasons and recovery) model-mismatch case with a "no model" ran value.
47. One path: `tsc` rejects a raw model ID where a `ModelLabel` is expected (a type test in the `modelName` test file). New unit test: `HarnessLabel` takes a `ModelLabel` only.

New flows and fixtures to write:

- Flows: "Header shape", "Menu button states", "Copy ID" (Design and Epic), "Epic menu", "Epic description popover". Add them to `webui/e2e/CATALOG.md` and the matching `.e2e.ts` files.
- Fixture: only an empty-tier-model step in a `design-model-tiers` Design session. The switched row uses the existing switched `dotfiles` session of `default-harness`, and the full-ID check uses its `claude-haiku-5-5`.
- Existing flows to update, by number and title: 3 (Pause reasons and recovery), 20 (Jump menu opens and closes), 72 (Design actions menu), 75 (Read a session), 84 (Session switcher), 95 (New Agents tab session tiers), 96 (Session model label), 97 (Session model label follows the handoff), 98 (Session model label on an interactive session), 117 (Default harness setting), 2 (Epic looks right), 118 (Codex queue rows), 122 (Codex design run).

### Mock states

Every state is in `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`. Fixture scenarios use the existing names.

- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `design-desktop`, width 960, fixture scenario `design-model-tiers` (the worker-tier session at a later step, started), parts: ring, title, step, menu-button, bar, agent.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `epic-desktop`, width 960, fixture scenario `design` joined as in flow 18 (the `design-epic-impl` page, with a summary set via `gx tickets section`), parts: ring, title, description, menu-button, bar, stats, open-design, queue.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `design-phone`, width 390, fixture scenario `design-model-tiers` (same session), parts: ring, title, step, menu-button, bar, agent.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `epic-phone`, width 390, fixture scenario `design` joined as in flow 18, parts: ring, title, description, menu-button, bar, stats, open-design, queue.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `design-menu-phone`, width 390, fixture scenario `design-two-live` (menu open as a bottom sheet), parts: ring, title, step, menu-button, bar, agent, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `epic-menu-phone`, width 390, fixture scenario `design` joined as in flow 18 (menu open as a bottom sheet; only Copy epic ID ships, the queue-and-epics rows in the mock do not), parts: ring, title, description, menu-button, bar, stats, open-design, queue, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `epic-menu-desktop`, width 960, fixture scenario `design` joined as in flow 18 (menu open as a popover; same note on the pause rows), parts: ring, title, description, menu-button, bar, stats, open-design, queue, menu.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `epic-description-popover-desktop`, width 960, fixture scenario `design` joined as in flow 18 with a long summary, parts: ring, title, description, menu-button, bar, stats, open-design, queue, description-popover.
- `/Users/david/.local/share/gx/tickets/gx/better-epic-header/mocks/01-header-mocks.html`, state `menu-button-states`, width 480, fixture scenario `design-model-tiers` (a Design header; hover, focus and open are driven by the new "Menu button states" flow; the strip is the four states rest, hover, focus and open), parts: none (a strip of four buttons).

## Out of Scope

- Any server change: no `model_name` field, no model catalog read, no probe, no fixture catalog support.
- Epic pause behaviour, its server state and gates, Remove from queue and the Auto-merge toggle behaviour: owned by `gx:queue-and-epics`. This epic only builds the shared menu those items fill.
- The pause rows the mock shows under "Added by queue-and-epics": they are mock-only here.
- A pause on Designs: the server never schedules one.
- The terminal UI: these headers and names exist only on the web.
- Redesigning the Design page body (chat, progress column) or the Epic ticket cards beyond their model label.
- Official model display names from a harness (a later change may add them).

## Further Notes

- Research for the naming rule: closed ticket `gx:better-epic-header/02`.
- The web rule can disagree with a harness's own display name for models it does not know. Unknown IDs show as they are.
- Map: `gx:better-epic-header`. Tickets: 01 (mocks), 02 (name research), 04 and 05 (spec rounds). Ticket 03 (server field) is superseded.
