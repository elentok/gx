# e2e triage 1 (worktree slow-tests-impl, webui/e2e)

Vitest paths are relative to webui/src. "EPT" = ui/expandable-text. Line numbers are test() lines in the existing vitest file.

| file | flow | title | verdict | reason | vitest file |
|---|---|---|---|---|---|
| vi-editor | 124 | 390px: expand button shows on the message box | TO-VITEST | button render only; each vi-editor flow also pays a 45s design-session start | EPT/ExpandableTextField.test.tsx:64 (existing) |
| vi-editor | 124 | vi on: Cmd-G in the field opens the editor | TO-VITEST | key handler logic | EPT/ExpandableTextField.test.tsx:195 (existing) |
| vi-editor | 124 | vi on: Cmd-G with focus on body opens nothing | TO-VITEST | handler is scoped to the field; fire key on document.body | EPT/ExpandableTextField.test.tsx:195/218 (add a body case) |
| vi-editor | 124 | vi off: Cmd-G opens nothing and is not cancelled | TO-VITEST | defaultPrevented check works in jsdom | EPT/ExpandableTextField.test.tsx:211 (existing) |
| vi-editor | 124 | editor opens in INSERT at the field's caret | TO-VITEST | selection hand-off | EPT/CodeEditor.vim.test.tsx:59 + ExpandableTextField.test.tsx:133 (existing) |
| vi-editor | 124 | ZZ closes the editor and keeps the text | TO-VITEST | already covered | EPT/CodeEditor.vim.test.tsx:101, ExpandableTextField.test.tsx:260 |
| vi-editor | 124 | Cmd-G inside the editor closes it | TO-VITEST | already covered | EPT/ExpandableTextField.test.tsx:226 |
| vi-editor | 124 | backdrop press closes at desktop, not at 390px | TO-VITEST | wide flag is a prop, not an engine measurement | EPT/LargeEditor.test.tsx:73 (existing) |
| vi-editor | 124 | 1280px mouse, vi on/off: Done visible and closes (x2) | TO-VITEST | already covered | EPT/LargeEditor.test.tsx:55,65 |
| vi-editor | 124 | Escape leaves the editor open in insert and normal | TO-VITEST | already covered | EPT/ExpandableTextField.test.tsx:155, LargeEditor.test.tsx:65, CodeEditor.vim.test.tsx:108 |
| vi-editor | 124 | closing with Done / Cmd-G / ZZ / backdrop focuses field at caret (x4) | TO-VITEST | already covered by one test for all four closers | EPT/ExpandableTextField.test.tsx:260 (add Cmd-G row if missing) |
| vi-editor | 124 | typing in the editor updates the field and sent request body | TO-VITEST | field sync is covered; request body needs a mocked fetch on the design composer | EPT/ExpandableTextField.test.tsx:73 + design/Composer.test.tsx (propose: assert POST /v1/sessions/prompt body) |
| vi-editor | 124 | Cmd-Enter in the agent composer's editor closes it and sends | KEEP-E2E | only flow with a real prompt round trip (`gx agents chat` shows the text); uses cheap `agents` scenario, no design session | (UI half: EPT/ExpandableTextField.test.tsx:236) |
| vi-editor | 124 | Cmd-Enter in a question's answer editor does nothing | TO-VITEST | no submit action => editor stays open and no fetch | EPT/ExpandableTextField.test.tsx:250 + design/QuestionSheet.test.tsx (mock fetch, assert 0 calls) |
| vi-editor | 124 | 390px: Cmd-Enter in the composer's editor does nothing | TO-VITEST | same as above | EPT/ExpandableTextField.test.tsx:250 |
| vi-editor | 123 | a touch device reports a coarse pointer | DELETE-DUPLICATE | tests the Playwright `touch` config, not gx; every touch flow would fail anyway if it broke | none |
| vi-editor | 123 | 390px touch, vi off: Done closes; bar has Done only | TO-VITEST | render of bar parts | EPT/LargeEditor.test.tsx:55,65,129 |
| vi-editor | 123 | 1280px touch: bar shows, modal at most 680px high | MERGE-INTO 123 "1280px mouse: 860 by 680 modal" | real height measurement; same layout flow, switch to touch via a second context | (e2e) |
| vi-editor | 123 | 1280px mouse, vi off: Done closes | DELETE-DUPLICATE | same assertion as 124 Done flow | EPT/LargeEditor.test.tsx:65 |
| vi-editor | 123 | 1280px mouse: no touch bar | TO-VITEST | bar is conditional on pointer prop | EPT/LargeEditor.test.tsx:55 |
| vi-editor | 123 | 1280px mouse: 860 by 680 modal with title, footer, Done, no x | KEEP-E2E | real computed modal size; target for the layout flows (also take: mode text left of footer, 1280 touch <=680px, Cmd-K over editor) | (e2e) |
| vi-editor | 123 | 390px mouse, vi on: title, footer, mode text, no bar | TO-VITEST | render by props | EPT/LargeEditor.test.tsx:55,114 |
| vi-editor | 123 | 1280px mouse: mode text left of footer, follows mode | MERGE-INTO 123 "860 by 680 modal" | x-order needs a real engine; mode follow is covered | EPT/LargeEditor.test.tsx:114 for the follow part |
| vi-editor | 123 | 1280px mouse, vi off: no mode text | TO-VITEST | already covered | EPT/LargeEditor.test.tsx:129 |
| vi-editor | 123 | 390px touch, vi on: bar mode text, Esc, Done; 44px | TO-VITEST | 44px is a style/size token; Esc behavior covered | EPT/LargeEditor.test.tsx:81,136 |
| vi-editor | 123 | 1280px touch, vi on: title, bar, no footer, one Done | TO-VITEST | render by props | EPT/LargeEditor.test.tsx:55 |
| vi-editor | 125 | mode text follows INSERT/NORMAL/VISUAL | TO-VITEST | covered | EPT/CodeEditor.vim.test.tsx:47 |
| vi-editor | 125 | h j k l w e 0 f gg G move the caret | TO-VITEST | vim motions in CodeMirror state; assert view.state.selection | EPT/CodeEditor.vim.test.tsx (propose; same harness as :83). Caveat: jsdom has no layout, so keep j/k only on one-line-per-row text |
| vi-editor | 125 | 390px: j moves one screen line in a wrapped paragraph | KEEP-E2E | needs real line wrapping (posAtCoords) | (e2e) |
| vi-editor | 125 | a click resets the column that j and k keep | MERGE-INTO 125 "390px: j moves one screen line" | needs real coordinates; same wrapped text | (e2e) |
| vi-editor | 125 | x dw yy p u Ctrl-R iXY . and v d edit the text | TO-VITEST | covered | EPT/CodeEditor.vim.test.tsx:83 (extend with . and v d) |
| vi-editor | 125 | i a o and . change text and mode | TO-VITEST | pure vim state | EPT/CodeEditor.vim.test.tsx (propose) |
| vi-editor | 125 | 390px touch: gs and ge move to line start/end | TO-VITEST | custom keymap, not touch specific | EPT/CodeEditor.vim.test.tsx (propose). Caveat: if gs/ge use visual line (wrapped) keep one case in the j-wrapped e2e flow |
| vi-editor | 126 | Escape in editor over question sheet closes neither | TO-VITEST | Escape routing between two components; layered Radix/Chakra dialogs behave the same in jsdom. If a regression shows up there, move it to the focus flow | design/QuestionSheet.test.tsx (propose: render sheet + editor, fire Escape, assert both present). Slight risk, see notes |
| vi-editor | 126 | backdrop press closes only the editor over start-over dialog | MERGE-INTO 126 "Escape closes the start-over dialog..." | same `paused` scenario and same two dialogs | (e2e) |
| vi-editor | 126 | Cmd-K does not open Jump over the editor | MERGE-INTO 123 "860 by 680 modal" | same design-working + openEditor setup; global key listener vs modal | (e2e), or nav/JumpMenu test with an open dialog |
| vi-editor | 126 | Tab and Shift-Tab never leave the editor; no tab char | KEEP-E2E | real focus order and CodeMirror tab handling; target for the Tab flows | EPT/ExpandableTextField.test.tsx:111 covers the no-tab-char part |
| vi-editor | 126 | touch: Tab cycles editor, Esc, Done | MERGE-INTO 126 "Tab and Shift-Tab never leave the editor" | same dialog, touch variant via a second context or just run in touch (touch viewport already has the bar) | (e2e) |
| vi-editor | 126 | touch: Shift-Tab wraps to Done; Tab returns | MERGE-INTO 126 "Tab and Shift-Tab never leave the editor" | same trap, two more key presses | (e2e) |
| vi-editor | 126 | editor is a named dialog with a named editable area | DELETE-DUPLICATE | every other flow already finds the dialog by that name; also checked in jsdom | EPT/ExpandableTextField.test.tsx:64 |
| vi-editor | 126 | Tab never leaves a plain modal | MERGE-INTO 126 "Escape closes the start-over dialog..." | control for the same focus-trap library; the start-over dialog flow asserts the same on a plain modal | (e2e) |
| vi-editor | 126 | Escape closes start-over dialog, Tab stays in it, editor over it keeps both open | KEEP-E2E | stacked-layer Escape/focus in a real browser; target for the start-over and plain-modal flows | (e2e) |
| vi-editor | 127 | menu has a Vi mode switch that turns on | MERGE-INTO 127 "flipping the setting with the editor open" | UI part is covered; the wire part is the same as the flip flow | settings/ConnectedTopBar tests, vimode.test.ts:11 |
| vi-editor | 127 | a second page of the same login shows the switch on | MERGE-INTO 127 "flipping the setting with the editor open" | add second page as one more step | (e2e) |
| vi-editor | 127 | flipping the setting with the editor open shows/hides vi at once | KEEP-E2E | server push of a setting into an open editor (CLI -> SSE -> live CodeMirror reconfigure) | (e2e) |
| checks | 64 | Check config banner | TO-VITEST | banner text and buttons from mocked status; the wide-row bbox check is minor (drop or keep in 70) | epics/EpicsPageChecks.test.tsx:86 (existing) |
| checks | 65 | Check config banner, all projects | TO-VITEST | covered | epics/EpicsPageChecks.test.tsx:99 |
| checks | 66 | Not now | TO-VITEST | covered, including "no write" | epics/EpicsPageChecks.test.tsx:145 |
| checks | 67 | Set up checks, save to repo | KEEP-E2E | real commit of .gx/checks.json on main and `server checks` readback; dialog contents are covered by SetupDialog.test.tsx:54-129 so slim the UI asserts | checks/SetupDialog.test.tsx |
| checks | 68 | Set up checks, save to project.json | MERGE-INTO 67 | use project `auto` (the checks-missing scenario has both) in the same server run | checks/SetupDialog.test.tsx:137 |
| checks | 68 | Set up checks, keep gx defaults | TO-VITEST | the dialog sends {} (covered); server write path is the same as project.json | checks/SetupDialog.test.tsx:121 |
| checks | 69 | Set up checks, refused commit | MERGE-INTO 67 | do it first (write the draft file, Save refused, dialog stays, HEAD unchanged), then remove the file and run the real save; one server start | checks/SetupDialog.test.tsx:144,153 cover UI |
| checks | 70 | Phone, Set up checks | MERGE-INTO 67 | real layout at 400px; resize at the start of the flow, before saving | (e2e) |
| disk | 76 | Disk banner and Clean now | KEEP-E2E | real disk polling, banner persisting across routes, click-through to server clean; target for the disk flows | ui/disk-strip/DiskStrip.test.tsx:9,23 cover rendering |
| disk | 77 | Disk hold | KEEP-E2E | real dispatch hold, `tickets explain` CLI, ticket verdict changes; needs `hold` option so it cannot share with 76 | ui/disk-strip/DiskStrip.test.tsx:17 covers the strip text |
| disk | 78 | Clean now fails | MERGE-INTO 76 | set clean to fail and click before the recovery steps; same disk-low scenario | ui/disk-strip/DiskStrip.test.tsx:35 |
| dirty-trunk | 58 | Dirty trunk on the Epic page | KEEP-E2E | real git dirty-trunk detection and pause; target for 60. The bbox-order check could go to epic/EpicPage.test.tsx but jsdom has no layout | (e2e) |
| dirty-trunk | 59 | Stash, merge and restore | KEEP-E2E | real stash/merge/restore via git; needs `hold` | (e2e) |
| dirty-trunk | 60 | Merge again on a dirty trunk | MERGE-INTO 58 | click Merge again right after the notice asserts; same scenario and starting state, run before 59's state change | (e2e) |
| modes | 61 | Epic page gets the wide layout and a popover jump menu (760px) | KEEP-E2E | real breakpoint layout; target for card columns | (e2e) |
| modes | 61 | Design page gets the phone layout (760px) | KEEP-E2E | real full-screen sheet size; different scenario (design) so it cannot share. If the mixed scenario can host a design page, merge into the first 61 flow | (e2e) |
| modes | 62 | Card columns | MERGE-INTO 61 "Epic page gets the wide layout" | same `mixed` scenario, both are layout measurements; resize to desktop for this one | (e2e) |
| stats | 57 | Stats, Grilling section | TO-VITEST | section order, totals, hatched days, range/project filters are all data -> render | stats/StatsPage.test.tsx:188,200,251,261 (existing) |
| stats | 182 | Stats limit burn | TO-VITEST | cards, partial, hatched, tooltip reset marks, project tag, range switches | stats/LimitBurnCards.test.tsx:58-131 and StatsPage.test.tsx:251,261 (add range 7/90 cases if absent). Server-side shaping of the `stats` numbers belongs in a Go test |
| stats | 183 | Stats tokens and spend by group | TO-VITEST | legend order, rebuilt/stamp-only dim bars, project filter | stats/StatsPage.test.tsx:123,140,160 (existing); add legend order and quiet-day tooltip with mock stats |
| stats | 184 | Live token ledger | KEEP-E2E | real transcript polling -> ledger -> page; also host the phone layout check from 182 phone | (e2e) |
| stats | 182 | Stats limit burn, phone | MERGE-INTO 184 | real layout (card stacking, no horizontal overflow); resize at the end of 184, which already shows the stats page with limit cards | (e2e) |
| stats | 57 | Stats, Grilling section, phone | KEEP-E2E | real layout, tiny; only keep if grilling cards are not reachable from 184's scenario, otherwise add grilling data there and merge | (e2e) |
| titles | 83 | Tab titles | TO-VITEST | document.title per route | routes.test.tsx:109-146 (Ticket/Design) and page/titleCount.test.tsx:43; add Epics, Stats and Epic cases there |
| long-running | 148 | Long-running badge | TO-VITEST | badge from ticket data (turn count/threshold in title); absence on plain ticket | page/LongRunningBadge.test.tsx:5,10 (existing) + ticket/TicketScreen.test.tsx and epic/EpicPage.test.tsx for "badge on epic row and ticket page". The server marking belongs to a Go test |
| check-lock | 185 | Check-lock waiting label | KEEP-E2E | only flow showing the label appearing and clearing when the real lock is released; rendering is already covered | ticket/TicketScreen.test.tsx:47 |
| health | 79 | Tray states | TO-VITEST | 2 widths x 3 pages x 4 steps (~24 server pokes) just to check meter state and fill height from data | health/HealthPopover.test.tsx:342-386 (existing tray tests); propose health/HealthTray.test.tsx: mock /v1/health + snapshot states for cpu/mem levels 0/3/4/unknown, assert data-state and fill style. One real-wire check stays in flow 81 |
| health | 80 | Held disk in the popover | TO-VITEST | critical state + "Dispatch held"/"Clean now" text from data | health/HealthPopover.test.tsx:254,293 (existing) |
| health | 81 | Tray popover | KEEP-E2E | real agent row tied to a fake agent, live "Updated Ns ago" ticking, live value change, real 390px popover bounds [42,388]; target for tray flows. Trim the text asserts already covered | health/HealthPopover.test.tsx:93,108,127,145,223,323 |
| health | 82 | Tray offline | MERGE-INTO 81 | stopServer as the last step of 81; same `health` scenario | health/HealthPopover.test.tsx:342,355 (existing) |

