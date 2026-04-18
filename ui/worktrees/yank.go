package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// yankDataMsg is sent when the file list for yank mode has been loaded.
type yankDataMsg struct {
	worktreePath string
	changes      []git.Change
	err          error
}

// clipboardState holds the yanked file paths and their source worktree.
type clipboardState struct {
	srcPath string   // absolute path of source worktree
	srcName string   // display name
	files   []string // relative file paths
}

// cmdLoadYankData loads uncommitted changes for the given worktree.
func cmdLoadYankData(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		changes, err := git.UncommittedChanges(wt.Path)
		return yankDataMsg{worktreePath: wt.Path, changes: changes, err: err}
	}
}

// enterYankMode initiates loading the file list for the selected worktree.
func (m Model) enterYankMode() (Model, tea.Cmd) {
	wt := m.selectedWorktree()
	if wt == nil {
		return m, nil
	}
	m.mode = modeYank
	m.yankLoading = true
	m.yankSource = *wt
	m.statusMsg = ""
	return m, cmdLoadYankData(*wt)
}

// handleYankKey handles key events while the yank checklist modal is open.
func (m Model) handleYankKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "enter":
		files := m.yankChecklist.Checked()
		if len(files) > 0 {
			m.clipboard = &clipboardState{
				srcPath: m.yankSource.Path,
				srcName: m.yankSource.Name,
				files:   files,
			}
			m.mode = modePaste
		} else {
			m.mode = modeNormal
		}
		return m, nil
	}
	m.yankChecklist = m.yankChecklist.Update(msg.String())
	return m, nil
}

// yankModalView renders the centered yank checklist modal.
func (m Model) yankModalView() string {
	if m.yankLoading {
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Body:        "Loading files…",
			BorderColor: ui.ColorBorder,
			HintColor:   ui.ColorGray,
		})
	}

	modalW := m.width * 2 / 3
	if modalW < 44 {
		modalW = 44
	}
	if modalW > 84 {
		modalW = 84
	}
	// overhead: border(2) + title(1) + blank(1) + blank(1) + hint(1) = 6
	listH := m.height/2 - 6
	if listH < 3 {
		listH = 3
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       "Yank files from: " + m.yankSource.Name,
		Body:        m.yankChecklist.View(modalW-4, listH),
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
