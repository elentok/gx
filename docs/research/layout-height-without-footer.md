# Layout height without the footer (gx:title-bar)

What changes for each page when the bottom tab bar goes away and a 2-row top chrome (title row +
blank row) takes its place. All sources are local files at `main` (d9118adf).

## How the shell works today

- The app passes the **full** terminal height to every page. `childWindowSizeMsg` only clamps to
  1 (`ui/app/model.go:292-298`). The resize path is `ui/app/model_update.go:109-113`, and
  `resizeCurrentCmd` (`ui/app/model.go:300-309`) uses the same message.
- So each page reserves its own bottom row. Most pages do `m.height - 1` and render a footer
  string or a blank row there.
- `View()` pads/trims the page to `m.width × m.height` (`normalizeFrameContent`,
  `ui/app/model.go:194-226`), then `injectTabsIntoFooter` rewrites the **last line**: tabs on the
  left, the page's last line (left-trimmed, tail-truncated) right-aligned (`ui/app/model.go:146`,
  `ui/app/model_tabs.go:162-200`).
- Tab clicks: `tabHitAt` hits only `y == m.height-1` (`ui/app/model_tabs.go:359-376`). It is called
  before page routing (`ui/app/model_update.go:38-45`).
- Mouse events reach pages **untranslated**. Pages hit-test in screen coordinates, which are
  page-local only because every page starts at row 0.
- `child.Cursor` is passed through unchanged (`ui/app/model.go:167`). Only Agents sets it
  (`ui/agents/model.go:335`).
- Notifications: `ui.OverlayTopRightMargin(content, stack, m.width, 1, 1)`, so they sit at y=1
  (`ui/app/model.go:160-162`).
- App-level overlays use the full screen. The chord hints go bottom-right (`ui/app/model.go:147-156`).
  Notify history is centered (`ui/app/model.go:157-159`).
- Every tab is built in `buildHistoryEntry` (`ui/app/model.go:238-290`). Every CLI entry point
  (`cmd/ui.go:60-290`) goes through `app.New`, so no page runs without the shell.

## Pages

Let `H` = terminal rows. New budget = what the page gets once the app sends `H-2`, after its own
`-1` for the footer is removed.

