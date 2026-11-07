package sidebar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tree"
)

// TestArchivedSectionShowsCountWithoutLoading: the Archived header renders
// "Archived epics (N)" straight from the cheap count, with no load triggered
// (and therefore no ArchivedEpics populated) until the section is expanded.
func TestArchivedSectionShowsCountWithoutLoading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-open-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	if !strings.Contains(view(s), "Archived epics (1)") {
		t.Fatalf("expected archived section header with count, got:\n%s", view(s))
	}
	if s.ArchivedLazy.State() != tree.LazyIdle {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyIdle before expand", s.ArchivedLazy.State())
	}
	if len(s.ArchivedEpics) != 0 {
		t.Fatalf("expected ArchivedEpics unpopulated before expand, got %v", s.ArchivedEpics)
	}
}

// TestExpandArchivedSectionLoadsAndPopulatesCollapsedByDefault drives the full
// lazy-load path: expanding the Archived header with a non-zero count returns
// a load command; delivering it populates ArchivedEpics and renders the
// archived epic row, collapsed by default.
func TestExpandArchivedSectionLoadsAndPopulatesCollapsedByDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	cmd := toggleArchivedHeader(t, &s)
	if cmd == nil {
		t.Fatal("expected expand of the Archived section to return a load command")
	}
	if s.ArchivedLazy.State() != tree.LazyLoading {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyLoading immediately after expand", s.ArchivedLazy.State())
	}

	deliverCmd(&s, cmd)

	if s.ArchivedLazy.State() != tree.LazyLoaded {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyLoaded", s.ArchivedLazy.State())
	}
	if len(s.ArchivedEpics) != 1 || s.ArchivedEpics[0].Name != "old-epic" {
		t.Fatalf("ArchivedEpics = %+v, want one epic named old-epic", s.ArchivedEpics)
	}
	if !strings.Contains(view(s), "old-epic") {
		t.Fatalf("expected archived epic row in view, got:\n%s", view(s))
	}
	if strings.Contains(view(s), "Old ticket") {
		t.Fatalf("expected archived epic to render collapsed by default, got:\n%s", view(s))
	}
}

// TestFailedArchivedLoadRendersInlineErrorAndStaysExpandedThenRetries: a
// failed load renders inline error text in place of "loading…" with the
// section still expanded, and a subsequent collapse+re-expand retries the
// load (LazySection.Expand only no-ops on Loading/Loaded, not Failed).
func TestFailedArchivedLoadRendersInlineErrorAndStaysExpandedThenRetries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")
	// Replace the archive directory with a file so tickets.LoadArchived's
	// os.ReadDir call fails, forcing LazyResultMsg.Err on the load.
	archiveDir := filepath.Join(scratchOf(root), ".archive")
	if err := os.RemoveAll(archiveDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archiveDir, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(scratchOf(root), "my-epic"))
	s := loadedSidebar(t, root)
	s.ArchivedEpicCount = 1 // bypass the count-0 short-circuit; the count itself tolerates the file
	s.ClampSelected(testEnv())

	deliverCmd(&s, toggleArchivedHeader(t, &s))

	if s.ArchivedLazy.State() != tree.LazyFailed {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyFailed", s.ArchivedLazy.State())
	}
	if body := view(s); !strings.Contains(body, "failed to load") {
		t.Fatalf("expected inline error text, got:\n%s", body)
	}
	if s.Tree.CollapsedIDs()[SectionID(SectionArchived)] {
		t.Fatal("expected Archived section to stay expanded after a failed load")
	}

	toggleArchivedHeader(t, &s) // collapse
	if cmd := toggleArchivedHeader(t, &s); cmd == nil {
		t.Fatal("expected re-expand after a failed load to return a load command again")
	}
}

// TestArchivedCountChangeInvalidatesLazyCacheOnNextExpand covers
// LazySection.SetCount's auto-invalidation: a refresh that changes the count
// after the section was already loaded must force the next expand to re-scan
// rather than reuse the stale cache.
func TestArchivedCountChangeInvalidatesLazyCacheOnNextExpand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	deliverCmd(&s, toggleArchivedHeader(t, &s))
	if s.ArchivedLazy.State() != tree.LazyLoaded {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyLoaded before the count change", s.ArchivedLazy.State())
	}

	tickettest.WriteArchivedTicket(t, root, "another-old-epic", "01-ticket.md", "Status: done\n\nBody.\n")
	reload(t, &s, root, false)

	if s.ArchivedEpicCount != 2 {
		t.Fatalf("ArchivedEpicCount after refresh = %d, want 2", s.ArchivedEpicCount)
	}
	if s.ArchivedLazy.State() != tree.LazyIdle {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyIdle after a count change invalidates the cache", s.ArchivedLazy.State())
	}

	toggleArchivedHeader(t, &s) // collapse
	if cmd := toggleArchivedHeader(t, &s); cmd == nil {
		t.Fatal("expected expand after a count change to return a load command again")
	}
}
