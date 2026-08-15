package worktrees

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

func newTestModelWithWorktrees(t *testing.T, n int) Model {
	t.Helper()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.worktrees = make([]git.Worktree, n)
	for i := range m.worktrees {
		m.worktrees[i] = git.Worktree{Name: fmt.Sprintf("wt-%02d", i), Path: fmt.Sprintf("/tmp/wt-%02d", i)}
	}
	m.ready = true
	m.width = 200
	m.height = 30
	m = m.resized()
	m.table.SetRows(m.buildRows())
	return m
}

func TestMouseWheelOverTableScrollsTable(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 20)
	rect := m.tableRect()
	prevCursor := m.table.Cursor()
	prevOffset := m.viewport.YOffset()

	updated, _ := m.Update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 2, Button: tea.MouseWheelDown})
	m2 := updated.(Model)

	if m2.table.Cursor() <= prevCursor {
		t.Fatalf("expected table cursor to advance, before=%d after=%d", prevCursor, m2.table.Cursor())
	}
	if m2.viewport.YOffset() != prevOffset {
		t.Fatalf("expected details preview scroll unchanged, before=%d after=%d", prevOffset, m2.viewport.YOffset())
	}
}

func TestMouseWheelOverPreviewScrollsPreview(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 3)
	lines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}
	content := ""
	for i, l := range lines {
		if i > 0 {
			content += "\n"
		}
		content += l
	}
	m.viewport.SetContent(content)

	rect := m.previewRect()
	prevCursor := m.table.Cursor()
	prevOffset := m.viewport.YOffset()

	updated, _ := m.Update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 2, Button: tea.MouseWheelDown})
	m2 := updated.(Model)

	if m2.viewport.YOffset() <= prevOffset {
		t.Fatalf("expected details preview to scroll down, before=%d after=%d", prevOffset, m2.viewport.YOffset())
	}
	if m2.table.Cursor() != prevCursor {
		t.Fatalf("expected table cursor unchanged, before=%d after=%d", prevCursor, m2.table.Cursor())
	}
}

func TestMouseWheelOverRegionWithNoOverflowNoOps(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 1)

	tableRect := m.tableRect()
	prevCursor := m.table.Cursor()
	updated, _ := m.Update(tea.MouseWheelMsg{X: tableRect.X + 2, Y: tableRect.Y + 2, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.table.Cursor() != prevCursor {
		t.Fatalf("expected table cursor unchanged with a single row, before=%d after=%d", prevCursor, m.table.Cursor())
	}
	if m.previewLoading {
		t.Fatalf("expected no preview reload when the table has nothing to scroll")
	}

	previewRect := m.previewRect()
	prevOffset := m.viewport.YOffset()
	updated, _ = m.Update(tea.MouseWheelMsg{X: previewRect.X + 2, Y: previewRect.Y + 2, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.viewport.YOffset() != prevOffset {
		t.Fatalf("expected details preview scroll unchanged when content doesn't overflow, before=%d after=%d", prevOffset, m.viewport.YOffset())
	}
}

func TestMouseWheelWithHelpOpenScrollsHelpNotTableOrPreview(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 20)
	m.width = 80
	m.height = 20
	m = m.resized()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if m.mode != modeHelp {
		t.Fatalf("expected modeHelp after '?', got %v", m.mode)
	}

	prevCursor := m.table.Cursor()
	prevOffset := m.viewport.YOffset()
	rect := m.tableRect()
	updated, _ = m.Update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 2, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.table.Cursor() != prevCursor {
		t.Fatalf("expected table cursor unchanged while help is open, before=%d after=%d", prevCursor, m.table.Cursor())
	}
	if m.viewport.YOffset() != prevOffset {
		t.Fatalf("expected details preview scroll unchanged while help is open, before=%d after=%d", prevOffset, m.viewport.YOffset())
	}
	if m.helpModel.Viewport.YOffset() == 0 {
		t.Fatal("expected wheel-down to scroll the help modal while it's open")
	}
}

func longContent(nLines int) string {
	return strings.TrimRight(strings.Repeat("line\n", nLines), "\n")
}

func TestMouseWheelWithLogsOpenScrollsLogsNotTableOrPreview(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 20)
	m.lastJobLog = longContent(80)
	m = m.enterLogsMode()

	prevCursor := m.table.Cursor()
	prevOffset := m.viewport.YOffset()
	rect := m.tableRect()
	updated, _ := m.Update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 2, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.table.Cursor() != prevCursor {
		t.Fatalf("expected table cursor unchanged while logs modal is open, before=%d after=%d", prevCursor, m.table.Cursor())
	}
	if m.viewport.YOffset() != prevOffset {
		t.Fatalf("expected details preview scroll unchanged while logs modal is open, before=%d after=%d", prevOffset, m.viewport.YOffset())
	}
	if m.logsViewport.YOffset() == 0 {
		t.Fatal("expected wheel-down to scroll the logs modal while it's open")
	}
}

func TestMouseWheelWithErrorOpenScrollsErrorNotTableOrPreview(t *testing.T) {
	t.Parallel()
	m := newTestModelWithWorktrees(t, 20)
	m = m.showError(longContent(80))

	prevCursor := m.table.Cursor()
	prevOffset := m.viewport.YOffset()
	rect := m.tableRect()
	updated, _ := m.Update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 2, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.table.Cursor() != prevCursor {
		t.Fatalf("expected table cursor unchanged while error modal is open, before=%d after=%d", prevCursor, m.table.Cursor())
	}
	if m.viewport.YOffset() != prevOffset {
		t.Fatalf("expected details preview scroll unchanged while error modal is open, before=%d after=%d", prevOffset, m.viewport.YOffset())
	}
	if m.errorViewport.YOffset() == 0 {
		t.Fatal("expected wheel-down to scroll the error modal while it's open")
	}
}
