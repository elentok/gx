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

func TestBadgeColorForDecoration(t *testing.T) {
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
		_ = badgeColorForDecoration(d) // just ensure no panic
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

func TestPackBadgeLines_Empty(t *testing.T) {
	lines := packBadgeLines(nil, 80)
	if len(lines) != 0 {
		t.Errorf("packBadgeLines(nil) = %v, want empty", lines)
	}
}

func TestPackBadgeLines_FitsOnOneLine(t *testing.T) {
	lines := packBadgeLines([]string{"a", "b"}, 80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "a") || !strings.Contains(lines[0], "b") {
		t.Errorf("expected both badges on line 1, got %q", lines[0])
	}
}

func TestPackBadgeLines_WrapsOntoNewLines(t *testing.T) {
	badges := []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"}
	lines := packBadgeLines(badges, 15)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping onto multiple lines, got %v", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 15 {
			t.Errorf("line %q exceeds maxWidth", l)
		}
	}
}

func TestHeaderLines_NoDecorations_Unchanged(t *testing.T) {
	m := Model{details: git.CommitDetails{Subject: "fix bug", AuthorName: "Dave"}, width: 80}
	lines := m.headerLines()
	if len(lines) != 2 {
		t.Fatalf("expected subject line + meta line with no decorations, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(ansi.Strip(lines[0]), "fix bug") {
		t.Errorf("expected subject on line 1, got %q", ansi.Strip(lines[0]))
	}
	if !strings.Contains(ansi.Strip(lines[1]), "by Dave") {
		t.Errorf("expected author on line 2, got %q", ansi.Strip(lines[1]))
	}
}

func TestHeaderLines_PushStateColorsSubjectAndAddsLabel(t *testing.T) {
	m := Model{
		details:   git.CommitDetails{Subject: "fix bug", AuthorName: "Dave"},
		width:     80,
		pushState: ui.CommitPushState(git.BranchHistoryLocalOnly, false),
	}
	lines := m.headerLines()
	stripped := ansi.Strip(lines[0])
	if !strings.Contains(stripped, "fix bug") || !strings.Contains(stripped, "unpushed") {
		t.Fatalf("expected subject line with unpushed label, got %q", stripped)
	}
	if lines[0] == stripped {
		t.Fatal("expected subject line to be colorized by push state")
	}
}

func TestHeaderLines_WithDecorations_KeepsBadgeOnMetaLineWhenItFits(t *testing.T) {
	m := Model{
		details: git.CommitDetails{
			Subject:     "fix bug",
			AuthorName:  "Dave",
			Decorations: []git.RefDecoration{{Kind: git.RefDecorationLocalBranch, Name: "main"}},
		},
		width: 80,
	}
	lines := m.headerLines()
	if len(lines) != 2 {
		t.Fatalf("expected subject line + meta line with badge, got %d lines: %v", len(lines), lines)
	}
	stripped := ansi.Strip(lines[1])
	byIdx := strings.Index(stripped, "by Dave")
	badgeIdx := strings.Index(stripped, "main")
	if badgeIdx == -1 || byIdx == -1 || badgeIdx < byIdx {
		t.Errorf("expected badge after author on the meta line, got %q", stripped)
	}
}

func TestHeaderLines_WithDecorations_WrapsBadgeWhenItDoesNotFit(t *testing.T) {
	m := Model{
		details: git.CommitDetails{
			Subject:     "fix bug",
			AuthorName:  "Dave",
			Decorations: []git.RefDecoration{{Kind: git.RefDecorationLocalBranch, Name: "main"}},
		},
		width: 15,
	}
	lines := m.headerLines()
	if len(lines) != 3 {
		t.Fatalf("expected subject line + meta line + separate badge line, got %d lines: %v", len(lines), lines)
	}
	if !strings.Contains(ansi.Strip(lines[1]), "by Dave") {
		t.Errorf("expected meta line without badge, got %q", ansi.Strip(lines[1]))
	}
	if !strings.Contains(ansi.Strip(lines[2]), "main") {
		t.Errorf("expected badge on its own line, got %q", ansi.Strip(lines[2]))
	}
}

func TestVisibleFileLines_UsesCommitSpecificLabelAndMeta(t *testing.T) {
	m := Model{settings: ui.Settings{}, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.fileTreeModel.SetEntries([]filetree.Entry[git.CommitFile]{
		{Kind: filetree.EntryFile, DisplayName: "new.go", Value: git.CommitFile{Path: "new.go", RenameFrom: "old.go", Status: "R "}},
	})

	lines := m.visibleFileLines(30, 3)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "old.go -> new.go") || !strings.Contains(got, "R") {
		t.Fatalf("line = %q, want rename path and commit status metadata", got)
	}
}

func TestContainerFocusDisablesCommitPaneActiveStyling(t *testing.T) {
	m := Model{settings: ui.Settings{}, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.fileTreeModel.SetEntries([]filetree.Entry[git.CommitFile]{
		{Kind: filetree.EntryFile, DisplayName: "selected.go", Value: git.CommitFile{Path: "selected.go", Status: "M "}},
	})

	if m.filesPaneBorderColor() != ui.ColorOrange {
		t.Fatal("expected active file tree border by default")
	}
	if !m.filetreeRenderOpts().Active {
		t.Fatal("expected file tree render opts active by default")
	}
	if line := m.visibleFileLines(30, 3)[0]; !strings.Contains(line, "\x1b[48;2;") {
		t.Fatal("expected active selected file row to have a highlight background")
	}

	inactive := m.WithContainerFocus(false)
	if inactive.filesPaneBorderColor() != ui.ColorBorder {
		t.Fatal("expected inactive file tree border when container is not focused")
	}
	if inactive.filetreeRenderOpts().Active {
		t.Fatal("expected inactive file tree render opts when container is not focused")
	}
	if line := inactive.visibleFileLines(30, 3)[0]; strings.Contains(line, "\x1b[48;2;") {
		t.Fatal("expected inactive selected file row without highlight background")
	}

	inactive.focusDiff = true
	if inactive.diffPaneBorderColor() != ui.ColorBorder {
		t.Fatal("expected inactive diff border even when diff has internal focus")
	}

	inactive.focusDiff = false
	inactive.focusHeader = true
	if inactive.headerPaneBorderColor() != ui.ColorBorder {
		t.Fatal("expected inactive header border even when header has internal focus")
	}
}

func TestView_FiletreeSearchOverlayAppearsInView(t *testing.T) {
	m := Model{ready: true, width: 100, height: 20, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.fileTreeModel.Search().Start("files")
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "files") {
		t.Fatalf("expected filetree search overlay, got %q", got)
	}
}

func TestInputFocused_DiffSearchDelegatesIntoDiffModel(t *testing.T) {
	m := Model{ready: true, width: 100, height: 20, commitSidebarState: commitSidebarState{fileTreeModel: filetree.NewModel[git.CommitFile]()}}
	m.focusDiff = true
	m.diffModel.Search().Start("diff")
	if !m.InputFocused() {
		t.Fatal("expected InputFocused=true when diffModel search is active")
	}
}
