package log

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReloadAssignsBranchHistoryClasses(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.MustGitExported(t, wtDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, wtDir, "shared.txt", "shared\n")
	testutil.CommitAll(t, wtDir, "shared commit")
	testutil.MustGitExported(t, wtDir, "push", "--force-with-lease", "-u", "origin", "feature")

	testutil.WriteFile(t, wtDir, "remote.txt", "remote\n")
	testutil.CommitAll(t, wtDir, "remote only")
	testutil.MustGitExported(t, wtDir, "push")

	testutil.MustGitExported(t, wtDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, wtDir, "local.txt", "local\n")
	testutil.CommitAll(t, wtDir, "local only")

	m := New(wtDir, "")
	got := map[string]git.BranchHistoryClass{}
	for _, row := range m.rows {
		if row.kind != rowCommit {
			continue
		}
		got[row.commit.Subject] = row.class
	}

	if got["shared commit"] != git.BranchHistoryShared {
		t.Fatalf("shared commit class = %q", got["shared commit"])
	}
	if got["local only"] != git.BranchHistoryLocalOnly {
		t.Fatalf("local only class = %q", got["local only"])
	}
}

func TestRenderCommitRowUsesBranchHistoryColors(t *testing.T) {
	local := logSubjectStyle(git.BranchHistoryLocalOnly).Render("local only")
	if ansi.Strip(local) != "local only" || local == "local only" {
		t.Fatalf("expected local-only subject to be colorized, got %q", local)
	}

	remote := logSubjectStyle(git.BranchHistoryRemoteOnly).Render("remote only")
	if ansi.Strip(remote) != "remote only" || remote == "remote only" {
		t.Fatalf("expected remote-only subject to be colorized, got %q", remote)
	}

	shared := logSubjectStyle(git.BranchHistoryShared).Render("shared")
	if shared != "shared" {
		t.Fatalf("expected shared subject to stay uncolored, got %q", shared)
	}
}

