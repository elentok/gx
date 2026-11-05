package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/sidebar"
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

func TestNewModel_RendersEpicsAndTicketsFromDisk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "my-epic") || !strings.Contains(content, "(1 done / 2)") {
		t.Fatalf("expected epic row with name + (1 done / 2) count, got:\n%s", content)
	}
	if !strings.Contains(content, "First ticket") || !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected ticket titles in view, got:\n%s", content)
	}
}

func TestNewModel_RendersAlphabeticallySuffixedTicketNumber(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "10a-split-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "10a Split ticket") {
		t.Fatalf("expected suffixed ticket identifier in view, got:\n%s", m.View().Content)
	}
}

func TestModel_StackedLayoutSplitsAvailableHeightEvenly(t *testing.T) {
	t.Parallel()
	m := Model{width: 80, height: 29}
	sidebarH, previewH := m.splitHeight(m.contentHeight())
	if sidebarH+previewH != 28 { // 29 minus the seam row
		t.Errorf("sidebarH(%d) + previewH(%d) != 28", sidebarH, previewH)
	}
	if sidebarH != 14 || previewH != 14 {
		t.Errorf("stacked heights = (%d, %d), want (14, 14)", sidebarH, previewH)
	}
}

func TestNewModel_ZeroEpicScratchDirRendersSameEmptyStateAsNoScratchDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".scratch"), 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "no .scratch/ directory found") {
		t.Fatalf("expected empty-state message for zero-epic .scratch/, got:\n%s", content)
	}
}

// TestModel_ArchivedEpicCountSuppressesEmptyState covers ticket 03's guard:
// a non-zero archivedEpicCount means the Archived section still has
// something to render, so an empty active .scratch/ alone must not render
// the "no .scratch/ directory found" short-circuit.
func TestModel_ArchivedEpicCountSuppressesEmptyState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".scratch"), 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	m.sidebar.ArchivedEpicCount = 1

	body := strings.Join(m.sidebar.Body(sidebarEnvFor(m), 20, 80), "\n")
	if strings.Contains(body, "no .scratch/ directory found") {
		t.Fatalf("expected archivedEpicCount>0 to suppress empty state, got:\n%s", body)
	}
}

// TestModel_CmdLoad_CarriesArchivedEpicCountAndRefreshesIt covers ticket
// 03's plumbing: cmdLoad computes the cheap archive count via
// tickets.CountArchivedEpics so it arrives on epicsLoadedMsg both on initial
// load and again after a subsequent "R" refresh, without m.archivedEpics
// itself ever being populated (that's ticket 04's lazy load).
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

// TestNewModel_TicketsInPlanOrderWithinEpic guards against re-grouping
// tickets by rendered status: they render in plan order (ticket number
// ascending) so a ticket never jumps position once it's done, regardless of
// status — see sortedTicketIndexes.
func TestNewModel_TicketsInPlanOrderWithinEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-done-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-open-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "03-paused-answer-ticket.md", "Status: paused\nPause reason: blocked-pane\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "04-blocked-ticket.md", "Status: open\nBlocked by: 02\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	wantOrder := []string{"Done ticket", "Open ticket", "Paused answer ticket", "Blocked ticket"}
	lastIdx := -1
	for _, title := range wantOrder {
		idx := strings.Index(content, title)
		if idx == -1 {
			t.Fatalf("expected %q in view, got:\n%s", title, content)
		}
		if idx < lastIdx {
			t.Fatalf("expected %q to render in plan (ticket number) order, got:\n%s", title, content)
		}
		lastIdx = idx
	}
}

// Reading the store itself (no server), the blocked overlay is not computed:
// the row shows the file's own status.
func TestNewModel_BlockedTicketShowsRawStatusWithoutServer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected no blocked-by suffix without a server, got:\n%s", content)
	}
}

func TestNewModel_PausedAnswerTicketShowsUnresolvedBlockerSuffix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-paused-answer-ticket.md", "Status: paused\nPause reason: blocked-pane\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix on paused (answer) ticket, got:\n%s", content)
	}
}

func TestNewModel_ResolvedBlockerDropsSuffixAndRegroups(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-formerly-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if strings.Contains(content, "blocked by") {
		t.Fatalf("expected no blocked-by suffix once blocker is done, got:\n%s", content)
	}
}

