package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
	"testing"
)

func TestSidebarSearchMovesToFileMatches(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.WriteFile(t, repo, "beta.txt", "two\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.WriteFile(t, repo, "beta.txt", "TWO\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.sidebar.fileTreeModel.Search().Mode() != search.SearchModeInput {
		t.Fatalf("expected / to enter search mode")
	}

	for _, r := range []rune("txt") {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if m.sidebar.fileTreeModel.Search().Query() != "txt" {
		t.Fatalf("search.Query() = %q, want txt", m.sidebar.fileTreeModel.Search().Query())
	}
	if m.sidebar.fileTreeModel.Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple sidebar matches, got %d", m.sidebar.fileTreeModel.Search().MatchesCount())
	}
	if m.focusDiff {
		t.Fatalf("expected sidebar search to keep focus in sidebar")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.sidebar.fileTreeModel.Search().Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}

	first := m.sidebar.fileTreeModel.SelectedIndex()
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.sidebar.fileTreeModel.SelectedIndex() == first {
		t.Fatalf("expected n to move to next sidebar match")
	}
}

func TestFilterPathHighlightsAndFocusesFileOnOpen(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.WriteFile(t, repo, "beta.txt", "two\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.WriteFile(t, repo, "beta.txt", "TWO\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModelWithFilter(repo, "HEAD", "beta.txt", ui.Settings{})
	file, ok := m.sidebar.selectedCommitFile()
	if !ok {
		t.Fatalf("expected selected commit file")
	}
	if file.Path != "beta.txt" {
		t.Fatalf("selected file = %q, want %q", file.Path, "beta.txt")
	}
	if m.sidebar.fileTreeModel.Search().Query() != "beta.txt" {
		t.Fatalf("search.Query() = %q, want %q", m.sidebar.fileTreeModel.Search().Query(), "beta.txt")
	}
	if m.sidebar.fileTreeModel.Search().Mode() != search.SearchModeNone {
		t.Fatalf("search.Mode() = %v, want SearchModeNone", m.sidebar.fileTreeModel.Search().Mode())
	}
	if m.sidebar.fileTreeModel.Search().MatchesCount() == 0 {
		t.Fatalf("expected sidebar search matches")
	}
}

func TestFilterPathPassiveSearchDoesNotBlockBack(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModelWithFilter(repo, "HEAD", "alpha.txt", ui.Settings{})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected nav back command on q")
	}
	if !nav.IsBack(cmd()) {
		t.Fatalf("expected nav back message")
	}
}

func TestDiffSearchMovesToMatchesAndNavigates(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "alpha\nTWO\nalpha three\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	for _, r := range []rune("alpha") {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if m.diffModel.Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple diff matches, got %d", m.diffModel.Search().MatchesCount())
	}
	if !m.focusDiff || m.diffModel.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected diff search to focus diff in line mode")
	}

	first := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine == first {
		t.Fatalf("expected n to move to next diff match")
	}
	second := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine == second {
		t.Fatalf("expected N to move to previous diff match")
	}
}
