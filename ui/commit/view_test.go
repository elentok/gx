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

func TestWrapDecorationBadges_NoBadges(t *testing.T) {
	lines := wrapDecorationBadges("subject", nil, 80)
	if len(lines) != 1 || lines[0] != "subject" {
		t.Errorf("wrapDecorationBadges(no badges) = %v, want [\"subject\"]", lines)
	}
}

func TestWrapDecorationBadges_FitsOnFirstLine(t *testing.T) {
	lines := wrapDecorationBadges("subject", []string{"a", "b"}, 80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "subject") || !strings.Contains(lines[0], "a") || !strings.Contains(lines[0], "b") {
		t.Errorf("expected subject and badges on line 1, got %q", lines[0])
	}
}

func TestWrapDecorationBadges_WrapsOntoNewLines(t *testing.T) {
	subject := "subject"
	badges := []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"}
	lines := wrapDecorationBadges(subject, badges, 15)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping onto multiple lines, got %v", lines)
	}
	if lines[0] != subject {
		t.Errorf("expected line 1 to be subject alone when no badge fits, got %q", lines[0])
	}
	for _, l := range lines[1:] {
		if ansi.StringWidth(l) > 15 {
			t.Errorf("line %q exceeds maxWidth", l)
		}
	}
}

func TestWrapDecorationBadges_SubjectAlwaysOnLine1(t *testing.T) {
	subject := "a very long subject line that takes most of the width"
	badges := []string{"tag1", "tag2"}
	lines := wrapDecorationBadges(subject, badges, 20)
	if !strings.HasPrefix(lines[0], subject) {
		t.Errorf("expected line 1 to start with subject, got %q", lines[0])
	}
}

func TestHeaderLines_NoDecorations_Unchanged(t *testing.T) {
	m := Model{details: git.CommitDetails{Subject: "fix bug", AuthorName: "Dave"}, width: 80}
	lines := m.headerLines()
	if len(lines) != 1 {
		t.Fatalf("expected 1 line with no decorations, got %d: %v", len(lines), lines)
	}
}

func TestHeaderLines_WithDecorations_AddsBadges(t *testing.T) {
	m := Model{
		details: git.CommitDetails{
			Subject:     "fix bug",
			AuthorName:  "Dave",
			Decorations: []git.RefDecoration{{Kind: git.RefDecorationLocalBranch, Name: "main"}},
		},
		width: 80,
	}
	lines := m.headerLines()
	if len(lines) != 1 {
		t.Fatalf("expected badges to fit on line 1, got %d lines: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "main") {
		t.Errorf("expected badge label in header line, got %q", lines[0])
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
