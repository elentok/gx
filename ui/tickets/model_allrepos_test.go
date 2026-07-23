package tickets

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// writeTicketAt writes a ticket file under worktreeRoot/.scratch/epic/issues,
// mirroring writeTicket but for a specific worktree root rather than always
// t.TempDir().
func writeTicketAt(t *testing.T, worktreeRoot, epic, filename, content string) {
	t.Helper()
	writeTicket(t, worktreeRoot, epic, filename, content)
}

func TestNewModelWithScope_AllReposAggregatesAcrossWorktrees(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtA := filepath.Join(repoDir, "feature-a")
	wtB := filepath.Join(repoDir, "feature-b")

	writeTicketAt(t, wtA, "epic-a", "01-first.md", "Status: open\n\nBody.\n")
	writeTicketAt(t, wtB, "epic-b", "01-second.md", "Status: open\n\nBody.\n")

	m := NewModelWithScope(wtA, ui.Settings{}, keys.New(nil), true)
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "feature-a") {
		t.Fatalf("expected feature-a worktree tag, got:\n%s", content)
	}
	if !strings.Contains(content, "feature-b") {
		t.Fatalf("expected feature-b worktree tag, got:\n%s", content)
	}
	if !strings.Contains(content, "epic-a") || !strings.Contains(content, "epic-b") {
		t.Fatalf("expected both epics rendered, got:\n%s", content)
	}
}

func TestNewModelWithScope_AllReposNavigationCoversAllEpics(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtA := filepath.Join(repoDir, "feature-a")
	wtB := filepath.Join(repoDir, "feature-b")

	writeTicketAt(t, wtA, "epic-a", "01-first.md", "Status: open\n\nBody.\n")
	writeTicketAt(t, wtB, "epic-b", "01-second.md", "Status: open\n\nBody.\n")

	m := NewModelWithScope(wtA, ui.Settings{}, keys.New(nil), true)
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	rows := m.visibleRows()
	// 2 epics x (1 epic row + 1 ticket row) = 4 selectable rows, interleaved
	// into the tab's normal single Open/Closed grouping (no per-worktree
	// header rows).
	if len(rows) != 4 {
		t.Fatalf("expected 4 selectable rows (2 epics + 2 tickets), got %d", len(rows))
	}
	epicRows := 0
	for _, r := range rows {
		if r.isEpic() {
			epicRows++
		}
	}
	if epicRows != 2 {
		t.Fatalf("expected 2 epic rows among visibleRows(), got %d", epicRows)
	}
}

func TestNewModel_NonAllModeIgnoresOtherWorktrees(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtA := filepath.Join(repoDir, "feature-a")
	wtB := filepath.Join(repoDir, "feature-b")

	writeTicketAt(t, wtA, "epic-a", "01-first.md", "Status: open\n\nBody.\n")
	writeTicketAt(t, wtB, "epic-b", "01-second.md", "Status: open\n\nBody.\n")

	m := NewModel(wtA, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "epic-a") {
		t.Fatalf("expected epic-a rendered, got:\n%s", content)
	}
	if strings.Contains(content, "epic-b") {
		t.Fatalf("expected epic-b (other worktree) NOT rendered without --all, got:\n%s", content)
	}
}