| Page | Current height budget & where computed | Footer row today | New budget | Extra work beyond the height swap |
|---|---|---|---|---|
| worktrees | `contentHeight() = m.height - helpLineCount()` (=1), min 4 (`ui/worktrees/model_layout.go:67-77`); used by `resized`, `normalView`, `tableRect`/`previewRect` (`model_layout.go:52-65,79-104`, `view.go:133-155`) | `statusBarView()`: `? help`, **spinner + label** while busy, **paste-mode hints** (`ui/worktrees/view.go:167-194`) | `H-2` (net −1) | Spinner/paste-mode status needs a new home (title row or a notification). Set `helpLineCount` to 0. Modals (`m.height/2-6`, `InputModalBottom.ResolveY(m.height…)`, `view.go:26-66`) and wheel/confirm mouse (`model_mouse.go:35`, `model_confirm_modal.go:54`) are page-local: fine once the app translates mouse Y. |
| log | Split gets `m.height` (`ui/log/model_split.go:48`). List panel renders `m.height-1` (`ui/log/list_panel.go:172`), rows `m.height-3` (`list_panel.go:153`). Embedded commit detail: `layoutHeights` `m.height-1` (`ui/commit/view.go:276-279`) | List: blank row. Detail: `? help` (`ui/commit/view.go:39-40,440-456`) | `H-2` (net −1) | Drop the `-1` in the list panel and commit detail. **Kitty image-diff origin**: `DetailOrigin` is split-relative and "a container at a non-zero screen offset must add that offset" (`ui/splitview/model.go:291-317`). Log pushes it as-is (`ui/log/model_split.go:93-98`), so images would land 2 rows high. Commit-info popup `m.height-8` (`ui/log/model_commitinfo.go:87-99`) is page-relative: fine. |
| status | `mainContentHeight() = m.height-1`, min 4 (`ui/status/view_panes.go:75-81`). The same `m.height - 1` is copied at `diff_view.go:161`, `model_render_mode.go:11`, `image_diff.go:71`, `model_mouse.go:38,57` | `helpLine()`: focus section, diff nav mode / render mode / wrap, split-app (`terminalLabel`), **`VISUAL` prefix**, `? help` (`ui/status/view.go:42-43`, `view_footer_help.go:13-39,95-97`, `model_help.go:9-14`) | `H-2` (net −1) | Fold the 6 copies into `mainContentHeight()`. Move or remove the footer indicators; `VISUAL` is the only mode cue. **Kitty `PanelGeometry`** assumes "Status owns the whole screen … origin (0,0)" (`ui/status/image_diff.go:65-90`): needs +2 rows. Wheel hit-tests use `filetreeRect`/`diffRect` at y=0 (`model_mouse.go:37-90`): fine once mouse is translated. Modals use `OverlayCenter(…, m.height)` (`view.go:44-60`): fine. |
| stash | Split gets `m.height` (`ui/stashlist/model.go:104-110`). List panel renders `m.height-1` (`ui/stashlist/view.go:23`), rows `m.height-3` (`list_panel.go:96`). Commit detail `m.height-1` as in log | `stashFooter()` `? help`, joined under the body (`ui/stashlist/model.go:283,295,302-304`). In split mode the body is already `H` rows (commit detail is full height), so the footer is the `H+1`th row and `normalizeFrameContent` drops it; the commit's `? help` shows instead | `H-2` (net −1) | Remove `stashFooter` and the `-1`s. **Kitty origin** +2, same as log (`ui/stashlist/model.go:226-236`). Wheel goes through `splitview.RouteWheel` (`ui/splitview/wheel_route.go:23-42`): page-local, fine. |
| prs | `panelHeight = m.height-1` in `buildMainContent` and `viewportH` (`ui/prs/model.go:163-182`) | `prsFooter()` `? help` (`ui/prs/model.go:172,238-240`) | `H-2` (net −1) | None. The wheel scroll does not depend on position (`ui/prs/model_mouse.go:13-30`). The comments popup and overlays are page-relative. |
| tickets | `contentHeight() = m.height-1`, min 4 (`ui/tickets/model.go:596-602`) | `footerView()` `? help`. Its comment says it exists "so the app shell's tab bar has a dedicated row" (`ui/tickets/model.go:507-516`) | `H-2` (net −1) | Remove `footerView`. Click/wheel use page-local Y (`model.go:370-392` `bodyLine = mouse.Y-1`; `preview_focus.go:142-157`): fine once mouse is translated. Watch popup `height-10` (`watch_agent.go:77`) and `ResolveY(m.height…)` (`model.go:469`) are page-relative. |
| queue | `contentHeight() = max(m.height-1, 1)`. Its comment says "below the tab bar" (`ui/tickets/queue.go:297-302`) | Blank row (no footer string, `ui/tickets/queue_view.go:17-70`) | `H-2` (net −1) | Remove the `-1`. Click `bodyLine = mouse.Y-1-queueHeaderReservedLines` (`queue.go:322`) is page-local: fine. |
| agents | No `-1`. Sidebar and main panels render the full `m.height` (`ui/agents/model.go:386,391`; `sidebar.go:157`). PTY rows `m.height-2` (`ui/agents/terminal.go:197-203`; new session `create.go:444`). Sidebar window `bodyH = m.height-1` (`sidebar.go:97-112`) | None of its own. The tab bar is drawn **over the panels' last body row**. The terminal leaves that row unused (`-2` = header + that row). The sidebar's last row is hidden under the tabs, and so is the cursor when it sits there | Panels `H-2`; PTY rows should be `(H-2)-1` = `H-3` (net −1) | Change `screenSize` and `create.go:444` from `m.height-2` to `m.height-1`, or the PTY loses one more row and keeps a dead row. `bodyH` stays `m.height-1` and is correct then. PTY resize follows by itself: `Update` calls `syncAttachSize` after every message (`model.go:163-168`, `attach.go:330-345`). **Cursor** `tea.NewCursor(x+…, y+…)` (`terminal.go:154-166`) is page-local: the app must add +2 to `child.Cursor.Y`. Mouse `screenCell`/`overPanel`/`sidebarSessionAt` (`terminal.go:179-185`, `attach.go:321-324`, `sidebar.go:132-142`) are page-local. Jump/create popups (`jump.go:98`, `create.go:343`) are page-relative. |

