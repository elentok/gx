package status

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/tree"
)

// statusData is the file list and branch state the filetree pane shows.
type statusData struct {
	files         []stage.StageFileStatus
	branchName    string
	branchBaseRef string
	branchSync    git.SyncStatus
	statusEntries []statusEntry
	statusRows    []tree.Entry[stage.StageFileStatus]
	listState     list.Model
}

// filetreePane owns the file tree pane: the status data and the tree model
// that renders it.
type filetreePane struct {
	data  statusData
	model tree.Model[stage.StageFileStatus]
}

// FiletreeResult reports what a key press asked of the parent Model.
type FiletreeResult struct {
	SelectionChanged bool
	OpenSelected     bool
	Rebuilt          bool
	Back             bool
	PageMoved        bool
	ToggleStage      bool
	Discard          bool
}

// filetreeView carries the Model state the pane needs to render.
type filetreeView struct {
	worktreeRoot string
	isBareRepo   bool
	nerdIcons    bool
	active       bool
}

// Update handles a key press. The returned child command is the tree model's
// own command; the caller decides how to run it.
func (p filetreePane) Update(msg tea.KeyPressMsg) (filetreePane, tea.Cmd, FiletreeResult) {
	var res FiletreeResult
	p.reconcile()
	updated, childCmd, result := p.model.Update(msg)
	selectionChanged := updated.SelectedIndex() != p.model.SelectedIndex()
	p.model = updated

	if !result.Handled {
		match, consumed := p.model.Keys().Process(msg)
		if consumed && match == nil {
			return p, nil, res // chord in progress
		}
		if match != nil {
			switch match.ID {
			case tree.BindingBack:
				res.Back = true
			case tree.BindingPageDown:
				res.PageMoved = p.scrollPage(1)
			case tree.BindingPageUp:
				res.PageMoved = p.scrollPage(-1)
			case bindingToggleStage:
				res.ToggleStage = true
			case bindingDiscard:
				res.Discard = true
			}
		}
		return p, nil, res
	}

	if result.RebuildRequested {
		p.rebuild()
		res.Rebuilt = true
	}
	res.OpenSelected = result.OpenSelected
	res.SelectionChanged = selectionChanged
	return p, childCmd, res
}

func (p filetreePane) selectedEntry() (statusEntry, bool) {
	sel := p.data.listState.Selected()
	if sel < 0 || sel >= len(p.data.statusEntries) {
		return statusEntry{}, false
	}
	return p.data.statusEntries[sel], true
}

func (p filetreePane) selectedRow() (tree.Entry[stage.StageFileStatus], bool) {
	sel := p.data.listState.Selected()
	if sel < 0 || sel >= len(p.data.statusRows) {
		return tree.Entry[stage.StageFileStatus]{}, false
	}
	return p.data.statusRows[sel], true
}

func (p filetreePane) selectedFile() (stage.StageFileStatus, bool) {
	entry, ok := p.selectedEntry()
	if !ok || entry.Kind != statusEntryFile {
		return stage.StageFileStatus{}, false
	}
	return entry.File, true
}

// reconcile rebuilds the tree model's entries from the status rows.
//
// The final setSelection call is required after SetEntries because row
// count/shape may have changed. It reapplies the list selection, clamps it to a
// valid index in the tree model, and writes the clamped value back so parent and
// child selection cannot drift.
func (p *filetreePane) reconcile() {
	p.model.SetEntries(p.data.statusRows)
	p.setSelection(p.data.listState.Selected())
}

func (p *filetreePane) setSelection(index int) {
	p.model.SetSelectedIndex(index)
	p.syncListSelection()
}

// syncListSelection copies the tree model's selection into the list state.
func (p *filetreePane) syncListSelection() {
	p.data.listState.SetSelected(p.model.SelectedIndex(), len(p.data.statusEntries))
}

func (p *filetreePane) rebuild() {
	p.data.statusEntries, p.data.statusRows = buildStatusEntriesAndRows(p.data.files, p.model.CollapsedIDs())
	p.reconcile()
}

func (p *filetreePane) setFiles(files []stage.StageFileStatus) {
	p.data.files = files
	p.rebuild()
}

func (p *filetreePane) clearFiles() {
	p.data.files = nil
	p.data.statusEntries = nil
	p.data.statusRows = nil
	p.reconcile()
}

func (p *filetreePane) reloadBranchState(worktreeRoot string) {
	p.data.branchName = ""
	p.data.branchBaseRef = ""
	p.data.branchSync = git.SyncStatus{Name: git.StatusUnknown}

	branch, err := git.CurrentBranch(worktreeRoot)
	if err != nil || strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		return
	}
	p.data.branchName = strings.TrimSpace(branch)
	p.data.branchBaseRef = git.UpstreamBranch(worktreeRoot, p.data.branchName)
}

func (p *filetreePane) applyBranchSync(branchName string, sync git.SyncStatus) {
	if branchName == p.data.branchName {
		p.data.branchSync = sync
	}
}

func (p *filetreePane) cmdLoadBranchSync(worktreeRoot string) tea.Cmd {
	if p.data.branchName == "" || p.data.branchBaseRef == "" {
		return nil
	}
	branchName := p.data.branchName
	branchBaseRef := p.data.branchBaseRef
	return func() tea.Msg {
		sync, err := git.BranchSyncStatusAgainstRef(worktreeRoot, branchName, branchBaseRef)
		if err != nil {
			return branchSyncLoadedMsg{branchName: branchName, sync: git.SyncStatus{Name: git.StatusUnknown}}
		}
		return branchSyncLoadedMsg{branchName: branchName, sync: sync}
	}
}

