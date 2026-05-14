package log

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var settings = Settings{}

func newTestModel() Model {
	m := Model{
		keys:   newLogManager(),
		search: search.NewModel(),
	}
	m.help = help.NewModel(buildKeySections(m.keys))
	return m
}

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

	m := NewModel(wtDir, "", settings)
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
	local := logSubjectStyle(git.BranchHistoryLocalOnly, false).Render("local only")
	if ansi.Strip(local) != "local only" || local == "local only" {
		t.Fatalf("expected local-only subject to be colorized, got %q", local)
	}

	remote := logSubjectStyle(git.BranchHistoryRemoteOnly, false).Render("remote only")
	if ansi.Strip(remote) != "remote only" || remote == "remote only" {
		t.Fatalf("expected remote-only subject to be colorized, got %q", remote)
	}

	shared := logSubjectStyle(git.BranchHistoryShared, false).Render("shared")
	if shared != "shared" {
		t.Fatalf("expected shared subject to stay uncolored, got %q", shared)
	}
}

func TestRenderCommitRowUsesDivergedLocalColor(t *testing.T) {
	normal := logSubjectStyle(git.BranchHistoryLocalOnly, false).Render("local only")
	diverged := logSubjectStyle(git.BranchHistoryLocalOnly, true).Render("local only")
	if normal == diverged {
		t.Fatalf("expected diverged local style to differ from normal local style")
	}
}

func TestGHResetsCustomRefToHead(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "one")

	m := NewModel(repo, "HEAD~1", Settings{EnableNavigation: true})
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

	m := NewModel(repo, "", settings)
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
	m := newTestModel()
	m.width = 80
	m.rows = []row{{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "subject",
			Date:        time.Now().Add(-2 * time.Hour),
			Graph:       "*",
			Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
		},
	}}
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
	m := newTestModel()
	m.search.Start("fix")
	m.rows = []row{{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "fix search highlighting",
		},
	}}

	line := m.renderCommitRow(m.rows[0])
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

	m := NewModel(repo, "", settings)
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
}

func TestNAndNShiftMoveBetweenSearchResults(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "fix one")
	testutil.WriteFile(t, repo, "two.txt", "two\n")
	testutil.CommitAll(t, repo, "fix two")

	m := NewModel(repo, "", settings)
	m.search.Start("fix")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected at least two search matches, got %d", m.search.MatchesCount())
	}
	if match, ok := m.search.Match(0); ok {
		m.cursor = match.Index
	}
	m.search.SetCursor(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); ok && m.cursor != match.Index {
		t.Fatalf("expected n to move to next result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); ok && m.cursor != match.Index {
		t.Fatalf("expected N to move to previous result")
	}
}

func TestTagJumpChordsMoveToTaggedCommits(t *testing.T) {
	m := newTestModel()
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1", Decorations: []git.RefDecoration{{Name: "v1.0.0", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c2"}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c3", Decorations: []git.RefDecoration{{Name: "v2.0.0", Kind: git.RefDecorationTag}}}},
	}
	m.cursor = 0

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
	m := newTestModel()
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{Subject: "c0", Decorations: []git.RefDecoration{{Name: "v1", Kind: git.RefDecorationTag}}}},
		{kind: rowCommit, commit: git.LogEntry{Subject: "c1"}},
	}
	m.cursor = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected [t at first tag to stay put, got %d", m.cursor)
	}
}

func TestFocusReloadsRowsAfterOnDiskChange(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := NewModel(repo, "", settings)
	initialRows := len(m.rows)

	testutil.WriteFile(t, repo, "new.txt", "new\n")
	testutil.CommitAll(t, repo, "new commit")

	updated, cmd := m.Update(tea.FocusMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected focus to trigger reload command")
	}

	reload, ok := cmd().(reloadMsg)
	if !ok {
		t.Fatalf("expected reloadMsg from focus reload cmd")
	}

	updated, _ = m.Update(reload)
	m = updated.(Model)
	if len(m.rows) <= initialRows {
		t.Fatalf("expected more rows after focus reload; before=%d after=%d", initialRows, len(m.rows))
	}
}
