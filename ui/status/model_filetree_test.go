package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/status/diffarea"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestFiletreeLOnFileEntersDiffAndKeepsSectionOnFileChange(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.WriteFile(t, repo, "b.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt", "b.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// File a has both staged and unstaged changes.
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	// File b is only unstaged.
	testutil.WriteFile(t, repo, "b.txt", "ONE\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionStaged

	if len(m.filetree.data.statusEntries) < 2 {
		t.Fatalf("expected two status entries, got %d", len(m.filetree.data.statusEntries))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff {
		t.Fatalf("expected l on file to enter diff")
	}
	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged for same file")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if m.focus != focusFiletree {
		t.Fatalf("expected h in diff to return to filetree")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged after active file change")
	}
}

func TestReloadDiffsForSelection_KeepsSectionForUntrackedFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "tracked.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "tracked.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "untracked.txt", "hello\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionStaged

	found := false
	for i, entry := range m.filetree.data.statusEntries {
		if entry.Path == "untracked.txt" {
			m.filetree.setSelection(i)
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected untracked file entry")
	}

	cmd := m.reloadDiffsForSelection()
	m = runStatusCmd(t, m, cmd)

	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged for untracked file, got %v", m.diffarea.ActiveSection)
	}
	if len(m.diffarea.Staged.DataRef().Parsed.Hunks) != 0 {
		t.Fatalf("expected staged section to remain empty for untracked file")
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) == 0 {
		t.Fatalf("expected unstaged diff content for untracked file")
	}
}

func TestTabSwitchesDiffSectionsOnly(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// Ensure both unstaged and staged sections exist for same file.
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusFiletree || m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("tab in filetree should switch to staged without moving focus, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusFiletree || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("second tab in filetree should switch back to unstaged without moving focus, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("l should focus unstaged diff, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("tab in diff should switch to staged, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("second tab in diff should switch back to unstaged, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}
}

func TestFiletreeHOnFileMovesSelectionToParentDir(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/ui/status")
	testutil.WriteFile(t, repo, "ui/status/model.go", "package status\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	fileIdx := -1
	for i, entry := range m.filetree.data.statusEntries {
		if entry.Kind == statusEntryFile && entry.Path == "ui/status/model.go" {
			fileIdx = i
			break
		}
	}
	if fileIdx < 0 {
		t.Fatalf("expected ui/status/model.go entry in status tree")
	}
	m.filetree.data.listState.SetSelected(fileIdx, len(m.filetree.data.statusEntries))

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	_ = cmd
	if m.focus != focusFiletree {
		t.Fatalf("expected h to keep filetree focus")
	}
	entry, ok := m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "ui/status" {
		t.Fatalf("expected selection to move to parent dir, got %+v", entry)
	}
}

func TestFiletreeLeftDoesNotMoveCompressedDirSelection(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/keyboards/iris/keymaps")
	testutil.WriteFile(t, repo, "keyboards/iris/keymaps/myfile.c", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	entry, ok := m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected compressed dir selected by default, got %+v", entry)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected left in filetree to be a no-op")
	}
	entry, ok = m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected selection to stay on compressed dir, got %+v", entry)
	}
}

func TestFiletreeHOnSelectedDirCollapsesDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entry, ok := m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after h, got %+v", entry)
	}
	if entry.Expanded {
		t.Fatalf("expected docs dir to collapse on h")
	}
}

func TestFiletreeEnterOnSelectedDirCollapsesDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	entry, ok := m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after enter, got %+v", entry)
	}
	if entry.Expanded {
		t.Fatalf("expected docs dir to collapse on enter")
	}
}

func TestFiletreeRightOnSelectedDirExpandsDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entry, ok := m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" || entry.Expanded {
		t.Fatalf("expected docs dir collapsed before expand test, got %+v", entry)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	entry, ok = m.filetree.selectedEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after l, got %+v", entry)
	}
	if !entry.Expanded {
		t.Fatalf("expected docs dir to expand on l")
	}
}

func TestFiletreePaneShowsBranchSummaryInTitle(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.filetree.data.branchName = "feature/test"
	m.filetree.data.branchBaseRef = "origin/main"
	m.filetree.data.branchSync = git.SyncStatus{Name: git.StatusAhead, Ahead: 2}

	pane := ansi.Strip(m.renderFiletreePane(72, 10))
	if !strings.Contains(pane, "Changes in") {
		t.Fatalf("expected branch summary title, got:\n%s", pane)
	}
	if !strings.Contains(pane, "feature/test") {
		t.Fatalf("expected branch summary to include branch name, got:\n%s", pane)
	}
	if !strings.Contains(pane, "↑2") {
		t.Fatalf("expected branch summary to include ahead state, got:\n%s", pane)
	}
	if strings.Contains(pane, "vs origin/main") {
		t.Fatalf("expected default base ref to stay hidden, got:\n%s", pane)
	}
}