func TestNewModel_UnrecognizedStatusRendersAsError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-bogus-status-ticket.md", "Status: bogus-value\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, ui.Icons(false).TicketError) {
		t.Fatalf("expected error icon %q in view, got:\n%s", ui.Icons(false).TicketError, content)
	}
}

func TestNewModel_FullyDoneEpicStartsCollapsedAndDimmed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "done-epic", "02-second-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if strings.Contains(content, "First ticket") || strings.Contains(content, "Second ticket") {
		t.Fatalf("expected done-epic's tickets hidden by default collapse, got:\n%s", content)
	}
}

func TestModel_EpicsLoadedMsgPreservesManualCollapseToggle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Manually collapse open-epic, which deliverLoad expanded.
	setCollapsedEpic(&m, indexOfEpic(t, m, "open-epic"), true)
	if !m.sidebar.IsCollapsed(findEpic(t, m, "open-epic")) {
		t.Fatalf("expected open-epic collapsed after manual toggle")
	}

	// A new epic appears (simulating the auto-refresh poll picking up fresh
	// disk state) — it should get its correct default while the manual
	// toggle above survives.
	tickettest.WriteTicket(t, root, "new-done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")

	msg := m.cmdLoad()()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if !m.sidebar.IsCollapsed(findEpic(t, m, "open-epic")) {
		t.Fatalf("expected manually-collapsed open-epic to stay collapsed after epicsLoadedMsg")
	}
	// A newly-closed epic now gets its own collapsed default (ticket 02),
	// on top of the Closed section itself also starting collapsed.
	if !m.sidebar.IsCollapsed(findEpic(t, m, "new-done-epic")) {
		t.Fatalf("expected new fully-done epic to default to collapsed")
	}
	// ...and its ticket is hidden either way, because the Closed section
	// itself starts collapsed.
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected new-done-epic's ticket hidden by the default-collapsed Closed section, got:\n%s", m.View().Content)
	}
}

// TestModel_ExpandedClosedSectionSurvivesRepeatedRefresh is ticket 01's
// regression: expanding "Complete epics (unmerged)" used to snap shut again on the very
// next auto-refresh poll, because expanding deleted the collapse-map entry
// instead of recording an explicit choice, so the section's collapsed-by-
// default policy re-asserted itself. Assert it survives more than one tick.
func TestModel_ExpandedClosedSectionSurvivesRepeatedRefresh(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section to start collapsed")
	}

	setCollapsedSection(&m, sidebar.SectionComplete, false)
	if m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section expanded after manual toggle")
	}
	// done-epic itself still defaults to collapsed once the section is
	// expanded (ticket 02) — expand it explicitly too, since this test is
	// about the section toggle's durability, not the per-epic default.
	setCollapsedEpic(&m, indexOfEpic(t, m, "done-epic"), false)

	for i := range 3 {
		msg := m.cmdLoad()()
		updated, _ = m.Update(msg)
		m = updated.(Model)

		if m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
			t.Fatalf("refresh %d: expected Closed section to stay expanded, got collapsed", i+1)
		}
		if !strings.Contains(m.View().Content, "Only ticket") {
			t.Fatalf("refresh %d: expected Closed section's ticket visible, got:\n%s", i+1, m.View().Content)
		}
	}
}

