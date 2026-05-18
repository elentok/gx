# Log Filter

Filter the log view by a file or line range, triggered from the status or commit view.

## Decisions

- **`f` key** in status/commit view pushes a filtered log via `nav.Push` (q returns to origin).
- **Filter granularity**: file-only when focused on the file tree or on a context line; `git log -L start,end:file` when focused on a hunk or changed line.
- **Staged section**: always use unstaged (working-tree) line coordinates for `-L`.
- **`f` key** in the log view toggles the filter off and reloads the full log.
- **Frame indicator**: right title shows `"path/to/file.ts L3-4"` (or just `"path/to/file.ts"`) while filtered, replacing the ref.
- **Graph with `-L`**: `git log -L` doesn't support `--graph`; render `*` for every row.
- **No selection / context line**: fall back to file-only.

## Tasks

- [x] Add `LogFilter` struct (`Path string`, `StartLine int`, `EndLine int`) to `git` and `ui/log`.
- [x] Add filter fields to `nav.Route` (`FilterPath`, `FilterStartLine`, `FilterEndLine`).
- [x] Add `git.LogEntriesFiltered` supporting `-- <file>` and `-L start,end:file`; drop `--graph` when using `-L`.
- [x] Update `ui/log/model.go`: store `filter LogFilter`; add `NewModelFiltered`.
- [x] Update `ui/log/model_data.go`: pass filter to the git call.
- [x] Update `ui/log/model_keys.go`: add `bindingClearFilter` (`f`); toggle filter off and reload.
- [x] Update `ui/log/view.go`: show filter in `RightTitle` via `frameRightTitle()`.
- [x] Update `ui/status/model_keys.go`: add `bindingFilterLog` (`f`); extract file/line range; push filtered log.
- [x] Update `ui/commit/model_keys.go`: same as status.
- [x] Update `ui/app/model.go`: pass filter fields from `nav.Route` into `logui.NewModelFiltered`.
- [x] Move diffarea fullscreen from `f` to `F` to free the key; update affected test.