func TestFiletreePaneHidesWorktreeLineForNonBareRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true

	pane := ansi.Strip(m.renderFiletreePane(72, 10))
	if strings.Contains(pane, "worktree:") {
		t.Fatalf("expected no worktree line for a plain single-worktree clone, got:\n%s", pane)
	}
}

func TestFiletreePaneShowsWorktreeLineForBareRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, DefaultSettings(), "")
	m.isBareRepo = true
	m.ready = true

	pane := ansi.Strip(m.renderFiletreePane(72, 10))
	want := "worktree: " + filepath.Base(repo)
	if !strings.Contains(pane, want) {
		t.Fatalf("expected worktree line %q, got:\n%s", want, pane)
	}
}

func TestNewWithInitialPathSelectsFileAndKeepsFiletreeFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/dir")
	testutil.WriteFile(t, repo, "dir/a.txt", "one\n")
	testutil.WriteFile(t, repo, "dir/b.txt", "two\n")

	m := newTestModel(repo, ui.Settings{
		DiffContextLines: 1,
		UseNerdFontIcons: true,
	}, "dir/b.txt")

	entry, ok := m.filetree.selectedEntry()
	if !ok {
		t.Fatal("expected selected entry")
	}
	if entry.Kind != statusEntryFile || entry.Path != "dir/b.txt" {
		t.Fatalf("selected entry = %+v, want file dir/b.txt", entry)
	}
	if m.focus != focusFiletree {
		t.Fatalf("focus = %v, want %v", m.focus, focusFiletree)
	}
	if m.activeFilePath != "dir/b.txt" {
		t.Fatalf("activeFilePath = %q, want %q", m.activeFilePath, "dir/b.txt")
	}
}

func TestBranchSummaryTitleOmitsBaseRef(t *testing.T) {
	t.Parallel()
	m := Model{
		settings: ui.Settings{UseNerdFontIcons: true},
		filetree: filetreePane{data: statusData{
			branchName:    "feature/x",
			branchBaseRef: "origin/release",
			branchSync:    git.SyncStatus{Name: git.StatusBehind, Behind: 1},
		}},
	}
	line := m.filetree.branchSummaryTitleSuffix(math.MaxInt32, m.settings.UseNerdFontIcons)
	if !strings.Contains(line, " feature/x ↓1") {
		t.Fatalf("unexpected title suffix: %q", line)
	}
	if strings.Contains(line, "vs") {
		t.Fatalf("expected base ref to no longer appear in title suffix: %q", line)
	}
}

func TestBranchSummaryTitleTruncatesLongBranchNameWithEllipsis(t *testing.T) {
	t.Parallel()
	m := Model{
		filetree: filetreePane{data: statusData{
			branchName: "feature/some-extremely-verbose-branch-name-that-keeps-going",
			branchSync: git.SyncStatus{Name: git.StatusBehind, Behind: 1},
		}},
	}
	line := m.filetree.branchSummaryTitleSuffix(30, m.settings.UseNerdFontIcons)
	if !strings.Contains(line, "…") {
		t.Fatalf("expected truncated branch name to use ellipsis, got %q", line)
	}
	if !strings.HasSuffix(line, "↓1") {
		t.Fatalf("expected sync token to stay visible at the end, got %q", line)
	}
	if ansi.StringWidth(line) > 30 {
		t.Fatalf("expected title suffix to fit within maxWidth, got %q (%d wide)", line, ansi.StringWidth(line))
	}
}

func TestReloadBranchStateUsesBranchUpstream(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.WriteFile(t, wtDir, "ahead.txt", "ahead")
	testutil.CommitAll(t, wtDir, "ahead")

	m := newTestModelDefault(wtDir)
	m.filetree.reloadBranchState(m.worktreeRoot)

	if m.filetree.data.branchName != "feature" {
		t.Fatalf("expected current branch feature, got %q", m.filetree.data.branchName)
	}
	if m.filetree.data.branchBaseRef != "origin/feature" {
		t.Fatalf("expected upstream base ref origin/feature, got %q", m.filetree.data.branchBaseRef)
	}

	// Branch sync is now async; run the cmd synchronously to get the result.
	if cmd := m.filetree.cmdLoadBranchSync(m.worktreeRoot); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}
	if m.filetree.data.branchSync.Name != git.StatusAhead || m.filetree.data.branchSync.Ahead != 1 {
		t.Fatalf("expected branch sync ahead of origin/feature, got %+v", m.filetree.data.branchSync)
	}
}

func TestStatusFileIconDeletedAndFallback(t *testing.T) {
	t.Parallel()
	deleted := stage.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'}

	nerd := filetreePaneIconsFor(true)
	if got := statusFileIcon(deleted, false, nerd); got != "" {
		t.Fatalf("expected deleted nerd icon, got %q", got)
	}

	plain := filetreePaneIconsFor(false)
	if got := statusFileIcon(deleted, false, plain); got != "D" {
		t.Fatalf("expected deleted fallback icon, got %q", got)
	}
}

