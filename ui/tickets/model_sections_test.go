package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestNewModel_RendersEmptyStateWithNoScratchDir(t *testing.T) {
	t.Parallel()
	m := NewModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "no .scratch/ directory found") {
		t.Fatalf("expected empty-state message in view, got:\n%s", content)
	}
	if !strings.Contains(content, "no ticket selected") {
		t.Fatalf("expected preview placeholder in view, got:\n%s", content)
	}
}

func TestModel_StackedLayoutSplitsAvailableHeightEvenly(t *testing.T) {
	t.Parallel()
	m := Model{width: 80, height: 29}
	sidebarH, previewH := m.layout().splitHeight(m.layout().contentHeight())
	if sidebarH+previewH != 28 { // 29 minus the seam row
		t.Errorf("sidebarH(%d) + previewH(%d) != 28", sidebarH, previewH)
	}
	if sidebarH != 14 || previewH != 14 {
		t.Errorf("stacked heights = (%d, %d), want (14, 14)", sidebarH, previewH)
	}
}

// TestModel_CmdLoad_CarriesArchivedEpicCountAndRefreshesIt covers the
// plumbing: cmdLoad computes the cheap archive count via
// tickets.CountArchivedEpics so it arrives on epicsLoadedMsg both on initial
// load and again after a subsequent "R" refresh, without the sidebar's
// ArchivedEpics itself ever being populated (that's the lazy load).
func TestModel_CmdLoad_CarriesArchivedEpicCountAndRefreshesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-open-ticket.md", "Status: open\n\nBody.\n")
	archiveDir := filepath.Join(root, ".scratch", ".archive", "old-epic")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	if m.sidebar.ArchivedEpicCount != 1 {
		t.Fatalf("archivedEpicCount after initial load = %d, want 1", m.sidebar.ArchivedEpicCount)
	}

	secondArchiveDir := filepath.Join(root, ".scratch", ".archive", "another-old-epic")
	if err := os.MkdirAll(secondArchiveDir, 0755); err != nil {
		t.Fatal(err)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = updated.(Model)
	m = deliverCmd(t, m, cmd)

	if m.sidebar.ArchivedEpicCount != 2 {
		t.Fatalf("archivedEpicCount after refresh = %d, want 2", m.sidebar.ArchivedEpicCount)
	}
}

func TestNewModel_RendersBeforeSizing(t *testing.T) {
	t.Parallel()
	m := NewModel("/repo", ui.Settings{}, keys.New(nil))
	// Never hidden: the tab must render something even before a WindowSizeMsg
	// arrives.
	content := m.View().Content
	if strings.TrimSpace(content) == "" {
		t.Fatal("expected non-empty view even before sizing")
	}
}
