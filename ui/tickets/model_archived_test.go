package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tree"
)

// TestModel_ArchivedSectionShowsCountWithoutLoading covers ticket 04's
// up-front-count requirement: the Archived header renders "Archived epics
// (N)" straight from ticket 03's cheap count, with no load triggered (and
// therefore no archivedEpics populated) until the section is actually
// expanded.
func TestModel_ArchivedSectionShowsCountWithoutLoading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-open-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "Archived epics (1)") {
		t.Fatalf("expected archived section header with count, got:\n%s", m.View().Content)
	}
	if m.archivedLazy.State() != tree.LazyIdle {
		t.Fatalf("archivedLazy.State() = %v, want LazyIdle before expand", m.archivedLazy.State())
	}
	if len(m.archivedEpics) != 0 {
		t.Fatalf("expected archivedEpics unpopulated before expand, got %v", m.archivedEpics)
	}
}

// TestModel_ExpandArchivedSectionLoadsAndPopulatesCollapsedByDefault drives
// the full lazy-load path: pressing "l" (expand) on the Archived header with
// a non-zero count returns a load command; running it through Update
// populates m.archivedEpics from the result and renders the archived epic
// row, collapsed by default (mirroring ticket 02's closed-epic default).
func TestModel_ExpandArchivedSectionLoadsAndPopulatesCollapsedByDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected expand of the Archived section to return a load command")
	}
	if m.archivedLazy.State() != tree.LazyLoading {
		t.Fatalf("archivedLazy.State() = %v, want LazyLoading immediately after expand", m.archivedLazy.State())
	}

	m = deliverCmd(t, m, cmd)

	if m.archivedLazy.State() != tree.LazyLoaded {
		t.Fatalf("archivedLazy.State() = %v, want LazyLoaded", m.archivedLazy.State())
	}
	if len(m.archivedEpics) != 1 || m.archivedEpics[0].Name != "old-epic" {
		t.Fatalf("archivedEpics = %+v, want one epic named old-epic", m.archivedEpics)
	}
	if !strings.Contains(m.View().Content, "old-epic") {
		t.Fatalf("expected archived epic row in view, got:\n%s", m.View().Content)
	}
	if strings.Contains(m.View().Content, "Old ticket") {
		t.Fatalf("expected archived epic to render collapsed by default, got:\n%s", m.View().Content)
	}
}

// TestModel_FailedArchivedLoadRendersInlineErrorAndStaysExpandedThenRetries
// covers the failure/retry contract: a failed load renders inline error text
// in place of "loading…" with the section still expanded, and a subsequent
// collapse+re-expand retries the load (LazySection.Expand only no-ops on
// Loading/Loaded, not Failed).
func TestModel_FailedArchivedLoadRendersInlineErrorAndStaysExpandedThenRetries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")
	// Replace the archive directory with a file so tickets.LoadArchived's
	// os.ReadDir call fails, forcing LazyResultMsg.Err on the load.
	archiveDir := filepath.Join(root, ".scratch", ".archive")
	if err := os.RemoveAll(archiveDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archiveDir, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	testutil.EnsureEpicTicketMD(t, filepath.Join(root, ".scratch", "my-epic"))
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m.archivedEpicCount = 1 // bypass the count-0 short-circuit; CountArchivedEpics itself tolerates the file fine
	m = deliverLoad(t, m)
	m.archivedEpicCount = 1
	m.clampSelected() // rebuild so the now non-empty Archived section shows
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	m = deliverCmd(t, m, cmd)

	if m.archivedLazy.State() != tree.LazyFailed {
		t.Fatalf("archivedLazy.State() = %v, want LazyFailed", m.archivedLazy.State())
	}
	body := strings.Join(m.sidebarBody(20, 80), "\n")
	if !strings.Contains(body, "failed to load") {
		t.Fatalf("expected inline error text, got:\n%s", body)
	}
	if m.sidebarTree.CollapsedIDs()[sidebarSectionID(sectionArchived)] {
		t.Fatal("expected Archived section to stay expanded after a failed load")
	}

	// Retry: collapse then re-expand.
	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // collapse
	m = updated.(Model)
	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // re-expand
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected re-expand after a failed load to return a load command again")
	}
}

// TestModel_ArchivedCountChangeInvalidatesLazyCacheOnNextExpand covers
// LazySection.SetCount's auto-invalidation contract wired through
// updateInner: a refresh ("R") that changes archivedEpicCount after the
// section was already loaded must force the next expand to re-scan rather
// than reuse the stale cache.
func TestModel_ArchivedCountChangeInvalidatesLazyCacheOnNextExpand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	m = deliverCmd(t, m, cmd)
	if m.archivedLazy.State() != tree.LazyLoaded {
		t.Fatalf("archivedLazy.State() = %v, want LazyLoaded before the count change", m.archivedLazy.State())
	}

	tickettest.WriteArchivedTicket(t, root, "another-old-epic", "01-ticket.md", "Status: done\n\nBody.\n")
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = updated.(Model)
	m = deliverCmd(t, m, cmd)

	if m.archivedEpicCount != 2 {
		t.Fatalf("archivedEpicCount after refresh = %d, want 2", m.archivedEpicCount)
	}
	if m.archivedLazy.State() != tree.LazyIdle {
		t.Fatalf("archivedLazy.State() = %v, want LazyIdle after a count change invalidates the cache", m.archivedLazy.State())
	}

	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // collapse
	m = updated.(Model)
	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // re-expand
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected expand after a count change to return a load command again")
	}
}