func TestStatusEntryColorDeletedFileIsDim(t *testing.T) {
	t.Parallel()
	entry := statusEntry{
		Kind: statusEntryFile,
		File: stage.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'},
	}
	if got := statusEntryColor(entry); got != "#a6adc8" {
		t.Fatalf("expected dim deleted color, got %q", got)
	}
}

func TestStatusFileIconRenamedAndFallback(t *testing.T) {
	t.Parallel()
	renamed := stage.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'}

	nerd := filetreePaneIconsFor(true)
	if got := statusFileIcon(renamed, false, nerd); got != "󰁔" {
		t.Fatalf("expected renamed nerd icon, got %q", got)
	}

	plain := filetreePaneIconsFor(false)
	if got := statusFileIcon(renamed, false, plain); got != "R" {
		t.Fatalf("expected renamed fallback icon, got %q", got)
	}
}

func TestStatusEntryColorRenamedFileIsBlue(t *testing.T) {
	t.Parallel()
	entry := statusEntry{
		Kind: statusEntryFile,
		File: stage.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'},
	}
	if got := statusEntryColor(entry); got != "#89b4fa" {
		t.Fatalf("expected renamed color, got %q", got)
	}
}

func TestFiletreeSelectionDebouncesDiffReload(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	before := m.activeFilePath
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected j in filetree to schedule debounced reload")
	}
	if m.activeFilePath != before {
		t.Fatalf("expected active file to remain unchanged before debounce fires")
	}

	updated, _ = m.Update(diffReloadMsg{seq: m.diffReloadSeq})
	m = updated.(Model)
	if m.activeFilePath == before {
		t.Fatalf("expected active file to update after debounce message")
	}
}

func TestNavigationStartupDefersInitialDiffLoad(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "dirty.txt", "dirty\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
	if m.activeFilePath != "" {
		t.Fatalf("expected initial navigation startup to skip diff load, got %q", m.activeFilePath)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) != 0 {
		t.Fatalf("expected no diff hunks before startup load, got %d", len(m.diffarea.Unstaged.DataRef().Parsed.Hunks))
	}

	updated, _ := m.Update(statusStartupLoadMsg{})
	m = updated.(Model)
	if m.activeFilePath != "dirty.txt" {
		t.Fatalf("expected startup load to select dirty diff, got %q", m.activeFilePath)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) == 0 {
		t.Fatalf("expected diff hunks after startup load")
	}
}

func TestDefaultSettingsEnableNerdFontIcons(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	if !settings.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestStatusEntryColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry statusEntry
		want  string
	}{
		{name: "unstaged", entry: statusEntry{HasUnstaged: true}, want: "#cdd6f4"},
		{name: "partial", entry: statusEntry{HasStaged: true, HasUnstaged: true}, want: "#fab387"},
		{name: "staged", entry: statusEntry{HasStaged: true}, want: "#a6e3a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusEntryColor(tt.entry); got != tt.want {
				t.Fatalf("statusEntryColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEntryMeta_NerdFont(t *testing.T) {
	t.Parallel()
	icons := filetreePaneIconsFor(true)

	if got := statusEntryMeta(statusEntry{HasStaged: true, HasUnstaged: true}, true, icons); got != "" {
		t.Fatalf("partial nerd icon = %q", got)
	}
	if got := statusEntryMeta(statusEntry{HasStaged: true}, true, icons); got != "" {
		t.Fatalf("staged nerd icon = %q", got)
	}
}

func TestStatusFileIcon(t *testing.T) {
	t.Parallel()
	icons := filetreePaneIconsFor(true)

	if got := statusFileIcon(stage.StageFileStatus{IndexStatus: '?', WorktreeCode: '?'}, false, icons); got != "" {
		t.Fatalf("untracked icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(stage.StageFileStatus{IndexStatus: 'A', WorktreeCode: ' '}, false, icons); got != "" {
		t.Fatalf("added icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(stage.StageFileStatus{IndexStatus: ' ', WorktreeCode: 'M'}, false, icons); got != "" {
		t.Fatalf("modified icon = %q, want modified file icon", got)
	}
}

func TestStatusFileIconSymlink(t *testing.T) {
	t.Parallel()
	nerd := filetreePaneIconsFor(true)
	plain := filetreePaneIconsFor(false)

	modified := stage.StageFileStatus{Path: "link", IndexStatus: ' ', WorktreeCode: 'M'}
	if got := statusFileIcon(modified, true, nerd); got != "󰌷" {
		t.Fatalf("symlink nerd icon = %q, want symlink icon", got)
	}
	if got := statusFileIcon(modified, true, plain); got != "L" {
		t.Fatalf("symlink plain icon = %q, want L", got)
	}

	// Deleted symlink keeps the delete icon (deletion takes precedence).
	deleted := stage.StageFileStatus{Path: "link", IndexStatus: 'D'}
	if got := statusFileIcon(deleted, true, nerd); got == "󰌷" {
		t.Fatalf("deleted symlink should show delete icon, not symlink icon")
	}
}
