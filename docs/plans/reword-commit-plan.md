# Reword Commit Plan

## Decisions

- Binding: chord `cr` in both log and commit views (under existing `c` prefix in commit; new `c` prefix in log)
- No confirm dialog — go straight to `$EDITOR`
- Editor opened via `tea.ExecProcess` (not `terminalrun.Command`) so we block until editor closes
- Full commit message (subject + body) pre-populated in temp file
- Pushed warning added as `# warning: ...` comment in temp file
- No-change detection: strip comment lines from saved file; if result equals original (or is empty/whitespace), abort with "no changes" status
- Git HEAD case: `git commit --amend -F <tmpfile>`
- Git non-HEAD case: `git rebase -i <hash>^` with injected `GIT_SEQUENCE_EDITOR` (pick→reword) and `GIT_EDITOR` (copy our file)
- Non-HEAD also needs conditional stash/pop (same pattern as amend) for unstaged working-tree changes
- Rebase conflict recovery: same as amend — show "reword failed", user resolves manually

## Tasks

### Git layer

- [x] Create `git/reword.go`:
  - `RewordHead(root, message string) (string, error)` — writes message to temp file, runs `git commit --amend -F <tmpfile>`, cleans up
  - `RewordCommit(root, hash, message string) (string, error)` — writes message + two temp scripts (sequence editor, message editor), runs `git rebase -i <hash>^ ` with injected env, cleans up all temp files

### UI reword package

- [x] Create `ui/reword/reword.go`:
  - `CmdOpenEditor(root, hash, subject, body string, pushed bool) (tea.Cmd, string, error)` — writes temp file (message + optional `# warning:` comment), returns `tea.ExecProcess` cmd + temp file path
  - `ReadResult(tmpFile, original string) (changed bool, newMsg string, err error)` — reads file, strips comment lines, compares to original; returns changed=false if result is empty/whitespace
  - `Model` struct — owns only the running phase (spinner + steps), `IsOpen bool`
  - `(m *Model) StartRunning(root, hash, newMsg string) (tea.Cmd, error)` — builds steps (HEAD vs non-HEAD, with stash dance), sets `IsOpen = true`, returns first cmd
  - `(m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result)` — handles spinner + step messages
  - `(m Model) View(width int) string` — renders running modal (spinner + steps, same style as amend)

### Log view

- [x] `ui/log/model.go` — add `reword reword.Model`, `rewordTmpFile string`, `rewordOrigMsg string`
- [x] `ui/log/model_keys.go` — add `bindingReword = "reword"`, chord `{Seq: []string{"c", "r"}, Title: "reword commit"}`, cancel `{Seq: []string{"c", "esc"}}`, dispatch to `m.cmdFetchRewordDetails()`
- [x] Create `ui/log/model_reword.go`:
  - `cmdFetchRewordDetails()` — async cmd to call `git.CommitDetailsForRef` + `git.IsCommitPushed`
  - `handleRewordDetails(msg)` — calls `reword.CmdOpenEditor`, stores tmp file + original
  - `handleRewordEditorDone(err)` — calls `reword.ReadResult`, calls `m.reword.StartRunning` if changed
  - `handleRewordDone(err)` — sets status msg, triggers reload
- [x] `ui/log/model_update.go` — route all messages to `m.reword.Update(msg)` when `m.reword.IsOpen`; add cases for `rewordDetailsMsg` and `rewordEditorFinishedMsg`
- [x] `ui/log/view.go` — add `ui.OverlayCenter` for `m.reword.IsOpen`

### Commit view

- [x] `ui/commit/model.go` — add `reword reword.Model`, `rewordTmpFile string`, `rewordOrigMsg string`
- [x] `ui/commit/model_keys.go` — add `bindingReword = "reword"`, chord `{Seq: []string{"c", "r"}, Title: "reword commit"}` under existing `c` prefix (cancel already exists), dispatch to `m.openRewordEditor()`
- [x] Create `ui/commit/model_reword.go`:
  - `openRewordEditor()` — calls `reword.CmdOpenEditor` using `m.details` (already loaded), stores tmp file + original
  - `handleRewordEditorDone(err)` — calls `reword.ReadResult`, calls `m.reword.StartRunning` if changed
  - `handleRewordDone(err)` — sets status msg, reloads or navigates
- [x] `ui/commit/model_update.go` — route all messages to `m.reword.Update(msg)` when `m.reword.IsOpen`; add case for `rewordEditorFinishedMsg`
- [x] `ui/commit/view.go` — add `ui.OverlayCenter` for `m.reword.IsOpen`
