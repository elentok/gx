package commit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filetree"
)

func TestIsMainOrMasterRef(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"main", true},
		{"master", true},
		{"origin/main", true},
		{"origin/master", true},
		{"feature/foo", false},
		{"", false},
		{"  main  ", true},
	}
	for _, c := range cases {
		if got := isMainOrMasterRef(c.in); got != c.want {
			t.Errorf("isMainOrMasterRef(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestBadgeVariantForDecoration(t *testing.T) {
	cases := []struct {
		kind git.RefDecorationKind
		name string
	}{
		{git.RefDecorationTag, "v1.0"},
		{git.RefDecorationLocalBranch, "main"},
		{git.RefDecorationLocalBranch, "feature"},
		{git.RefDecorationRemoteBranch, "origin/main"},
		{"unknown_kind", "other"},
	}
	for _, c := range cases {
		d := git.RefDecoration{Kind: c.kind, Name: c.name}
		_ = badgeVariantForDecoration(d) // just ensure no panic
	}
}

func TestRenderBadges_Empty(t *testing.T) {
	out := renderBadges(nil)
	if out != "" {
		t.Errorf("renderBadges(nil) = %q, want empty", out)
	}
}

func TestRenderBadges_NonEmpty(t *testing.T) {
	decorations := []git.RefDecoration{
		{Kind: git.RefDecorationTag, Name: "v1.0"},
		{Kind: git.RefDecorationLocalBranch, Name: "main"},
	}
	out := renderBadges(decorations)
	if out == "" {
		t.Error("expected non-empty renderBadges output")
	}
}

func TestVisibleFileLines_UsesCommitSpecificLabelAndMeta(t *testing.T) {
	m := Model{settings: ui.Settings{}, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.fileTreeModel.SetEntries([]filetree.Entry[git.CommitFile]{
		{Kind: filetree.EntryFile, DisplayName: "new.go", Value: git.CommitFile{Path: "new.go", RenameFrom: "old.go", Status: "R "}},
	})

	lines := m.visibleFileLines(3)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "old.go -> new.go") || !strings.Contains(got, "R") {
		t.Fatalf("line = %q, want rename path and commit status metadata", got)
	}
}

func TestView_RoutesSearchOverlayToActivePane(t *testing.T) {
	m := Model{ready: true, width: 100, height: 20, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.fileTreeModel.Search().Start("files")
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "files") {
		t.Fatalf("expected filetree search overlay, got %q", got)
	}

	m.fileTreeModel.Search().DismissAndClear()
	m.focusDiff = true
	m.search.Start("diff")
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "diff") {
		t.Fatalf("expected diff search overlay, got %q", got)
	}
}
