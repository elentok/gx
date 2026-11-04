package log

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestRenderCommitRowHighlightsSearchMatches(t *testing.T) {
	m := newTestModel()
	m.search.Start("fix")
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "fix search highlighting",
		},
	}
	m.listPanel = m.listPanel.WithRows([]row{r})

	lines := m.listPanel.WithHints(m.buildHints()).renderCommitRow(r)
	line := strings.Join(lines, "\n")
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "fix search highlighting") {
		t.Fatalf("stripped line = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered commit row")
	}
}

func TestRenderBadgesHighlightsSearchMatches(t *testing.T) {
	m := newTestModel()
	m.search.Start("main")
	line := m.listPanel.WithHints(m.buildHints()).renderBadges([]git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}})
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "origin/main") {
		t.Fatalf("stripped badges = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered badges")
	}
}

func TestCloseSearchKeepsMatchesVisible(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := newTestModelDefault(repo, "", settings)
	m.search.Start("fix")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected search matches")
	}

	m.search.DismissAndKeepResults()
	if m.search.Query() != "fix" {
		t.Fatalf("expected query to persist after close, got %q", m.search.Query())
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after close")
	}
	if got := ansi.Strip(m.frameRightTitle()); !strings.Contains(got, "1/2 matches") {
		t.Fatalf("expected search match count in frame title, got %q", got)
	}
}

func TestNAndNShiftMoveBetweenSearchResults(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := newTestModelDefault(repo, "", settings)
	m.search.Start("fix")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected at least two search matches, got %d", m.search.MatchesCount())
	}
	if match, ok := m.search.Match(0); ok {
		m.listPanel = m.listPanel.SetSelected(match.DataIndex)
	}
	m.search.SetCursor(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); ok && m.listPanel.Selected() != match.DataIndex {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); ok && m.listPanel.Selected() != match.DataIndex {
		t.Fatalf("expected N to move to previous result")
	}
}

func TestTagJumpChordsMoveToTaggedCommits(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1", Decorations: []git.RefDecoration{{Name: "v1.0.0", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c2"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c3", Decorations: []git.RefDecoration{{Name: "v2.0.0", Kind: git.RefDecorationTag}}}},
	}).SetSelected(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 1 {
		t.Fatalf("expected ]t to jump to first tag at 1, got %d", m.listPanel.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 3 {
		t.Fatalf("expected ]t to jump to next tag at 3, got %d", m.listPanel.Selected())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 1 {
		t.Fatalf("expected [t to jump back to tag at 1, got %d", m.listPanel.Selected())
	}
}

func TestTagJumpChordStopsAtEdges(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0", Decorations: []git.RefDecoration{{Name: "v1", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	}).SetSelected(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.listPanel.Selected() != 0 {
		t.Fatalf("expected [t at first tag to stay put, got %d", m.listPanel.Selected())
	}
}
