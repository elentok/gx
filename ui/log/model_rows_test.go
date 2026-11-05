package log

import (
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/pushstate"
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

	m := newTestModelDefault(wtDir, "", ui.Settings{TicketStore: t.TempDir()})
	got := map[string]git.BranchHistoryClass{}
	for _, row := range m.listPanel.Rows() {
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
	local := pushstate.CommitPushState(git.BranchHistoryLocalOnly, false).Style.Render("local only")
	if ansi.Strip(local) != "local only" || local == "local only" {
		t.Fatalf("expected local-only subject to be colorized, got %q", local)
	}

	remote := pushstate.CommitPushState(git.BranchHistoryRemoteOnly, false).Style.Render("remote only")
	if ansi.Strip(remote) != "remote only" || remote == "remote only" {
		t.Fatalf("expected remote-only subject to be colorized, got %q", remote)
	}

	shared := pushstate.CommitPushState(git.BranchHistoryShared, false).Style.Render("shared")
	if ansi.Strip(shared) != "shared" || shared == "shared" {
		t.Fatalf("expected shared subject to be colorized, got %q", shared)
	}
}

func TestRenderCommitRowUsesDivergedLocalColor(t *testing.T) {
	normal := pushstate.CommitPushState(git.BranchHistoryLocalOnly, false).Style.Render("local only")
	diverged := pushstate.CommitPushState(git.BranchHistoryLocalOnly, true).Style.Render("local only")
	if normal == diverged {
		t.Fatalf("expected diverged local style to differ from normal local style")
	}
}

func TestGHResetsCustomRefToHead(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "one.txt", "one\n")
	testutil.CommitAll(t, repo, "one")

	m := newTestModelDefault(repo, "HEAD~1", ui.Settings{EnableNavigation: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if cmd == nil {
		t.Fatalf("expected nav command for gh")
	}
	vs, ok := nav.IsSwitch(cmd())
	if !ok {
		t.Fatalf("expected nav replace")
	}
	if vs.Tab != nav.TabLog || vs.Ref != "HEAD" {
		t.Fatalf("expected log HEAD view state, got tab=%q ref=%q", vs.Tab, vs.Ref)
	}
}

func TestSelectedCommitRowFillsFullWidth(t *testing.T) {
	m := newTestModel()
	m.width = 80
	r := row{
		kind: rowCommit,
		commit: git.LogEntry{
			Hash:        "12345678",
			AuthorShort: "AB",
			Subject:     "subject",
			Date:        time.Now().Add(-2 * time.Hour),
			Graph:       "*",
			Decorations: []git.RefDecoration{{Name: "main", Kind: git.RefDecorationLocalBranch}},
		},
	}
	m.listPanel = m.listPanel.WithRows([]row{r})
	lines := m.listPanel.WithHints(m.buildHints()).renderRow(r, true, 40)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines for a commit row, got %d", len(lines))
	}
	for _, line := range lines {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 40 {
			t.Fatalf("selected row line width = %d, want 40 (line %q)", got, line)
		}
		if line == ansi.Strip(line) {
			t.Fatalf("expected selected row to preserve nested ansi colors")
		}
	}
}

func TestMatchRefRule(t *testing.T) {
	rules := compileRefRules(config.DefaultLogConfig().ImportantRefs)

	tests := []struct {
		name      string
		ref       string
		wantMatch bool
	}{
		{name: "main matches", ref: "main", wantMatch: true},
		{name: "master matches", ref: "master", wantMatch: true},
		{name: "origin/main matches", ref: "origin/main", wantMatch: true},
		{name: "origin/master matches", ref: "origin/master", wantMatch: true},
		{name: "feature branch does not match", ref: "feature/x", wantMatch: false},
		{name: "version tag matches", ref: "v1.0.0", wantMatch: true},
		{name: "non-version ref does not match", ref: "some-branch", wantMatch: false},
	}

	for _, tt := range tests {
		_, ok := matchRefRule(tt.ref, rules)
		if ok != tt.wantMatch {
			t.Fatalf("%s: match = %v, want %v", tt.name, ok, tt.wantMatch)
		}
	}
}
