# Log Flash & Focus Plan

## Decisions

- Flash: steady tint for 5 seconds, then disappears on a single tick
- Flash color: `#3d2810` (dark amber), defined as `logFlashBg` const — easy to adjust
- Flash duration: `logFlashDuration = 5 * time.Second` const
- Flash overrides selection highlight (amber background visible even on selected row)
- Flash triggers on successful amend or reword completion (not on normal back/escape navigation)
- Focus (cursor moved to changed commit) always accompanies flash
- Navigation mechanism: `FocusSubject string` added to `nav.Route`; log model holds `pendingFocusSubject` consumed by `OnPageActivated`

## Tasks

### Shared rendering helper

- [x] `ui/styles.go` — add `RenderRowWithBackground(text string, bg color.Color) string`
  (parameterized version of `RenderRowHighlight`; re-applies bg after ANSI resets)

### Nav route extension

- [x] `ui/nav/nav.go` — add `FocusSubject string` to `Route` struct

### Log model flash + focus

- [x] `ui/log/model.go` — add fields `pendingFocusSubject string`, `flashSubject string`, `flashUntil time.Time`
- [x] `ui/log/model.go` — add `WithPendingFocus(subject string) Model` method
- [x] `ui/log/model.go` — update `OnPageActivated`: use `cmdReloadFocusSubject(pendingFocusSubject)` when set, else `cmdReload()`
- [x] `ui/log/model_update.go` — `handleReload`: clear `pendingFocusSubject`; when `focusSubject != ""` set flash fields and return `cmdFlashClear()`; add `flashClearMsg` case to `Update`
- [x] `ui/log/view.go` — add `logFlashBg` and `logFlashDuration` consts; update `renderRow` to apply flash background (overrides selection)

### App shell wiring

- [x] `ui/app/model_tabs.go` — in `switchTab`, when `route.FocusSubject != ""`, call `logModel.WithPendingFocus(subject)` on current tab model before `onPageActivatedCmd`

### Commit view: pass focus subject on navigation

- [x] `ui/commit/model_amend.go` — add `FocusSubject: m.details.Subject` to `nav.Replace` call
- [x] `ui/commit/model_reword.go` — add `FocusSubject: m.rewordNewSubject` to `nav.Replace` call
