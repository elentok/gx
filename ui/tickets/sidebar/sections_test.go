package sidebar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

func TestRendersEpicsAndTicketsFromDisk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if !strings.Contains(content, "my-epic") || !strings.Contains(content, "(1 done / 2)") {
		t.Fatalf("expected epic row with name + (1 done / 2) count, got:\n%s", content)
	}
	if !strings.Contains(content, "First ticket") || !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected ticket titles in view, got:\n%s", content)
	}
}

func TestRendersAlphabeticallySuffixedTicketNumber(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "10a-split-ticket.md", "Status: open\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if !strings.Contains(content, "10a Split ticket") {
		t.Fatalf("expected suffixed ticket identifier in view, got:\n%s", content)
	}
}

func TestZeroEpicScratchDirRendersNoScratchEmptyState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(scratchOf(root), 0755); err != nil {
		t.Fatal(err)
	}

	content := view(loadedSidebar(t, root))

	if !strings.Contains(content, "no .scratch/ directory found") {
		t.Fatalf("expected empty-state message for zero-epic .scratch/, got:\n%s", content)
	}
}

// TestArchivedEpicCountSuppressesEmptyState: a non-zero ArchivedEpicCount
// means the Archived section still has something to render, so an empty
// active .scratch/ alone must not render the "no .scratch/ directory found"
// short-circuit.
func TestArchivedEpicCountSuppressesEmptyState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(scratchOf(root), 0755); err != nil {
		t.Fatal(err)
	}
	s := loadedSidebar(t, root)
	s.ArchivedEpicCount = 1

	if body := view(s); strings.Contains(body, "no .scratch/ directory found") {
		t.Fatalf("expected ArchivedEpicCount>0 to suppress empty state, got:\n%s", body)
	}
}

// TestTicketsInPlanOrderWithinEpic guards against re-grouping tickets by
// rendered status: they render in plan order (ticket number ascending) so a
// ticket never jumps position once it's done.
func TestTicketsInPlanOrderWithinEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-done-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-open-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "03-paused-answer-ticket.md", "Status: paused\nPause reason: blocked-pane\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "04-blocked-ticket.md", "Status: open\nBlocked by: 02\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	assertInOrder(t, content, []string{"Done ticket", "Open ticket", "Paused answer ticket", "Blocked ticket"})
}

// Reading the store itself (no server), the blocked overlay is not computed:
// the row shows the file's own status.
func TestBlockedTicketShowsRawStatusWithoutServer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected no blocked-by suffix without a server, got:\n%s", content)
	}
}

func TestPausedAnswerTicketShowsUnresolvedBlockerSuffix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-paused-answer-ticket.md", "Status: paused\nPause reason: blocked-pane\nBlocked by: 01\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix on paused (answer) ticket, got:\n%s", content)
	}
}

func TestResolvedBlockerDropsSuffixAndRegroups(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-formerly-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if strings.Contains(content, "blocked by") {
		t.Fatalf("expected no blocked-by suffix once blocker is done, got:\n%s", content)
	}
}

func TestUnrecognizedStatusRendersAsError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-bogus-status-ticket.md", "Status: bogus-value\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if !strings.Contains(content, ui.Icons(false).TicketError) {
		t.Fatalf("expected error icon %q in view, got:\n%s", ui.Icons(false).TicketError, content)
	}
}

func TestFullyDoneEpicStartsCollapsedAndDimmed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "done-epic", "02-second-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	content := view(loadedSidebar(t, root))

	if strings.Contains(content, "First ticket") || strings.Contains(content, "Second ticket") {
		t.Fatalf("expected done-epic's tickets hidden by default collapse, got:\n%s", content)
	}
}

