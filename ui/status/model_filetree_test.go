package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/status/filetree"
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

	if len(m.filetree.Data.Entries) < 2 {
		t.Fatalf("expected two status entries, got %d", len(m.filetree.Data.Entries))
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
	for i, entry := range m.filetree.Data.Entries {
		if entry.Path == "untracked.txt" {
			m.filetree.SetSelection(i)
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
	for i, entry := range m.filetree.Data.Entries {
		if entry.Kind == filetree.EntryFile && entry.Path == "ui/status/model.go" {
			fileIdx = i
			break
		}
	}
	if fileIdx < 0 {
		t.Fatalf("expected ui/status/model.go entry in status tree")
	}
	m.filetree.Data.ListState.SetSelected(fileIdx, len(m.filetree.Data.Entries))

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	_ = cmd
	if m.focus != focusFiletree {
		t.Fatalf("expected h to keep filetree focus")
	}
	entry, ok := m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "ui/status" {
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

	entry, ok := m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected compressed dir selected by default, got %+v", entry)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected left in filetree to be a no-op")
	}
	entry, ok = m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected selection to stay on compressed dir, got %+v", entry)
	}
}

func TestFiletreeHOnSelectedDirCollapsesDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entry, ok := m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "docs" {
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
	entry, ok := m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "docs" {
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
	entry, ok := m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "docs" || entry.Expanded {
		t.Fatalf("expected docs dir collapsed before expand test, got %+v", entry)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	entry, ok = m.filetree.SelectedEntry()
	if !ok || entry.Kind != filetree.EntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after l, got %+v", entry)
	}
	if !entry.Expanded {
		t.Fatalf("expected docs dir to expand on l")
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

	entry, ok := m.filetree.SelectedEntry()
	if !ok {
		t.Fatal("expected selected entry")
	}
	if entry.Kind != filetree.EntryFile || entry.Path != "dir/b.txt" {
		t.Fatalf("selected entry = %+v, want file dir/b.txt", entry)
	}
	if m.focus != focusFiletree {
		t.Fatalf("focus = %v, want %v", m.focus, focusFiletree)
	}
	if m.diffLoader.activeFilePath != "dir/b.txt" {
		t.Fatalf("activeFilePath = %q, want %q", m.diffLoader.activeFilePath, "dir/b.txt")
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

	before := m.diffLoader.activeFilePath
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected j in filetree to schedule debounced reload")
	}
	if m.diffLoader.activeFilePath != before {
		t.Fatalf("expected active file to remain unchanged before debounce fires")
	}

	updated, _ = m.Update(diffReloadMsg{seq: m.diffLoader.reloadSeq})
	m = updated.(Model)
	if m.diffLoader.activeFilePath == before {
		t.Fatalf("expected active file to update after debounce message")
	}
}

func TestNavigationStartupDefersInitialDiffLoad(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "dirty.txt", "dirty\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
	if m.diffLoader.activeFilePath != "" {
		t.Fatalf("expected initial navigation startup to skip diff load, got %q", m.diffLoader.activeFilePath)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) != 0 {
		t.Fatalf("expected no diff hunks before startup load, got %d", len(m.diffarea.Unstaged.DataRef().Parsed.Hunks))
	}

	updated, _ := m.Update(statusStartupLoadMsg{})
	m = updated.(Model)
	if m.diffLoader.activeFilePath != "dirty.txt" {
		t.Fatalf("expected startup load to select dirty diff, got %q", m.diffLoader.activeFilePath)
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
