package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// yankDataMsg is sent when the file list for yank mode has been loaded.
type yankDataMsg struct {
	worktreePath string
	changes      []git.Change
	err          error
}

// cmdLoadYankData loads uncommitted changes for the given worktree.
func cmdLoadYankData(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		changes, err := git.UncommittedChanges(wt.Path)
		return yankDataMsg{worktreePath: wt.Path, changes: changes, err: err}
	}
}

// yankModal is the checklist of a worktree's uncommitted files to copy.
type yankModal struct {
	source    git.Worktree
	loading   bool // true until the file list arrives
	checklist components.Checklist
}

// enterYankMode opens the yank modal and loads the file list for the selected
// worktree.
func (m Model) enterYankMode() (Model, tea.Cmd) {
	wt := m.cursorWorktree()
	if wt == nil {
		return m, nil
	}
	return m.openModal(yankModal{source: *wt, loading: true}), cmdLoadYankData(*wt)
}

// Update toggles checklist items, closes on esc, and on enter hands the
// checked files on as the paste clipboard.
func (y yankModal) Update(msg tea.Msg) (modal.Modal, tea.Cmd, modal.Result) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return y, nil, modal.Result{Unhandled: true}
	}
	switch k.String() {
	case "esc":
		return y, nil, modal.Result{Close: true}
	case "enter":
		files := y.checklist.Checked()
		if len(files) == 0 {
			return y, nil, modal.Result{Close: true}
		}
		return y, nil, modal.Result{Close: true, Action: modal.OpenPaste{Clipboard: modal.Clipboard{
			SrcPath: y.source.Path,
			SrcName: y.source.Name,
			Files:   files,
		}}}
	}
	y.checklist = y.checklist.Update(k.String())
	return y, nil, modal.Result{}
}

func (y yankModal) InputFocused() bool { return false }

// View renders the centered yank checklist modal.
func (y yankModal) View(width, height int) string {
	if y.loading {
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Body:        "Loading files…",
			BorderColor: ui.ColorBorder,
			HintColor:   ui.ColorGray,
		})
	}

	modalW := width * 2 / 3
	if modalW < 44 {
		modalW = 44
	}
	if modalW > 84 {
		modalW = 84
	}
	// overhead: border(2) + title(1) + blank(1) + blank(1) + hint(1) = 6
	listH := height/2 - 6
	if listH < 3 {
		listH = 3
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       "Yank files from: " + y.source.Name,
		Body:        y.checklist.View(modalW-4, listH),
		Hint:        ui.HintChecklistConfirm(),
		Width:       modalW - 4,
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		HintColor:   ui.ColorGray,
	})
}

// changesToChecklistItems converts git changes into checklist items.
func changesToChecklistItems(changes []git.Change) []components.Item {
	items := make([]components.Item, len(changes))
	for i, c := range changes {
		path := strings.TrimSuffix(c.Path, "/") // untracked dirs have trailing slash
		items[i] = components.Item{
			Label:   string(c.Kind) + "  " + path,
			Value:   filepath.ToSlash(path),
			Checked: true,
		}
	}
	return items
}
