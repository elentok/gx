package worktrees

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
)

func TestDirtyStateFromChanges(t *testing.T) {
	tests := []struct {
		name    string
		changes []git.Change
		want    dirtyState
	}{
		{name: "clean", changes: nil, want: dirtyState{}},
		{
			name:    "modified only",
			changes: []git.Change{{Kind: git.ChangeModified, Path: "a.txt"}},
			want:    dirtyState{hasModified: true},
		},
		{
			name:    "untracked only",
			changes: []git.Change{{Kind: git.ChangeUntracked, Path: "a.txt"}},
			want:    dirtyState{hasUntracked: true},
		},
		{
			name: "mixed",
			changes: []git.Change{
				{Kind: git.ChangeUntracked, Path: "a.txt"},
				{Kind: git.ChangeModified, Path: "b.txt"},
			},
			want: dirtyState{hasModified: true, hasUntracked: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dirtyStateFromChanges(tt.changes)
			if got != tt.want {
				t.Fatalf("dirtyStateFromChanges() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWorktreeCellBranchSuffix(t *testing.T) {
	ic := icons(false)
	icNerd := icons(true)

	// Branch same as worktree name — no suffix.
	got := worktreeCell("feature-a", "feature-a", ic, false, false)
	if strings.Contains(got, "(") {
		t.Errorf("same name/branch: unexpected suffix in %q", got)
	}

	// Branch empty — no suffix.
	got = worktreeCell("feature-a", "", ic, false, false)
	if strings.Contains(got, "(") {
		t.Errorf("empty branch: unexpected suffix in %q", got)
	}

	// Branch differs — suffix shows branch name.
	got = worktreeCell("my-worktree", "feature/TICKET-123", ic, false, false)
	if !strings.Contains(got, "my-worktree") {
		t.Errorf("worktree name missing: %q", got)
	}
	if !strings.Contains(got, "(feature/TICKET-123)") {
		t.Errorf("branch suffix missing or wrong: %q", got)
	}

	// With nerd font: suffix includes branch prefix icon.
	got = worktreeCell("my-worktree", "feature/TICKET-123", icNerd, false, false)
	if !strings.Contains(got, icNerd.branchPrefix) {
		t.Errorf("nerd font branch prefix missing in %q", got)
	}
	if !strings.Contains(got, "feature/TICKET-123") {
		t.Errorf("branch name missing in %q", got)
	}

	// Selected row — suffix still contains the branch name.
	plain := worktreeCell("my-worktree", "other-branch", ic, false, true)
	if !strings.Contains(plain, "other-branch") {
		t.Errorf("selected: branch name missing in %q", plain)
	}

	// Main branch with differing branch name — whole cell is orange (styleMainBranch).
	got = worktreeCell("main-wt", "main", ic, true, false)
	if !strings.Contains(got, "main-wt") {
		t.Errorf("main+diff: worktree name missing in %q", got)
	}
	if !strings.Contains(got, "main") {
		t.Errorf("main+diff: branch name missing in %q", got)
	}
}

func TestDirtyCellSymbols(t *testing.T) {
	tests := []struct {
		name  string
		dirty dirtyState
		want  string
	}{
		{name: "clean", dirty: dirtyState{}, want: "-"},
		{name: "modified", dirty: dirtyState{hasModified: true}, want: "M"},
		{name: "untracked", dirty: dirtyState{hasUntracked: true}, want: "?"},
		{name: "mixed", dirty: dirtyState{hasModified: true, hasUntracked: true}, want: "M?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dirtyCell(tt.dirty, icons(false), false)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("dirtyCell() = %q, want symbol %q", got, tt.want)
			}
		})
	}
}

func TestDirtyAndStatusCellSelectedKeepContent(t *testing.T) {
	if got := dirtyCell(dirtyState{hasModified: true, hasUntracked: true}, icons(false), true); !strings.Contains(got, "M?") {
		t.Fatalf("dirtyCell(selected) = %q, want symbol %q", got, "M?")
	}
	if got := statusCell(git.SyncStatus{Name: git.StatusSame}, icons(false), true, false); !strings.Contains(got, "synced") {
		t.Fatalf("statusCell(selected) = %q, want %q", got, "synced")
	}
}

func TestStatusCellNerdFontReplacesAheadBehind(t *testing.T) {
	s := git.SyncStatus{Name: git.StatusDiverged, Ahead: 2, Behind: 1}
	got := statusCell(s, icons(true), false, true)
	if !strings.Contains(got, "") || !strings.Contains(got, "") {
		t.Fatalf("statusCell() = %q, expected nerd-font arrows", got)
	}
	if strings.Contains(got, "ahead") || strings.Contains(got, "behind") {
		t.Fatalf("statusCell() = %q, should not include words ahead/behind", got)
	}
}
