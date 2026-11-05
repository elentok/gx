package commit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	notifypkg "github.com/elentok/gx/ui/notify"
)

func TestCMOutsideDiffDoesNothing(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.focusDiff = false

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("first c should not run command")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("cm outside diff should not run command")
	}
}

func TestAskAIInDiffWithoutFileContextShowsError(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	m.focusDiff = true
	m.sidebar.files = nil
	m.sidebar.fileTreeModel.SetEntries(nil)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	_ = m
	if cmd == nil {
		t.Fatalf("aa (Ask AI) without file context should return a notify cmd")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok || notifyMsg.Message != "no file context for comment" {
		t.Fatalf("expected notify msg %q, got %T %+v", "no file context for comment", msg, msg)
	}
}

func TestEEOpensEditorFromFiletreeAndDiff(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "line1\nline2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "edit.txt", "line1\nline2 changed\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	// ee chord: sidebar focus
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in sidebar focus to launch editor command")
	}

	// ee chord: diff focus
	m.focusDiff = true
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	_ = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in diff focus to launch editor command")
	}
}

func TestEditChordSplitVariantsCommit(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "edit.txt", "one changed\n")
	testutil.CommitAll(t, repo, "change")

	chords := []struct {
		name   string
		second string
	}{
		{"es (split)", "s"},
		{"ev (vsplit)", "v"},
		{"et (tab)", "t"},
	}

	for _, tt := range chords {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(repo, "HEAD")
			m.ready = true

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(Model)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: tt.second})
			_ = updated.(Model)
			if cmd == nil {
				t.Fatalf("expected e%s chord to return a non-nil cmd", tt.second)
			}
		})
	}
}

func TestEditorLineForCurrentSelectionInDiffModeCommit(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "old-1\nold-2\nold-3\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "line.txt", "new-1\nnew-2\nnew-3\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diff.diffModel.SetNavMode(diffview.NavModeLine)

	for i, cl := range m.diff.diffModel.DataRef().Parsed.Changed {
		if cl.NewLine == 2 {
			m.diff.diffModel.DataRef().ActiveLine = i
			break
		}
	}

	line := m.editorLineForCurrentSelection()
	if line != 2 {
		t.Fatalf("editorLineForCurrentSelection()=%d, want 2", line)
	}
}

func TestEditorLineForCurrentSelectionInSidebarFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "changed\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.focusDiff = false

	line := m.editorLineForCurrentSelection()
	if line != 0 {
		t.Fatalf("editorLineForCurrentSelection() in sidebar focus = %d, want 0", line)
	}
}
