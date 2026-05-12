package filetree

import (
	"testing"

	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelUpdate_NavigationAndOpen(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
		{Kind: EntryFile, Path: "dir/b.txt", ParentPath: "dir", DisplayName: "b.txt", Value: 2},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !result.Handled {
		t.Fatal("expected j to be handled")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1", next.SelectedIndex())
	}

	next, _, result = next.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !result.Handled {
		t.Fatal("expected k to be handled")
	}
	if next.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0", next.SelectedIndex())
	}

	next.SetSelectedIndex(1)
	next, _, result = next.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !result.Handled {
		t.Fatal("expected l to be handled")
	}
	if !result.OpenSelected {
		t.Fatal("expected OpenSelected result on file")
	}
}

func TestModelUpdate_DirExpandCollapse(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for collapse")
	}
	if !next.CollapsedDirs()["dir"] {
		t.Fatal("expected dir to be collapsed")
	}

	next.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: false},
	})
	next, _, result = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Handled {
		t.Fatal("expected enter to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for enter toggle")
	}
	if next.CollapsedDirs()["dir"] {
		t.Fatal("expected dir to be expanded")
	}
}

func TestModelUpdate_LeftOnNestedExpandedDirCollapsesBeforeParent(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "top", DisplayName: "top", Expanded: true},
		{Kind: EntryDir, Path: "top/nested", ParentPath: "top", DisplayName: "nested", Expanded: true},
		{Kind: EntryFile, Path: "top/nested/a.txt", ParentPath: "top/nested", DisplayName: "a.txt", Value: 1},
	})
	m.SetSelectedIndex(1) // nested dir

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected rebuild request (collapse) before parent focus")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("expected selection to stay on nested dir for collapse, got %d", next.SelectedIndex())
	}
	if !next.CollapsedDirs()["top/nested"] {
		t.Fatal("expected nested dir to be marked collapsed")
	}
}

func TestMoveToAdjacentFile(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
		{Kind: EntryDir, Path: "other", DisplayName: "other", Expanded: true},
		{Kind: EntryFile, Path: "other/b.txt", ParentPath: "other", DisplayName: "b.txt", Value: 2},
	})

	m.SetSelectedIndex(1)
	if ok := m.MoveToAdjacentFile(1); !ok {
		t.Fatal("expected move down to adjacent file")
	}
	if m.SelectedIndex() != 3 {
		t.Fatalf("selected=%d want=3", m.SelectedIndex())
	}
	if ok := m.MoveToAdjacentFile(1); ok {
		t.Fatal("expected no move past last file")
	}
	if ok := m.MoveToAdjacentFile(-1); !ok {
		t.Fatal("expected move up to previous file")
	}
	if m.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1", m.SelectedIndex())
	}
}

func TestModelUpdate_SearchStartAndQueryMsg(t *testing.T) {
	m := NewModel[int]()

	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !result.Handled {
		t.Fatal("expected / to be handled")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	next, cmd, result := next.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !result.Handled {
		t.Fatal("expected a to be handled in search input")
	}
	if cmd == nil {
		t.Fatal("expected search query updated cmd")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("unexpected cmd msg type %T", msg)
	}

	found := false
	for _, batchCmd := range batch {
		if queryMsg, ok := batchCmd().(search.SearchQueryUpdatedMsg); ok {
			if queryMsg.Query != "a" {
				t.Fatalf("query=%q want=a", queryMsg.Query)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected SearchQueryUpdatedMsg in batch")
	}
}
