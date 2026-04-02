# gx

A collection of git helper (worktree management, etc...)

## Disclaimer

I wrote the original version of the tool in Typescript a while ago but at some
point I realized I wanted something a bit different and had Claude Code migrate it
to Go with a lot of UI changes (see [convert-to-go.md](./docs/prompts/convert-to-go.md)
and [go-migration-plan.md](/docs/go-migration-plan.md)).

## Features

- Browse all linked worktrees in a table with sync status (ahead / behind / diverged) and rebase status relative to main
- Sidebar showing the latest commit (with relative date), commits ahead/behind the remote tracking branch, rebase status relative to main, and uncommitted file changes
- Create, rename, clone, and delete worktrees interactively (optionally opening a new tmux session or window)
- Yank files from one worktree and paste them into another
- Pull, push, and remote-update the selected worktree's branch; pull on a dirty worktree offers to stash first
- Rebase the selected worktree on main (`b`), with optional stash-and-restore for dirty worktrees
- `gx wt clone` clones using the `.bare` directory trick for a clean layout
- `gx wt list` and `gx wt abs-path` for scripting and shell integration
- `gx status` interactive status UI with file/hunk/line stage + unstage flows
- Press `/` to search and highlight matching worktrees by name or branch
- Press `l` to open the selected worktree in lazygit
- `gx bump` creates an annotated version tag with an interactive picker (or pass `major`/`minor`/`patch` directly) and optionally pushes
- `gx doctor` checks for and optionally fixes common configuration issues
- Startup check for misconfigured fetch refspec with an option to fix automatically
- Scrollable error modal for any git failures
- See [Changelog](./CHANGELOG.md)

![screenshot](./docs/screenshot.png)

## Requirements

- Go 1.21+
- Git
- tmux (optional, for `N` and `T` keybindings)

## Installation

Using homebrew:

```sh
brew tap elentok/stuff
brew install --cask gx
```

Using `go install`:

```sh
go install github.com/elentok/gx@latest
```

```sh
make install
```

## Usage

Run from inside any git repository or bare repo:

```sh
gx
```

If launched from inside a worktree, the cursor starts on that worktree.

You can also run the TUI explicitly:

```sh
gx worktrees
gx wt
```

Open the interactive staging UI:

```sh
gx status
```

Status UI highlights:

- Status tree + split Unstaged/Staged diff panes
- Stage or unstage at file, hunk, or line level
- Visual line-range mode (`v`) to stage/unstage selected blocks with `space`
- Discard changes with confirmation (`d`) in status and diff views
- Yank content/location/all/filename with `yy` / `yl` / `ya` / `yf`
- Status header shows branch sync at a glance (`✓`, `↑N`, `↓N`, `↑N ↓N`)
- Live search in status/diff with highlights and `n` / `N` navigation
- Vim-like navigation (`j`/`k`, `gg`/`G`, `ctrl+u`/`ctrl+d`)
- Mouse wheel scrolling in diff panes (unstaged/staged, including fullscreen)
- Toggle unified/side-by-side diff rendering with `s` (supports hunk, line, and visual actions)
- File-to-file diff jumps with `,` / `.`
- Edit selected file in `$EDITOR` with `e` (opens at selected line/hunk in diff view)
- Open lazygit log with `ol`
- Pull/push/rebase/amend actions directly in status (`p`/`P`/`b`/`A`) with confirmations
- Push divergence flow uses a menu (`j`/`k` + `enter`) with relative commit times
- Push in status detects GitHub PR URLs and asks whether to open them
- Keyboard help overlay (`?`) and full git-error overlay
- Live action output overlay with cancellation (`ctrl+c`)
- Fullscreen diff hides the status pane
- Focus refresh keeps your diff scroll position

Clone using the `.bare` directory trick and bootstrap the initial worktree:

```sh
gx wt clone <repo-url> [directory]
```

This creates:

```
my-repo/
  .bare/      ← bare git repo
  .git         ← gitdir: ./.bare
  main/        ← initial worktree
```

List worktree names or get the absolute path of one (useful for scripting):

```sh
gx wt list
gx wt abs-path <name>
```

Push current worktree branch, with proactive divergence detection and visible preflight progress:

```sh
gx push
```

Stash uncommitted changes, run a command, then auto-pop the stash on success (prompts to pop on failure):

```sh
gx stashify git rebase main
```

Create an initial config file with defaults:

```sh
gx init
```

Edit config in `$EDITOR`:

```sh
gx edit-config
```

Bump the version tag (interactive picker if no argument given):

```sh
gx bump
gx bump patch   # or minor / major
```

Check the repo for common configuration issues:

```sh
gx doctor
gx doctor --fix   # interactively apply fixes
```

Print the current binary version:

```sh
gx version
```

## Configuration

Optional config file:

```sh
~/.config/gx/config.json
```

Example:

```json
{
  "use-nerdfont-icons": true
}
```

## Key bindings

| Key            | Action                                                    |
| -------------- | --------------------------------------------------------- |
| `j` / `↓`      | Move down                                                 |
| `k` / `↑`      | Move up                                                   |
| `n`            | New worktree                                              |
| `N`            | New worktree and open a tmux session (switches to it)     |
| `T`            | New worktree and open a tmux window                       |
| `d`            | Delete selected worktree (and its branch)                 |
| `r`            | Rename selected worktree and branch                       |
| `c`            | Clone selected worktree (copies uncommitted files)        |
| `y`            | Yank files from selected worktree into clipboard          |
| `p`            | Pull selected worktree's branch (stash prompt if dirty)   |
| `P`            | Push selected worktree's branch (confirms before pushing) |
| `b`            | Rebase selected worktree on main (stash prompt if dirty)  |
| `l`            | Open selected worktree in lazygit                         |
| `o`            | View output log of last pull/push job                     |
| `/`            | Search worktrees by name or branch                        |
| `t`            | Track remote branch (set upstream)                        |
| `R`            | Refresh worktree list and statuses                        |
| `U`            | Run `git remote update` and refresh                       |
| `?`            | Toggle full help                                          |
| `q` / `Ctrl+C` | Quit                                                      |

### Search mode (after `/`)

| Key           | Action                            |
| ------------- | --------------------------------- |
| (type)        | Filter and highlight matches      |
| `ctrl+n`      | Jump to next match                |
| `ctrl+p`      | Jump to previous match            |
| `enter`/`esc` | Exit search, keep cursor position |

### Paste mode (after `y` + confirm)

| Key       | Action                                    |
| --------- | ----------------------------------------- |
| `j` / `↓` | Move down                                 |
| `k` / `↑` | Move up                                   |
| `p`       | Paste yanked files into selected worktree |
| `esc`     | Cancel and clear clipboard                |

## Development

```sh
make test   # run all tests
make run    # run without building
```
