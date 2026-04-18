package worktrees

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
)

func boolPtr(v bool) *bool { return &v }

func TestRenderSidebarContent_IncludesBehindSection(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a"}
	ahead := []git.Commit{{Hash: "abc1234", Subject: "ahead commit"}}
	behind := []git.Commit{{Hash: "def5678", Subject: "behind commit"}}

	out := renderSidebarContent(wt, "origin/feature-a", git.Commit{}, ahead, behind, nil, false, nil, "", false)
	if !strings.Contains(out, "Commits ahead of remote") {
		t.Fatal("missing ahead section")
	}
	if !strings.Contains(out, "Commits behind remote") {
		t.Fatal("missing behind section")
	}
	if !strings.Contains(out, "behind commit") {
		t.Fatal("missing behind commit entry")
	}
}

func TestRenderSidebarContent_NoUpstream(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a"}
	out := renderSidebarContent(wt, "", git.Commit{}, nil, nil, nil, false, nil, "", false)
	if !strings.Contains(out, "no remote tracking branch") {
		t.Fatal("missing no-tracking note")
	}
	if !strings.Contains(out, "t") || !strings.Contains(out, "track") || !strings.Contains(out, "origin/<branch>") {
		t.Fatal("missing tracking hint")
	}
	if strings.Contains(out, "Commits ahead") {
		t.Fatal("should not show ahead section when no upstream")
	}
}

func TestRenderSidebarContent_UsesNerdFontIcons(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a"}
	out := renderSidebarContent(wt, "origin/feature-a", git.Commit{}, nil, nil, nil, false, nil, "", true)
	if !strings.Contains(out, "󰙅 Worktree") {
		t.Fatal("missing nerd-font worktree title")
	}
	if !strings.Contains(out, " Commits ahead of remote") {
		t.Fatal("missing nerd-font ahead title")
	}
}

func TestRenderSidebarContent_RebasedOnMain(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a", Branch: "feature-a"}
	out := renderSidebarContent(wt, "origin/feature-a", git.Commit{}, nil, nil, boolPtr(true), false, nil, "", false)
	if !strings.Contains(out, "rebased on main") {
		t.Fatal("expected 'rebased on main' indicator")
	}
}

func TestRenderSidebarContent_NeedsRebase(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a", Branch: "feature-a"}
	out := renderSidebarContent(wt, "origin/feature-a", git.Commit{}, nil, nil, boolPtr(false), false, nil, "", false)
	if !strings.Contains(out, "needs rebase on main") {
		t.Fatal("expected 'needs rebase on main' indicator")
	}
}

func TestRenderSidebarContent_MainBranchHidesSection(t *testing.T) {
	wt := &git.Worktree{Name: "main", Branch: "main"}
	out := renderSidebarContent(wt, "origin/main", git.Commit{}, nil, nil, nil, true, nil, "", false)
	if strings.Contains(out, "Base") {
		t.Fatal("main branch should not show base section")
	}
}

func TestRenderSidebarContent_SpinnerInTitle(t *testing.T) {
	wt := &git.Worktree{Name: "feature-a"}
	out := renderSidebarContent(wt, "", git.Commit{}, nil, nil, nil, false, nil, "⣾", false)
	if !strings.Contains(out, "⣾") {
		t.Fatal("expected spinner in output")
	}
	if !strings.Contains(out, "Worktree") {
		t.Fatal("title should still be present")
	}
}
