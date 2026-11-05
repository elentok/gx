package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/tree"
)

const (
	BindingToggleStage keys.BindingID = "toggle-stage"
	BindingDiscard     keys.BindingID = "discard"
)

// BranchSyncLoadedMsg carries the result of the async branch sync lookup.
type BranchSyncLoadedMsg struct {
	BranchName string
	Sync       git.SyncStatus
}

// Data is the file list and branch state the filetree pane shows.
type Data struct {
	Files         []stage.StageFileStatus
	BranchName    string
	BranchBaseRef string
	BranchSync    git.SyncStatus
	Entries       []Entry
	Rows          []tree.Entry[stage.StageFileStatus]
	ListState     list.Model
}

// Pane owns the file tree pane: the status data and the tree model
// that renders it.
type Pane struct {
	Data  Data
	Model tree.Model[stage.StageFileStatus]
}

// Result reports what a key press asked of the parent Model.
type Result struct {
	SelectionChanged bool
	OpenSelected     bool
	Rebuilt          bool
	Back             bool
	PageMoved        bool
	ToggleStage      bool
	Discard          bool
}

// View carries the Model state the pane needs to render.
type View struct {
	WorktreeRoot string
	IsBareRepo   bool
	NerdIcons    bool
	Active       bool
}

// Update handles a key press. The returned child command is the tree model's
// own command; the caller decides how to run it.
func (p Pane) Update(msg tea.KeyPressMsg) (Pane, tea.Cmd, Result) {
	var res Result
	p.reconcile()
	updated, childCmd, result := p.Model.Update(msg)
	selectionChanged := updated.SelectedIndex() != p.Model.SelectedIndex()
	p.Model = updated

	if !result.Handled {
		match, consumed := p.Model.Keys().Process(msg)
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
			case BindingToggleStage:
				res.ToggleStage = true
			case BindingDiscard:
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

func (p Pane) SelectedEntry() (Entry, bool) {
	sel := p.Data.ListState.Selected()
	if sel < 0 || sel >= len(p.Data.Entries) {
		return Entry{}, false
	}
	return p.Data.Entries[sel], true
}

func (p Pane) SelectedRow() (tree.Entry[stage.StageFileStatus], bool) {
	sel := p.Data.ListState.Selected()
	if sel < 0 || sel >= len(p.Data.Rows) {
		return tree.Entry[stage.StageFileStatus]{}, false
	}
	return p.Data.Rows[sel], true
}

func (p Pane) SelectedFile() (stage.StageFileStatus, bool) {
	entry, ok := p.SelectedEntry()
	if !ok || entry.Kind != EntryFile {
		return stage.StageFileStatus{}, false
	}
	return entry.File, true
}

// reconcile rebuilds the tree model's entries from the status rows.
//
// The final SetSelection call is required after SetEntries because row
// count/shape may have changed. It reapplies the list selection, clamps it to a
// valid index in the tree model, and writes the clamped value back so parent and
// child selection cannot drift.
func (p *Pane) reconcile() {
	p.Model.SetEntries(p.Data.Rows)
	p.SetSelection(p.Data.ListState.Selected())
}

func (p *Pane) SetSelection(index int) {
	p.Model.SetSelectedIndex(index)
	p.SyncListSelection()
}

// SyncListSelection copies the tree model's selection into the list state.
func (p *Pane) SyncListSelection() {
	p.Data.ListState.SetSelected(p.Model.SelectedIndex(), len(p.Data.Entries))
}

func (p *Pane) rebuild() {
	p.Data.Entries, p.Data.Rows = buildEntriesAndRows(p.Data.Files, p.Model.CollapsedIDs())
	p.reconcile()
}

func (p *Pane) SetFiles(files []stage.StageFileStatus) {
	p.Data.Files = files
	p.rebuild()
}

func (p *Pane) ClearFiles() {
	p.Data.Files = nil
	p.Data.Entries = nil
	p.Data.Rows = nil
	p.reconcile()
}

func (p *Pane) ReloadBranchState(worktreeRoot string) {
	p.Data.BranchName = ""
	p.Data.BranchBaseRef = ""
	p.Data.BranchSync = git.SyncStatus{Name: git.StatusUnknown}

	branch, err := git.CurrentBranch(worktreeRoot)
	if err != nil || strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		return
	}
	p.Data.BranchName = strings.TrimSpace(branch)
	p.Data.BranchBaseRef = git.UpstreamBranch(worktreeRoot, p.Data.BranchName)
}

func (p *Pane) ApplyBranchSync(branchName string, sync git.SyncStatus) {
	if branchName == p.Data.BranchName {
		p.Data.BranchSync = sync
	}
}

func (p *Pane) CmdLoadBranchSync(worktreeRoot string) tea.Cmd {
	if p.Data.BranchName == "" || p.Data.BranchBaseRef == "" {
		return nil
	}
	branchName := p.Data.BranchName
	branchBaseRef := p.Data.BranchBaseRef
	return func() tea.Msg {
		sync, err := git.BranchSyncStatusAgainstRef(worktreeRoot, branchName, branchBaseRef)
		if err != nil {
			return BranchSyncLoadedMsg{BranchName: branchName, Sync: git.SyncStatus{Name: git.StatusUnknown}}
		}
		return BranchSyncLoadedMsg{BranchName: branchName, Sync: sync}
	}
}

func (p *Pane) scrollPage(direction int) bool {
	if len(p.Model.Entries()) == 0 {
		return false
	}
	old := p.Model.SelectedIndex()
	p.Model.ScrollPage(direction * list.DefaultScroll)
	if p.Model.SelectedIndex() == old {
		return false
	}
	p.SetSelection(p.Model.SelectedIndex())
	return true
}

// JumpTop selects the first entry and reports whether the selection moved.
func (p *Pane) JumpTop() bool {
	p.SetSelection(p.Data.ListState.Selected())
	if len(p.Model.Entries()) == 0 || p.Model.SelectedIndex() == 0 {
		return false
	}
	p.SetSelection(0)
	return true
}

// JumpBottom selects the last entry and reports whether the selection moved.
func (p *Pane) JumpBottom() bool {
	p.SetSelection(p.Data.ListState.Selected())
	entryCount := len(p.Model.Entries())
	if entryCount == 0 || p.Model.SelectedIndex() == entryCount-1 {
		return false
	}
	p.SetSelection(entryCount - 1)
	return true
}

// MoveToAdjacent moves the selection to the next or previous file.
func (p *Pane) MoveToAdjacent(delta int) bool {
	if !p.Model.MoveToAdjacentFile(delta) {
		return false
	}
	p.SetSelection(p.Model.SelectedIndex())
	return true
}

// HasStashableChanges reports whether the current file list holds anything that
// the requested stash variant would capture. Untracked files are excluded: the
// stash is run without --include-untracked (out of scope), so `git stash push`
// ignores them and a tree of only-untracked files is "nothing to stash".
func (p Pane) HasStashableChanges(stagedOnly bool) bool {
	for _, f := range p.Data.Files {
		if f.HasStagedChanges() {
			return true
		}
		if !stagedOnly && !f.IsUntracked() && f.HasUnstagedChanges() {
			return true
		}
	}
	return false
}

func (p Pane) renderOpts(v View) tree.RenderOpts[stage.StageFileStatus] {
	icons := paneIconsFor(v.NerdIcons)
	return tree.RenderOpts[stage.StageFileStatus]{
		AccentColor: ui.ColorBlue,
		Active:      v.Active,
		EmptyLine:   ui.StyleMuted.Render("clean working tree"),
		Icon:        treeIcon(v.WorktreeRoot, icons),
		Label:       treeLabel(),
		MetaText: func(entry tree.Entry[stage.StageFileStatus]) string {
			return entryMeta(entryFromRow(entry), v.NerdIcons, icons)
		},
		RowColor: func(entry tree.Entry[stage.StageFileStatus]) string {
			return entryColor(entryFromRow(entry))
		},
		Faint: func(entry tree.Entry[stage.StageFileStatus]) bool {
			return !entry.HasChildren && isDeletedFileStatus(entry.Value)
		},
	}
}

func (p Pane) RenderLines(width, height int, v View) []string {
	opts := p.renderOpts(v)
	opts.Width = width - 2
	return p.Model.RenderLines(height, opts)
}

func (p Pane) requiredWidth(height int, v View) int {
	return p.Model.RequiredWidth(height, p.renderOpts(v))
}

// treeIcon renders a directory's expand/collapse folder glyph or a
// file's change-kind glyph, replacing filetree's built-in EntryKind-based
// folder icon now that ui/tree has no leaf-vs-directory split of its own.
func treeIcon(worktreeRoot string, icons paneIcons) func(entry tree.Entry[stage.StageFileStatus]) string {
	return func(entry tree.Entry[stage.StageFileStatus]) string {
		if entry.HasChildren {
			if entry.Expanded {
				return icons.folderOpen
			}
			return icons.folderClosed
		}
		return fileIcon(entry.Value, isWorktreeSymlink(worktreeRoot, entry.Value.Path), icons)
	}
}

func treeLabel() func(entry tree.Entry[stage.StageFileStatus]) string {
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

// isWorktreeSymlink reports whether the file at relPath inside worktreeRoot is a
// symlink in the working tree. Returns false if the file does not exist or any
// error occurs (e.g. deleted files).
func isWorktreeSymlink(worktreeRoot, relPath string) bool {
	info, err := os.Lstat(filepath.Join(worktreeRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func (p Pane) BranchSummaryTitleSuffix(maxWidth int, nerdIcons bool) string {
	if strings.TrimSpace(p.Data.BranchName) == "" {
		return ""
	}
	branchLabel := "branch"
	if nerdIcons {
		branchLabel = ""
	}
	prefix := branchLabel + " "
	suffix := " " + p.branchSyncToken()
	name := p.Data.BranchName
	available := maxWidth - ansi.StringWidth(prefix) - ansi.StringWidth(suffix)
	if available > 0 && ansi.StringWidth(name) > available {
		name = ansi.Truncate(name, available, "…")
	}
	return prefix + name + suffix
}

func (p Pane) branchSyncToken() string {
	switch p.Data.BranchSync.Name {
	case git.StatusSame:
		return "✓"
	case git.StatusAhead:
		return fmt.Sprintf("↑%d", p.Data.BranchSync.Ahead)
	case git.StatusBehind:
		return fmt.Sprintf("↓%d", p.Data.BranchSync.Behind)
	case git.StatusDiverged:
		return fmt.Sprintf("↑%d ↓%d", p.Data.BranchSync.Ahead, p.Data.BranchSync.Behind)
	}
	return "?"
}