Pages pushed with `nav.Open` use the same `buildHistoryEntry`, so the list above is complete. The
commit page exists only embedded in log/stash (`ui/log/model.go:126`, `ui/stashlist/model.go:36`).
`ui/history` is a separate program (`ui/history/run.go:16`) and is not affected.

## App-level work (shared by all pages)

1. **Height**: `childWindowSizeMsg` sends `max(1, m.height-2)` (`ui/app/model.go:292-298`).
   `View()` normalizes the child to `H-2`, then prepends the title row and blank row instead of
   calling `injectTabsIntoFooter` (`model.go:145-146`).
2. **Mouse translation**: every `tea.MouseMsg` (click, release, motion, wheel) must get `Y -= 2`
   before page routing (`ui/app/model_update.go:38-45` and the fall-through at `:123-130`). Events
   with `Y < 2` go to the title row: view switcher / attention items / `? help`. This replaces
   `tabHitAt`. Without this step, every page's hit-test is off by 2. Nothing else needs to change,
   because all page hit-tests are page-local.
3. **Cursor**: offset `child.Cursor.Y` by +2 (`model.go:167`). Only Agents uses it.
4. **Kitty image placements**: they are absolute screen cells written by `imagediff.Overlay.place`
   (`ui/imagediff/overlay.go:244-275`). Hosts: status `PanelGeometry`, plus commit via
   `WithScreenOrigin` from log/stash. Either add +2 in each host, or give the app/`imagediff` a
   single top-offset. The second option is less likely to drift.
5. **Notifications**: `OverlayTopRightMargin(…, 1, 1)` → `marginY = 2`, below the blank row
   (`model.go:160-162`).
6. **Overlays**: chord hints (bottom-right) and notify history (centered) are drawn on the full
   screen. They can stay as they are; there is no footer to avoid any more.
7. **Tests**: `ui/app/model_mouse_test.go`, `model_test.go`, `server_conn_test.go`,
   `server_stream_test.go` use `tabsView`/`tabHitAt`/`injectTabsIntoFooter`. Page tests that
   assert `height-1` geometry will need updating too.

## Answer

- The app sends every page the full height (`ui/app/model.go:292-298`). Each page reserves its own
  bottom row, and the app paints the tab bar over that row (`injectTabsIntoFooter`,
  `model_tabs.go:162`).
- New contract: the app sends `H-2` and draws title + blank row above the page. Every page drops its
  `-1`. **Net change: −1 content row on every page** (worktrees, log, status, stash, prs, tickets,
  queue, agents).
- Height swap only: prs, tickets, queue. Remove their `-1` and their `? help` footer
  (`prs/model.go:164,180,238`; `tickets/model.go:597,514`; `tickets/queue.go:301`).
- Footer content to move or drop: status (`VISUAL`, focus/nav/render/wrap, split-app label,
  `view_footer_help.go:13-39`), worktrees (spinner + paste-mode hints, `worktrees/view.go:167-194`).
  Stash/commit footers are only `? help`.
- Agents is not a plain swap. Its panels already use the full height, and the tab bar hides their
  last row. Change PTY rows from `m.height-2` to `m.height-1` (`agents/terminal.go:198`,
  `create.go:444`). PTY resize then follows by itself through `syncAttachSize`.
- Mouse: pages hit-test in screen coordinates, which equal page-local only because pages start at
  row 0. The app must subtract 2 from every mouse event's Y and handle `Y<2` itself (this replaces
  `tabHitAt`). After that, no page's hit-test code changes.
- Cursor: the app adds +2 to `child.Cursor.Y` (Agents terminal cursor, `agents/terminal.go:165`).
- Kitty image diffs use absolute screen rows. Add +2 in status `PanelGeometry`
  (`status/image_diff.go:70`) and in the log/stash `WithScreenOrigin` origins (`log/model_split.go:94`,
  `stashlist/model.go:231`), or as one shared top-offset.
- Notifications move from `marginY 1` to `2` (`app/model.go:161`). Page modals and app overlays are
  page-relative or full-screen and need no change.
- Cleanup worth doing in the same pass: status repeats `m.height - 1` in 6 places. Fold them into
  `mainContentHeight()`.