func TestReloadPreservesManualCollapseToggle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	s := loadedSidebar(t, root)

	// Manually collapse open-epic, which loadedSidebar expanded.
	setCollapsedEpic(&s, indexOfEpic(t, s, "open-epic"), true)
	if !s.IsCollapsed(findEpic(t, s, "open-epic")) {
		t.Fatalf("expected open-epic collapsed after manual toggle")
	}

	// A new epic appears (the auto-refresh poll picking up fresh disk state):
	// it gets its correct default while the manual toggle survives.
	tickettest.WriteTicket(t, root, "new-done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	reload(t, &s, root, false)

	if !s.IsCollapsed(findEpic(t, s, "open-epic")) {
		t.Fatalf("expected manually-collapsed open-epic to stay collapsed after a reload")
	}
	if !s.IsCollapsed(findEpic(t, s, "new-done-epic")) {
		t.Fatalf("expected new fully-done epic to default to collapsed")
	}
	// ...and its ticket is hidden either way, because the Closed section
	// itself starts collapsed.
	if content := view(s); strings.Contains(content, "First ticket") {
		t.Fatalf("expected new-done-epic's ticket hidden by the default-collapsed Closed section, got:\n%s", content)
	}
}

// TestExpandedClosedSectionSurvivesRepeatedRefresh: expanding "Complete epics
// (unmerged)" used to snap shut again on the very next auto-refresh poll,
// because expanding deleted the collapse-map entry instead of recording an
// explicit choice, so the section's collapsed-by-default policy re-asserted
// itself. Assert it survives more than one tick.
func TestExpandedClosedSectionSurvivesRepeatedRefresh(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	if !s.Tree.CollapsedIDs()[SectionID(SectionComplete)] {
		t.Fatalf("expected Closed section to start collapsed")
	}

	setCollapsedSection(&s, SectionComplete, false)
	if s.Tree.CollapsedIDs()[SectionID(SectionComplete)] {
		t.Fatalf("expected Closed section expanded after manual toggle")
	}
	// done-epic itself still defaults to collapsed once the section is
	// expanded — expand it explicitly too, since this test is about the
	// section toggle's durability, not the per-epic default.
	setCollapsedEpic(&s, indexOfEpic(t, s, "done-epic"), false)

	for i := range 3 {
		reload(t, &s, root, false)

		if s.Tree.CollapsedIDs()[SectionID(SectionComplete)] {
			t.Fatalf("refresh %d: expected Closed section to stay expanded, got collapsed", i+1)
		}
		if content := view(s); !strings.Contains(content, "Only ticket") {
			t.Fatalf("refresh %d: expected Closed section's ticket visible, got:\n%s", i+1, content)
		}
	}
}

func TestGroupsEpicsIntoSectionsInOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	testutil.Mkdir(t, filepath.Join(root, ".scratch", "empty-epic", "issues"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(root, ".scratch", "empty-epic"))
	tickettest.WriteMap(t, root, "open-map", "")
	tickettest.WriteTicket(t, root, "open-map", "01-only-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "complete-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")
	writeMergedEpic(t, root, "merged-epic")
	tickettest.WriteTicket(t, root, "merged-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteMap(t, root, "complete-map", "")
	tickettest.WriteTicket(t, root, "complete-map", "01-only-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteArchivedTicket(t, root, "archived-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	content := ansi.Strip(view(s))
	assertInOrder(t, content, []string{
		"Open epics (2)", "empty-epic", "open-epic",
		"Open maps (1)", "open-map",
		"Complete epics (unmerged) (1)",
		"Merged epics (1)",
		"Complete maps (1)",
		"Archived epics (1)",
	})
	if strings.Contains(content, "Closed epics") {
		t.Fatalf("expected no 'Closed epics' section, got:\n%s", content)
	}

	// The complete sections start collapsed; each expands and collapses again.
	for _, c := range []struct {
		section Section
		epic    string
	}{
		{SectionComplete, "complete-epic"},
		{SectionMerged, "merged-epic"},
		{SectionCompleteMaps, "complete-map"},
	} {
		if strings.Contains(content, c.epic) {
			t.Fatalf("expected %s hidden under its collapsed section, got:\n%s", c.epic, content)
		}
		setCollapsedSection(&s, c.section, false)
		if !strings.Contains(ansi.Strip(view(s)), c.epic) {
			t.Fatalf("expected %s visible once its section expands, got:\n%s", c.epic, view(s))
		}
		setCollapsedSection(&s, c.section, true)
		if strings.Contains(ansi.Strip(view(s)), c.epic) {
			t.Fatalf("expected %s hidden once its section collapses again", c.epic)
		}
	}
}

// assertInOrder fails unless every needle appears in content, each after the
// previous one.
func assertInOrder(t *testing.T, content string, needles []string) {
	t.Helper()
	pos := 0
	for _, needle := range needles {
		i := strings.Index(content[pos:], needle)
		if i < 0 {
			t.Fatalf("expected %q after offset %d, got:\n%s", needle, pos, content)
		}
		pos += i + len(needle)
	}
}

func writeMergedEpic(t *testing.T, root, epic string) {
	t.Helper()
	epicDir := filepath.Join(root, ".scratch", epic)
	testutil.Mkdir(t, epicDir)
	testutil.WriteFile(t, epicDir, "ticket.md", "---\nstatus: open\nmerged_at: 2026-01-02T00:00:00Z\n---\n")
}

// TestSectionHeadersMatchTreeRowShape: section headers use the same
// expand-glyph-led shape epic/ticket rows use, with no state icon on either
// header — "Complete epics" matches "Open epics" — and no blank spacer row
// between the two sections.
func TestSectionHeadersMatchTreeRowShape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	s := loadedSidebar(t, root)
	setCollapsedSection(&s, SectionComplete, false)

	content := ansi.Strip(view(s))
	if strings.Contains(content, "──") {
		t.Fatalf("expected the decorative border to be gone, got:\n%s", content)
	}

	icons := ui.Icons(false)
	for _, header := range []string{"Open epics (1)", "Complete epics (unmerged) (1)"} {
		prefix := prefixBefore(t, content, header)
		if !strings.Contains(prefix, icons.TriangleExpanded) {
			t.Fatalf("expected %q row to lead with the expand glyph, got: %q", header, prefix)
		}
		if strings.Contains(prefix, icons.TicketDone) {
			t.Fatalf("expected %q row to show no icon, got: %q", header, prefix)
		}
	}

	lines := strings.Split(content, "\n")
	closedHeaderLine := -1
	for i, line := range lines {
		if strings.Contains(line, "Complete epics (unmerged) (1)") {
			closedHeaderLine = i
			break
		}
	}
	if closedHeaderLine <= 0 {
		t.Fatalf("expected to find the 'Complete epics (unmerged)' header past line 0, got:\n%s", content)
	}
	if strings.TrimSpace(lines[closedHeaderLine-1]) == "" {
		t.Fatalf("expected no blank spacer row directly above the 'Complete epics (unmerged)' header, got:\n%s", content)
	}
}

// prefixBefore returns the part of the single line containing needle that
// precedes it — the row's own glyph/icon column. Fails the test unless needle
// is on exactly one line.
func prefixBefore(t *testing.T, content, needle string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, needle) {
			found = append(found, line[:strings.Index(line, needle)])
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one line containing %q, got %d:\n%s", needle, len(found), content)
	}
	return found[0]
}

func TestHidesEmptySections(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	content := ansi.Strip(view(loadedSidebar(t, root)))

	if !strings.Contains(content, "Open epics (1)") {
		t.Fatalf("expected 'Open epics (1)' header, got:\n%s", content)
	}
	for _, label := range []string{"Open maps", "Complete epics", "Merged epics", "Complete maps", "Archived epics"} {
		if strings.Contains(content, label) {
			t.Fatalf("expected empty %q section hidden, got:\n%s", label, content)
		}
	}
}

func TestDimmingTracksAllDoneNotCollapseState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	s := loadedSidebar(t, root)

	dimPrefix := strings.SplitN(rowfmt.StatusDoneStyle.Render("PROBE"), "PROBE", 2)[0]
	doneEpic := findEpic(t, s, "done-epic")
	openEpic := findEpic(t, s, "open-epic")

	// done-epic starts collapsed by default: expand it and confirm it's
	// still dimmed (dimming tracks AllDone(), not the collapse toggle).
	setCollapsedEpic(&s, indexOfEpic(t, s, "done-epic"), false)
	if got := s.RenderEpicRow(testEnv(), doneEpic); !strings.Contains(got, dimPrefix) {
		t.Fatalf("expected done-epic to stay dimmed after manual expand, got: %q", got)
	}

	// open-epic starts expanded: collapse it and confirm it does NOT
	// become dimmed just because it's collapsed.
	setCollapsedEpic(&s, indexOfEpic(t, s, "open-epic"), true)
	if got := s.RenderEpicRow(testEnv(), openEpic); strings.Contains(got, dimPrefix) {
		t.Fatalf("expected open-epic to stay undimmed after manual collapse, got: %q", got)
	}
}
