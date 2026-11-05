package modal

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// Yank is the checklist of a worktree's uncommitted files to copy.
type Yank struct {
	source    git.Worktree
	loading   bool // true until the file list arrives
	checklist components.Checklist
}

// NewYank opens the yank modal for source, loading until Loaded is called.
func NewYank(source git.Worktree) Yank {
	return Yank{source: source, loading: true}
}

// Source is the worktree the files are yanked from.
func (y Yank) Source() git.Worktree { return y.source }

// Loaded returns the modal showing items, all as the checklist.
func (y Yank) Loaded(items []components.Item) Yank {
	y.loading = false
	y.checklist = components.NewChecklist(items)
	return y
}

// Update toggles checklist items, closes on esc, and on enter hands the
// checked files on as the paste clipboard.
func (y Yank) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return y, nil, Result{Unhandled: true}
	}
	switch k.String() {
	case "esc":
		return y, nil, Result{Close: true}
	case "enter":
		files := y.checklist.Checked()
		if len(files) == 0 {
			return y, nil, Result{Close: true}
		}
		return y, nil, Result{Close: true, Action: OpenPaste{Clipboard: Clipboard{
			SrcPath: y.source.Path,
			SrcName: y.source.Name,
			Files:   files,
		}}}
	}
	y.checklist = y.checklist.Update(k.String())
	return y, nil, Result{}
}

func (y Yank) InputFocused() bool { return false }

// View renders the centered yank checklist modal.
func (y Yank) View(width, height int) string {
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