func (p *filetreePane) scrollPage(direction int) bool {
	if len(p.model.Entries()) == 0 {
		return false
	}
	old := p.model.SelectedIndex()
	p.model.ScrollPage(direction * list.DefaultScroll)
	if p.model.SelectedIndex() == old {
		return false
	}
	p.setSelection(p.model.SelectedIndex())
	return true
}

// jumpTop selects the first entry and reports whether the selection moved.
func (p *filetreePane) jumpTop() bool {
	p.setSelection(p.data.listState.Selected())
	if len(p.model.Entries()) == 0 || p.model.SelectedIndex() == 0 {
		return false
	}
	p.setSelection(0)
	return true
}

// jumpBottom selects the last entry and reports whether the selection moved.
func (p *filetreePane) jumpBottom() bool {
	p.setSelection(p.data.listState.Selected())
	entryCount := len(p.model.Entries())
	if entryCount == 0 || p.model.SelectedIndex() == entryCount-1 {
		return false
	}
	p.setSelection(entryCount - 1)
	return true
}

// moveToAdjacent moves the selection to the next or previous file.
func (p *filetreePane) moveToAdjacent(delta int) bool {
	if !p.model.MoveToAdjacentFile(delta) {
		return false
	}
	p.setSelection(p.model.SelectedIndex())
	return true
}

// hasStashableChanges reports whether the current file list holds anything that
// the requested stash variant would capture. Untracked files are excluded: the
// stash is run without --include-untracked (out of scope), so `git stash push`
// ignores them and a tree of only-untracked files is "nothing to stash".
func (p filetreePane) hasStashableChanges(stagedOnly bool) bool {
	for _, f := range p.data.files {
		if f.HasStagedChanges() {
			return true
		}
		if !stagedOnly && !f.IsUntracked() && f.HasUnstagedChanges() {
			return true
		}
	}
	return false
}

func (p filetreePane) renderOpts(v filetreeView) tree.RenderOpts[stage.StageFileStatus] {
	icons := filetreePaneIconsFor(v.nerdIcons)
	return tree.RenderOpts[stage.StageFileStatus]{
		AccentColor: ui.ColorBlue,
		Active:      v.active,
		EmptyLine:   ui.StyleMuted.Render("clean working tree"),
		Icon:        statusTreeIcon(v.worktreeRoot, icons),
		Label:       statusTreeLabel(),
		MetaText: func(entry tree.Entry[stage.StageFileStatus]) string {
			return statusEntryMeta(statusEntryFromRow(entry), v.nerdIcons, icons)
		},
		RowColor: func(entry tree.Entry[stage.StageFileStatus]) string {
			return statusEntryColor(statusEntryFromRow(entry))
		},
		Faint: func(entry tree.Entry[stage.StageFileStatus]) bool {
			return !entry.HasChildren && isDeletedFileStatus(entry.Value)
		},
	}
}

func (p filetreePane) renderLines(width, height int, v filetreeView) []string {
	opts := p.renderOpts(v)
	opts.Width = width - 2
	return p.model.RenderLines(height, opts)
}

func (p filetreePane) requiredWidth(height int, v filetreeView) int {
	return p.model.RequiredWidth(height, p.renderOpts(v))
}

// statusTreeIcon renders a directory's expand/collapse folder glyph or a
// file's change-kind glyph, replacing filetree's built-in EntryKind-based
// folder icon now that ui/tree has no leaf-vs-directory split of its own.
func statusTreeIcon(worktreeRoot string, icons filetreePaneIcons) func(entry tree.Entry[stage.StageFileStatus]) string {
	return func(entry tree.Entry[stage.StageFileStatus]) string {
		if entry.HasChildren {
			if entry.Expanded {
				return icons.folderOpen
			}
			return icons.folderClosed
		}
		return statusFileIcon(entry.Value, isWorktreeSymlink(worktreeRoot, entry.Value.Path), icons)
	}
}

func statusTreeLabel() func(entry tree.Entry[stage.StageFileStatus]) string {
	return func(entry tree.Entry[stage.StageFileStatus]) string {
		if entry.HasChildren {
			return entry.DisplayName + "/"
		}
		if entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
			return entry.Value.RenameFrom + " -> " + entry.Value.Path
		}
		return entry.DisplayName
	}
}

func (p filetreePane) branchSummaryTitleSuffix(maxWidth int, nerdIcons bool) string {
	if strings.TrimSpace(p.data.branchName) == "" {
		return ""
	}
	branchLabel := "branch"
	if nerdIcons {
		branchLabel = ""
	}
	prefix := branchLabel + " "
	suffix := " " + p.branchSyncToken()
	name := p.data.branchName
	available := maxWidth - ansi.StringWidth(prefix) - ansi.StringWidth(suffix)
	if available > 0 && ansi.StringWidth(name) > available {
		name = ansi.Truncate(name, available, "…")
	}
	return prefix + name + suffix
}

func (p filetreePane) branchSyncToken() string {
	switch p.data.branchSync.Name {
	case git.StatusSame:
		return "✓"
	case git.StatusAhead:
		return fmt.Sprintf("↑%d", p.data.branchSync.Ahead)
	case git.StatusBehind:
		return fmt.Sprintf("↓%d", p.data.branchSync.Behind)
	case git.StatusDiverged:
		return fmt.Sprintf("↑%d ↓%d", p.data.branchSync.Ahead, p.data.branchSync.Behind)
	}
	return "?"
}