func TestNewModel_GroupsEpicsIntoSectionsInOrder(t *testing.T) {
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

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	want := []string{
		"Open epics (2)", "empty-epic", "open-epic",
		"Open maps (1)", "open-map",
		"Complete epics (unmerged) (1)",
		"Merged epics (1)",
		"Complete maps (1)",
		"Archived epics (1)",
	}
	assertInOrder(t, content, want)
	if strings.Contains(content, "Closed epics") {
		t.Fatalf("expected no 'Closed epics' section, got:\n%s", content)
	}

	// The complete sections start collapsed; each expands and collapses again.
	for _, c := range []struct {
		section sidebar.Section
		epic    string
	}{
		{sidebar.SectionComplete, "complete-epic"},
		{sidebar.SectionMerged, "merged-epic"},
		{sidebar.SectionCompleteMaps, "complete-map"},
	} {
		if strings.Contains(content, c.epic) {
			t.Fatalf("expected %s hidden under its collapsed section, got:\n%s", c.epic, content)
		}
		setCollapsedSection(&m, c.section, false)
		if !strings.Contains(ansi.Strip(m.View().Content), c.epic) {
			t.Fatalf("expected %s visible once its section expands, got:\n%s", c.epic, m.View().Content)
		}
		setCollapsedSection(&m, c.section, true)
		if strings.Contains(ansi.Strip(m.View().Content), c.epic) {
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

// TestNewModel_SectionHeadersMatchTreeRowShape covers ticket 02's redesign:
// section headers drop the old "── … ──" border in favor of the same
// expand-glyph-led shape epic/ticket rows use, with no state icon on either
// header — "Complete epics" matches "Open epics" — and the blank spacer row
// that used to sit between the two sections is gone.
func TestNewModel_SectionHeadersMatchTreeRowShape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sidebar.SectionComplete, false)

	rawContent := m.View().Content
	content := ansi.Strip(rawContent)
	if strings.Contains(content, "──") {
		t.Fatalf("expected the decorative border to be gone, got:\n%s", content)
	}

	icons := m.icons()
	openPrefix := prefixBefore(t, content, "Open epics (1)")
	if !strings.Contains(openPrefix, icons.TriangleExpanded) {
		t.Fatalf("expected 'Open epics' row to lead with the expand glyph, got: %q", openPrefix)
	}
	if strings.Contains(openPrefix, icons.TicketDone) {
		t.Fatalf("expected 'Open epics' row to show no icon, got: %q", openPrefix)
	}

	closedPrefix := prefixBefore(t, content, "Complete epics (unmerged) (1)")
	if !strings.Contains(closedPrefix, icons.TriangleExpanded) {
		t.Fatalf("expected 'Complete epics (unmerged)' row to lead with the expand glyph, got: %q", closedPrefix)
	}
	if strings.Contains(closedPrefix, icons.TicketDone) {
		t.Fatalf("expected 'Complete epics (unmerged)' row to show no icon, got: %q", closedPrefix)
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

// prefixBefore returns the portion of the single line in content containing
// needle that precedes it — i.e. the row's own glyph/icon column, excluding
// whatever the split-pane preview happens to render on the same physical
// line. Fails the test if needle isn't found on exactly one line.
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

func TestNewModel_HidesEmptySections(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Open epics (1)") {
		t.Fatalf("expected 'Open epics (1)' header, got:\n%s", content)
	}
	for _, label := range []string{"Open maps", "Complete epics", "Merged epics", "Complete maps", "Archived epics"} {
		if strings.Contains(content, label) {
			t.Fatalf("expected empty %q section hidden, got:\n%s", label, content)
		}
	}
}

func TestModel_DimmingTracksAllDoneNotCollapseState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	dimPrefix := strings.SplitN(rowfmt.StatusDoneStyle.Render("PROBE"), "PROBE", 2)[0]

	doneEpic := findEpic(t, m, "done-epic")
	openEpic := findEpic(t, m, "open-epic")

	// done-epic starts collapsed by default: expand it and confirm it's
	// still dimmed (dimming tracks AllDone(), not the collapse toggle).
	setCollapsedEpic(&m, indexOfEpic(t, m, "done-epic"), false)
	if !strings.Contains(m.sidebar.RenderEpicRow(sidebarEnvFor(m), doneEpic), dimPrefix) {
		t.Fatalf("expected done-epic to stay dimmed after manual expand, got: %q", m.sidebar.RenderEpicRow(sidebarEnvFor(m), doneEpic))
	}

	// open-epic starts expanded: collapse it and confirm it does NOT
	// become dimmed just because it's collapsed.
	setCollapsedEpic(&m, indexOfEpic(t, m, "open-epic"), true)
	if strings.Contains(m.sidebar.RenderEpicRow(sidebarEnvFor(m), openEpic), dimPrefix) {
		t.Fatalf("expected open-epic to stay undimmed after manual collapse, got: %q", m.sidebar.RenderEpicRow(sidebarEnvFor(m), openEpic))
	}
}

func TestNewModel_RendersBeforeSizing(t *testing.T) {
	t.Parallel()
	m := NewModel("/repo", ui.Settings{}, keys.New(nil))
	// Never hidden: the tab must render something even before a WindowSizeMsg
	// arrives (mirrors the "reachable and visually present" acceptance
	// criterion from ticket 01).
	content := m.View().Content
	if strings.TrimSpace(content) == "" {
		t.Fatal("expected non-empty view even before sizing")
	}
}
