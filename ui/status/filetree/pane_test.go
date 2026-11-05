package filetree

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/tree"
)

func TestBranchSummaryTitleOmitsBaseRef(t *testing.T) {
	t.Parallel()
	p := Pane{Data: Data{
		BranchName:    "feature/x",
		BranchBaseRef: "origin/release",
		BranchSync:    git.SyncStatus{Name: git.StatusBehind, Behind: 1},
	}}
	line := p.BranchSummaryTitleSuffix(math.MaxInt32, true)
	if !strings.Contains(line, " feature/x ↓1") {
		t.Fatalf("unexpected title suffix: %q", line)
	}
	if strings.Contains(line, "vs") {
		t.Fatalf("expected base ref to no longer appear in title suffix: %q", line)
	}
}

func TestBranchSummaryTitleTruncatesLongBranchNameWithEllipsis(t *testing.T) {
	t.Parallel()
	p := Pane{Data: Data{
		BranchName: "feature/some-extremely-verbose-branch-name-that-keeps-going",
		BranchSync: git.SyncStatus{Name: git.StatusBehind, Behind: 1},
	}}
	line := p.BranchSummaryTitleSuffix(30, false)
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

func TestPaneShowsBranchSummaryInTitle(t *testing.T) {
	t.Parallel()
	p := Pane{Data: Data{
		BranchName:    "feature/test",
		BranchBaseRef: "origin/main",
		BranchSync:    git.SyncStatus{Name: git.StatusAhead, Ahead: 2},
	}}

	title := p.Title(70, View{NerdIcons: true})
	for _, want := range []string{"Changes in", "feature/test", "↑2"} {
		if !strings.Contains(title, want) {
			t.Fatalf("expected title to contain %q, got %q", want, title)
		}
	}
	if strings.Contains(title, "vs origin/main") {
		t.Fatalf("expected default base ref to stay hidden, got %q", title)
	}
}

func TestPaneWorktreeLineOnlyForBareRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	if lines := (Pane{}).InfoLines(View{WorktreeRoot: repo}); len(lines) != 0 {
		t.Fatalf("expected no worktree line for a plain single-worktree clone, got %q", lines)
	}

	lines := (Pane{}).InfoLines(View{WorktreeRoot: repo, IsBareRepo: true, NerdIcons: true})
	want := "worktree: " + filepath.Base(repo)
	if len(lines) != 1 || !strings.Contains(ansi.Strip(lines[0]), want) {
		t.Fatalf("expected worktree line %q, got %q", want, lines)
	}
}

func TestRenderLines_UsesStatusSpecificLabelAndMeta(t *testing.T) {
	t.Parallel()
	p := Pane{Model: tree.NewModel[stage.StageFileStatus]()}
	file := stage.StageFileStatus{Path: "new.go", RenameFrom: "old.go", IndexStatus: 'R', WorktreeCode: ' '}
	p.Model.SetEntries([]tree.Entry[stage.StageFileStatus]{
		{DisplayName: "new.go", Value: file},
	})

	lines := p.RenderLines(30, 3, View{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "old.go -> new.go") || !strings.Contains(got, "R") {
		t.Fatalf("line = %q, want rename path and status metadata", got)
	}
}

func TestReloadBranchStateUsesBranchUpstream(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.WriteFile(t, wtDir, "ahead.txt", "ahead")
	testutil.CommitAll(t, wtDir, "ahead")

	var p Pane
	p.ReloadBranchState(wtDir)

	if p.Data.BranchName != "feature" {
		t.Fatalf("expected current branch feature, got %q", p.Data.BranchName)
	}
	if p.Data.BranchBaseRef != "origin/feature" {
		t.Fatalf("expected upstream base ref origin/feature, got %q", p.Data.BranchBaseRef)
	}

	// Branch sync is async; run the cmd synchronously to get the result.
	if cmd := p.CmdLoadBranchSync(wtDir); cmd != nil {
		msg := cmd().(BranchSyncLoadedMsg)
		p.ApplyBranchSync(msg.BranchName, msg.Sync)
	}
	if p.Data.BranchSync.Name != git.StatusAhead || p.Data.BranchSync.Ahead != 1 {
		t.Fatalf("expected branch sync ahead of origin/feature, got %+v", p.Data.BranchSync)
	}
}