## Summary

Rows (a row can cover 2 to 4 playwright tests): KEEP-E2E 17, TO-VITEST 37, MERGE-INTO 18, DELETE-DUPLICATE 3 (75 rows, ~100 tests incl. loops).
After the plan roughly 17 e2e flows remain from about 75 (a ~75% cut in fixture starts for these files).

5 largest wins:
1. vi-editor.e2e.ts: ~45 flows, each with a fixture + 45s design session, down to ~7 flows (composer send, 860x680 layout, wrapped-line j, Tab trap, start-over layers, live setting flip). Most are already in vitest under ui/expandable-text.
2. health 79 "Tray states": ~24 server pokes in one flow, becomes a mocked-health vitest test; 80 also goes; 82 folds into 81.
3. stats 57/182/183: three data-rendering flows already covered by StatsPage.test.tsx and LimitBurnCards.test.tsx; phone check folds into 184.
4. checks 64-66, 68(defaults): covered by EpicsPageChecks.test.tsx and SetupDialog.test.tsx; 67/68/69/70 collapse to one flow.
5. disk 78, dirty-trunk 60, modes 62: three more fixture starts removed by merging into 76, 58, 61.

Notes and risks:
- vi-editor "Escape over question sheet" and motions tests assume jsdom layering and CodeMirror state work like the browser; if the first jsdom attempt is flaky, move it into the Tab/layers flow.
- Server-side values (stats shaping, long-running marking, waiting label) should have Go tests if they do not already.