func TestGHResetsCustomRefToHead(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "one")

	m := NewWithSettings(repo, "HEAD~1", Settings{EnableNavigation: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if cmd == nil {
		t.Fatalf("expected nav command for gh")
	}
	route, ok := nav.IsReplace(cmd())
	if !ok {
		t.Fatalf("expected nav replace")
	}
	if route.Kind != nav.RouteLog || route.Ref != "HEAD" {
		t.Fatalf("expected log HEAD route, got kind=%q ref=%q", route.Kind, route.Ref)
	}
}

func TestEnterOnCommitRowOpensCommitRoute(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New(repo, "")
	for i := range m.rows {
		if m.rows[i].kind == rowCommit {
			m.cursor = i
			break
		}
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected nav command on enter")
	}
	route, ok := nav.IsPush(cmd())
	if !ok {
		t.Fatalf("expected nav push")
	}
	if route.Kind != nav.RouteCommit {
		t.Fatalf("expected commit route, got %q", route.Kind)
	}
	_ = updated
}

func TestSelectedCommitRowFillsFullWidth(t *testing.T) {
	m := Model{
		width: 80,
		rows: []row{{
			kind: rowCommit,
			commit: git.LogEntry{
				Hash:        "12345678",
				AuthorShort: "AB",
				Subject:     "subject",
				Date:        time.Now().Add(-2 * time.Hour),
				Graph:       "*",
				Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
			},
		}},
	}
	line := m.renderRow(m.rows[0], true, 40)
	if got := ansi.StringWidth(ansi.Strip(line)); got != 40 {
		t.Fatalf("selected row width = %d, want 40", got)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected selected row to preserve nested ansi colors")
	}
}

func TestBadgeVariantForDecoration(t *testing.T) {
	tests := []struct {
		name string
		dec  git.RefDecoration
		want ui.BadgeVariant
	}{
		{name: "main local branch", dec: git.RefDecoration{Name: "main", Kind: git.RefDecorationLocalBranch}, want: ui.BadgeVariantYellow},
		{name: "main remote branch", dec: git.RefDecoration{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}, want: ui.BadgeVariantYellow},
		{name: "feature branch", dec: git.RefDecoration{Name: "feature/x", Kind: git.RefDecorationLocalBranch}, want: ui.BadgeVariantMauve},
		{name: "tag", dec: git.RefDecoration{Name: "v1.0.0", Kind: git.RefDecorationTag}, want: ui.BadgeVariantBlue},
	}

	for _, tt := range tests {
		if got := badgeVariantForDecoration(tt.dec); got != tt.want {
			t.Fatalf("%s: variant = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRenderCommitRowHighlightsSearchMatches(t *testing.T) {
	m := Model{
		searchQuery: "fix",
		rows: []row{{
			kind: rowCommit,
			commit: git.LogEntry{
				Hash:        "12345678",
				AuthorShort: "AB",
				Subject:     "fix search highlighting",
			},
		}},
	}

	line := m.renderCommitRow(m.rows[0])
	if stripped := ansi.Strip(line); !strings.Contains(stripped, "fix search highlighting") {
		t.Fatalf("stripped line = %q", stripped)
	}
	if line == ansi.Strip(line) {
		t.Fatalf("expected ansi highlight in rendered commit row")
	}
}

func TestRenderBadgesHighlightsSearchMatches(t *testing.T) {
	m := Model{searchQuery: "main"}
	line := m.renderBadges([]git.RefDecoration{{Name: "origin/main", Kind: git.RefDecorationRemoteBranch}})
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

	m := New(repo, "")
	m.enterSearchMode()
	m.searchQuery = "fix"
	m.recomputeSearchMatches()
	if len(m.searchMatch) == 0 {
		t.Fatalf("expected search matches")
	}

	m.closeSearch()
	if m.searchQuery != "fix" {
		t.Fatalf("expected query to persist after close, got %q", m.searchQuery)
	}
	if len(m.searchMatch) == 0 {
		t.Fatalf("expected matches to persist after close")
	}
}

func TestNAndNShiftMoveBetweenSearchResults(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := New(repo, "")
	m.searchQuery = "fix"
	m.recomputeSearchMatches()
	if len(m.searchMatch) < 2 {
		t.Fatalf("expected at least two search matches, got %d", len(m.searchMatch))
	}
	m.cursor = m.searchMatch[0]
	m.searchCursor = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.cursor != m.searchMatch[1] {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if m.cursor != m.searchMatch[0] {
		t.Fatalf("expected N to move to previous result")
	}
}

func TestTagJumpChordsMoveToTaggedCommits(t *testing.T) {
	m := Model{
		rows: []row{
			{kind: rowCommit, commit: git.LogEntry{Subject: "c0"}},
			{kind: rowCommit, commit: git.LogEntry{Subject: "c1", Decorations: []git.RefDecoration{{Name: "v1.0.0", Kind: git.RefDecorationTag}}}},
			{kind: rowCommit, commit: git.LogEntry{Subject: "c2"}},
			{kind: rowCommit, commit: git.LogEntry{Subject: "c3", Decorations: []git.RefDecoration{{Name: "v2.0.0", Kind: git.RefDecorationTag}}}},
		},
		cursor: 0,
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected ]t to jump to first tag at 1, got %d", m.cursor)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.cursor != 3 {
		t.Fatalf("expected ]t to jump to next tag at 3, got %d", m.cursor)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected [t to jump back to tag at 1, got %d", m.cursor)
	}
}

func TestTagJumpChordStopsAtEdges(t *testing.T) {
	m := Model{
		rows: []row{
			{kind: rowCommit, commit: git.LogEntry{Subject: "c0", Decorations: []git.RefDecoration{{Name: "v1", Kind: git.RefDecorationTag}}}},
			{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
		},
		cursor: 0,
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected [t at first tag to stay put, got %d", m.cursor)
	}
}
