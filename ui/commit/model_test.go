package commit

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keys"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestModel(worktreeRoot, ref string) Model {
	return NewModel(worktreeRoot, ref, "", ui.Settings{UseNerdFontIcons: true}, keys.Manager{})
}

func newTestModelWithFilter(worktreeRoot, ref, filterPath string, settings ui.Settings) Model {
	return NewModel(worktreeRoot, ref, filterPath, settings, keys.Manager{})
}

func TestCurrentRef(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	if m.CurrentRef() != "HEAD" {
		t.Errorf("CurrentRef() = %q, want 'HEAD'", m.CurrentRef())
	}
}

func TestKeyManager(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	km := m.KeyManager()
	_ = km // just verify no panic
}

func TestInputFocused(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	if m.InputFocused() {
		t.Error("expected InputFocused=false initially")
	}
}

func TestNewLoadsCommitDetails(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "first")

	m := newTestModel(repo, "HEAD")
	if m.err != nil {
		t.Fatalf("New err: %v", m.err)
	}
	if m.details.Hash == "" || m.details.Subject != "first" {
		t.Fatalf("unexpected details: %#v", m.details)
	}
}

func TestBToggleBody(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nbody")

	m := newTestModel(repo, "HEAD")
	if !m.bodyExpanded {
		t.Fatalf("expected body expanded by default")
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if m.bodyExpanded {
		t.Fatalf("expected body collapsed after b")
	}
	m.ready = true
	m.width = 80
	m.height = 20
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated.(Model)
	if !m.focusHeader {
		t.Fatalf("expected i to open the commit info popup")
	}
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "body hidden") {
		t.Fatalf("expected collapsed view without body-hidden hint")
	}
	if !strings.Contains(view, "subject") {
		t.Fatalf("expected collapsed header to show subject line")
	}
	if !strings.Contains(view, "by Test") {
		t.Fatalf("expected collapsed header to show author line")
	}
	if !strings.Contains(view, "Commit (b to expand)") {
		t.Fatalf("expected collapsed title hint")
	}
}

func TestEnterFocusesDiffAndJKMoveInsideDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "one\nTWO\nTHREE\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.focusDiff {
		t.Fatalf("expected enter to focus diff")
	}
	if m.diffModel.NavMode() != diffview.NavModeHunk {
		t.Fatalf("expected default nav mode hunk, got %v", m.diffModel.NavMode())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.diffModel.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected t to switch to line mode, got %v", m.diffModel.NavMode())
	}

	before := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine <= before {
		t.Fatalf("expected j to move active line, before=%d after=%d", before, m.diffModel.Data().ActiveLine)
	}
}

func TestJKWithoutDiffFocusMoveFiles(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "A\n")
	testutil.WriteFile(t, repo, "b.txt", "B\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	if len(m.files) < 2 {
		t.Fatalf("expected multiple files, got %d", len(m.files))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.fileTreeModel.SelectedIndex() != 1 {
		t.Fatalf("expected j to move selected file, got %d", m.fileTreeModel.SelectedIndex())
	}
	if m.focusDiff {
		t.Fatalf("expected file navigation not to force diff focus")
	}
}

func TestEscLeavesDiffFocusBeforeBackingOut(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected esc in diff to only change focus")
	}
	if m.focusDiff {
		t.Fatalf("expected esc to leave diff focus")
	}
}

func TestHeaderInfoPopupTogglesAndReclaimsSpace(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "popup-subject")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	closedView := ansi.Strip(m.View().Content)
	if strings.Contains(closedView, "popup-subject") {
		t.Fatalf("expected subject hidden while popup is closed")
	}
	_, closedContentH := m.layoutHeights()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated.(Model)
	if !m.focusHeader {
		t.Fatalf("expected i to open the commit info popup")
	}
	openView := ansi.Strip(m.View().Content)
	if !strings.Contains(openView, "popup-subject") {
		t.Fatalf("expected subject visible while popup is open, got:\n%s", openView)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.focusHeader {
		t.Fatalf("expected esc to close the popup")
	}
	_, reclaimedContentH := m.layoutHeights()
	if reclaimedContentH != closedContentH {
		t.Fatalf("expected content height unaffected by popup, got %d want %d", reclaimedContentH, closedContentH)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if m.focusHeader {
		t.Fatalf("expected q to close the popup")
	}
}

func TestTabCyclesBetweenSidebarDiffAndHeader(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.focusDiff {
		t.Fatalf("expected tab to focus diff")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.focusHeader || m.focusDiff {
		t.Fatalf("expected second tab to focus header")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focusHeader || m.focusDiff {
		t.Fatalf("expected third tab to return focus to files")
	}
}

func TestCommitMessageBodyNormalizesMixedNewlines(t *testing.T) {
	m := Model{
		details: git.CommitDetails{
			Subject: "subject",
			Body:    "subject\r\nline 1\nline 2\rline 3",
		},
	}

	if got := m.commitMessageBody(); got != "line 1\nline 2\nline 3" {
		t.Fatalf("commitMessageBody = %q", got)
	}
}
