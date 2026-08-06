package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestTicketProgressSpinnerHasEightFramesAtDocumentedCodepoints(t *testing.T) {
	want := []string{
		"\U000F0A9E", "\U000F0A9F", "\U000F0AA0", "\U000F0AA1",
		"\U000F0AA2", "\U000F0AA3", "\U000F0AA4", "\U000F0AA5",
	}
	if len(TicketProgressSpinner.Frames) != 8 {
		t.Fatalf("expected 8 frames, got %d", len(TicketProgressSpinner.Frames))
	}
	for i, frame := range want {
		if TicketProgressSpinner.Frames[i] != frame {
			t.Errorf("frame %d: got %q, want %q", i, TicketProgressSpinner.Frames[i], frame)
		}
	}
}

func TestImplementAgentMenuDefaultsToClaude(t *testing.T) {
	menu := newImplementAgentMenu()
	if menu.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", menu.Cursor)
	}
	if got := menu.Items[menu.Cursor].Value; got != string(ralphloop.AgentClaude) {
		t.Fatalf("default agent = %q, want %q", got, ralphloop.AgentClaude)
	}
}

func TestBuildImplementRunOptionsUsesSelectedAgent(t *testing.T) {
	root := testutil.TempRepo(t)
	opts, err := buildImplementRunOptions(root, "my-epic", ralphloop.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Agent != ralphloop.AgentCodex {
		t.Fatalf("agent = %q, want %q", opts.Agent, ralphloop.AgentCodex)
	}
}

func TestBuildImplementRunOptionsUsesConfiguredTicketConcurrency(t *testing.T) {
	root := testutil.TempRepo(t)
	opts, err := buildImplementRunOptionsForTickets(root, "my-epic", ralphloop.AgentCodex, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.MaxParallel != 5 {
		t.Fatalf("MaxParallel = %d, want 5", opts.MaxParallel)
	}
}

func TestNewModel_RendersEmptyStateWithNoScratchDir(t *testing.T) {
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

// deliverLoad runs the model's Init cmd and feeds its result back through
// Update, mirroring what the runtime does between Init and the first
// WindowSizeMsg.
func deliverLoad(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		return m
	}
	updated, _ := m.Update(cmd())
	return updated.(Model)
}

func TestNewModel_RendersEpicsAndTicketsFromDisk(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

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
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "10a-split-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "10a Split ticket") {
		t.Fatalf("expected suffixed ticket identifier in view, got:\n%s", m.View().Content)
	}
}

func TestModel_StackedLayoutSplitsAvailableHeightEvenly(t *testing.T) {
	m := Model{width: 80, height: 30}
	sidebarH, previewH := m.splitHeight(m.contentHeight())
	if sidebarH+previewH != 28 { // 30 minus the footer line minus the seam row
		t.Errorf("sidebarH(%d) + previewH(%d) != 28", sidebarH, previewH)
	}
	if sidebarH != 14 || previewH != 14 {
		t.Errorf("stacked heights = (%d, %d), want (14, 14)", sidebarH, previewH)
	}
}

func TestNewModel_ZeroEpicScratchDirRendersSameEmptyStateAsNoScratchDir(t *testing.T) {
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

// TestNewModel_TicketsInPlanOrderWithinEpic guards against re-grouping
// tickets by rendered status: they render in plan order (ticket number
// ascending) so a ticket never jumps position once it's done, regardless of
// status — see sortedTicketIndexes.
func TestNewModel_TicketsInPlanOrderWithinEpic(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-done-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-open-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "03-needs-info-ticket.md", "Status: needs-info\n\nBody.\n")
	writeTicket(t, root, "my-epic", "04-blocked-ticket.md", "Status: open\nBlocked by: 02\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	wantOrder := []string{"Done ticket", "Open ticket", "Needs info ticket", "Blocked ticket"}
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

func TestNewModel_BlockedTicketShowsUnresolvedBlockerSuffix(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix in view, got:\n%s", content)
	}
}

func TestNewModel_NeedsInfoTicketShowsUnresolvedBlockerSuffix(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-needs-info-ticket.md", "Status: needs-info\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix on needs-info ticket, got:\n%s", content)
	}
}

func TestNewModel_ResolvedBlockerDropsSuffixAndRegroups(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-formerly-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

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
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-bogus-status-ticket.md", "Status: bogus-value\n\nBody.\n")

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
	root := t.TempDir()
	writeTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "done-epic", "02-second-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if strings.Contains(content, "First ticket") || strings.Contains(content, "Second ticket") {
		t.Fatalf("expected done-epic's tickets hidden by default collapse, got:\n%s", content)
	}
	if !strings.Contains(content, "Only ticket") {
		t.Fatalf("expected open-epic to start expanded, got:\n%s", content)
	}
}

func TestModel_EpicsLoadedMsgPreservesManualCollapseToggle(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Manually collapse open-epic, which defaults to expanded.
	m.setCollapsed(indexOfEpic(t, m, "open-epic"), true)
	if !m.isCollapsed(findEpic(t, m, "open-epic")) {
		t.Fatalf("expected open-epic collapsed after manual toggle")
	}

	// A new epic appears (simulating the auto-refresh poll picking up fresh
	// disk state) — it should get its correct default while the manual
	// toggle above survives.
	writeTicket(t, root, "new-done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")

	msg := m.cmdLoad()()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if !m.isCollapsed(findEpic(t, m, "open-epic")) {
		t.Fatalf("expected manually-collapsed open-epic to stay collapsed after epicsLoadedMsg")
	}
	if !m.isCollapsed(findEpic(t, m, "new-done-epic")) {
		t.Fatalf("expected new fully-done epic to default to collapsed")
	}
}

func TestNewModel_ZeroTicketEpicStartsExpanded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".scratch", "empty-epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// A zero-ticket epic must not start dimmed/collapsed (only fully-done
	// epics with >=1 ticket do).
	if len(m.collapsedEpics) != 0 {
		t.Fatalf("expected zero-ticket epic to start expanded, collapsedEpics=%v", m.collapsedEpics)
	}
}

func TestNewModel_SplitsOpenAndClosedEpicSections(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "Open epics (1)") {
		t.Fatalf("expected 'Open epics (1)' header, got:\n%s", content)
	}
	if !strings.Contains(content, "Closed epics (1)") {
		t.Fatalf("expected 'Closed epics (1)' header, got:\n%s", content)
	}
	openIdx := strings.Index(content, "open-epic")
	closedIdx := strings.Index(content, "done-epic")
	openHeaderIdx := strings.Index(content, "Open epics (1)")
	closedHeaderIdx := strings.Index(content, "Closed epics (1)")
	if !(openHeaderIdx < openIdx && openIdx < closedHeaderIdx && closedHeaderIdx < closedIdx) {
		t.Fatalf("expected order [open header, open-epic, closed header, done-epic], got:\n%s", content)
	}
}

func TestNewModel_EmptySectionShowsMutedPlaceholder(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "Closed epics (0)") || !strings.Contains(content, "no closed epics") {
		t.Fatalf("expected empty Closed epics section placeholder, got:\n%s", content)
	}
}

func TestModel_NavigationAndSelectionUnaffectedBySectionHeaders(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on the first row of visibleRows(), which is the
	// open-epic (open epics render first) — not a header.
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() || m.epics[r.epicIdx].Name != "open-epic" {
		t.Fatalf("expected initial selection on open-epic, got row=%+v ok=%v", r, ok)
	}

	// open-epic isn't collapsed, so the next row down is its own ticket, then
	// done-epic (collapsed by default) — neither a header line.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	r, ok = m.selectedRow()
	if !ok || r.isEpic() {
		t.Fatalf("expected selection on open-epic's ticket, got row=%+v ok=%v", r, ok)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	r, ok = m.selectedRow()
	if !ok || !r.isEpic() || m.epics[r.epicIdx].Name != "done-epic" {
		t.Fatalf("expected selection on done-epic after moving down twice, got row=%+v ok=%v", r, ok)
	}
}

// TestModel_MouseClickSelectsSidebarRowOnly covers ticket 05c: clicking a
// row in the Tickets tab's sidebar must select it (like arrowing there) and
// do nothing else — no checkbox toggle, no confirm modal.
func TestModel_MouseClickSelectsSidebarRowOnly(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Navigate to the second ticket row (epic header, then its two tickets)
	// so we can read the line its click needs to land on.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.selected != 2 {
		t.Fatalf("expected selection at row 2 after two downs, got %d", m.selected)
	}
	line, _, ok := m.sidebarLineForSelected()
	if !ok {
		t.Fatalf("expected a line for the selected row")
	}
	checkedBefore := len(m.checked)

	m.selected = 0
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: line + 1})
	m = updated.(Model)
	if m.selected != 2 {
		t.Fatalf("expected click to select row 2, got %d", m.selected)
	}
	if m.confirm.IsOpen {
		t.Fatalf("expected click-to-select to not open the confirm modal")
	}
	if len(m.checked) != checkedBefore {
		t.Fatalf("expected click-to-select to leave checked state untouched, before=%d after=%d", checkedBefore, len(m.checked))
	}

	// A click landing on the preview panel (past the sidebar's width) is a no-op.
	sidebarW, _ := m.splitWidth()
	m.selected = 0
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: sidebarW, Y: line + 1})
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected click on the preview panel to leave selection at row 0, got %d", m.selected)
	}

	// A non-left click must not move the selection either.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: 0, Y: line + 1})
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected non-left click to leave selection at row 0, got %d", m.selected)
	}
}

// TestModel_EnterOnExpandedEpicFocusesPreview covers the epic-row case of
// "enter"/"l": since my-epic starts expanded, enter must not collapse it —
// it should instead focus the preview panel, exactly like it does on a
// ticket row. Collapsing an epic is "h"/left's job only (see
// TestModel_HLCollapseAndExpandSelectedEpic).
func TestModel_EnterOnExpandedEpicFocusesPreview(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible before enter, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected epic to stay expanded after enter, got:\n%s", m.View().Content)
	}
	if m.focus != focusPreview {
		t.Fatalf("expected enter on already-expanded epic to focus preview, got focus=%v", m.focus)
	}
}

// TestModel_EnterOnCollapsedEpicExpandsThenFocusesPreview covers the other
// half: on a collapsed epic, the first enter expands it (staying on the
// sidebar so the user can see what appeared); only a second enter, now that
// it's expanded, focuses the preview panel.
func TestModel_EnterOnCollapsedEpicExpandsThenFocusesPreview(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket hidden after 'h' collapse, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected first enter to expand the collapsed epic, got:\n%s", m.View().Content)
	}
	if m.focus != focusSidebar {
		t.Fatalf("expected first enter on collapsed epic to keep sidebar focus, got focus=%v", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.focus != focusPreview {
		t.Fatalf("expected second enter on now-expanded epic to focus preview, got focus=%v", m.focus)
	}
}

func TestModel_HLCollapseAndExpandSelectedEpic(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket hidden after 'h' collapse, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible after 'l' expand, got:\n%s", m.View().Content)
	}
}

func TestModel_NavigationSkipsCollapsedEpicTickets(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "epic-b", "01-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Rows so far: [epic-a, first-ticket, epic-b, second-ticket]. Collapse
	// epic-a (row 0), then moving down once should land on epic-b, not its
	// now-hidden ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || !r.isEpic() || m.epics[r.epicIdx].Name != "epic-b" {
		t.Fatalf("expected selection to land on epic-b after collapsing epic-a, got row=%+v ok=%v", r, ok)
	}
}

func TestModel_NoGlobalCollapseExpandAllBinding(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "epic-b", "01-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Collapsing the selected epic (epic-a) must not affect epic-b.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected epic-b's ticket unaffected by collapsing epic-a, got:\n%s", m.View().Content)
	}
}

func TestModel_DimmingTracksAllDoneNotCollapseState(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	dimPrefix := strings.SplitN(statusDoneStyle.Render("PROBE"), "PROBE", 2)[0]

	doneEpic := findEpic(t, m, "done-epic")
	openEpic := findEpic(t, m, "open-epic")

	// done-epic starts collapsed by default: expand it and confirm it's
	// still dimmed (dimming tracks AllDone(), not the collapse toggle).
	m.setCollapsed(indexOfEpic(t, m, "done-epic"), false)
	if !strings.Contains(m.renderEpicRow(doneEpic), dimPrefix) {
		t.Fatalf("expected done-epic to stay dimmed after manual expand, got: %q", m.renderEpicRow(doneEpic))
	}

	// open-epic starts expanded: collapse it and confirm it does NOT
	// become dimmed just because it's collapsed.
	m.setCollapsed(indexOfEpic(t, m, "open-epic"), true)
	if strings.Contains(m.renderEpicRow(openEpic), dimPrefix) {
		t.Fatalf("expected open-epic to stay undimmed after manual collapse, got: %q", m.renderEpicRow(openEpic))
	}
}

func TestModel_TCTogglesHideDoneTickets(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket visible before 'tc', got:\n%s", m.View().Content)
	}
	if got := len(m.visibleRows()); got != 3 {
		t.Fatalf("expected 3 visible rows (epic + 2 tickets) before 'tc', got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket hidden after 'tc', got:\n%s", m.View().Content)
	}
	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected open ticket still visible after 'tc', got:\n%s", m.View().Content)
	}
	if got := len(m.visibleRows()); got != 2 {
		t.Fatalf("expected 2 visible rows (epic + open ticket) after 'tc', got %d", got)
	}
	// Epic header counts read epic.Tickets directly, so the filter must not
	// change them.
	if !strings.Contains(m.View().Content, "(1 done / 2)") {
		t.Fatalf("expected epic header counts unaffected by 'tc', got:\n%s", m.View().Content)
	}

	// Toggling 'tc' again restores the done ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket visible again after second 'tc', got:\n%s", m.View().Content)
	}
	if got := len(m.visibleRows()); got != 3 {
		t.Fatalf("expected 3 visible rows again after second 'tc', got %d", got)
	}
}

func TestModel_TCOnFullyDoneEpicHidesAllTicketsButKeepsEpicRow(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// done-epic starts collapsed by default (AllDone); expand it so its
	// ticket row would normally be visible.
	m.setCollapsed(indexOfEpic(t, m, "done-epic"), false)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	rows := m.visibleRows()
	if len(rows) != 1 || !rows[0].isEpic() {
		t.Fatalf("expected only the epic row visible for a fully-done epic under 'tc', got %+v", rows)
	}
	if !strings.Contains(m.View().Content, "done-epic") {
		t.Fatalf("expected epic row to still render, got:\n%s", m.View().Content)
	}
}

func TestModel_UnrelatedTAndCSequencesUnaffectedByHideDoneChord(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	// "e", "t" is the edit-tab chord, sharing the 't' key with the
	// hide-complete chord's second key: it must still fire as edit-tab, not
	// toggle hideDone.
	t.Setenv("EDITOR", "true")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected 'et' to still launch the edit-tab command")
	}
	if m.hideDone {
		t.Fatalf("expected 'et' to leave hideDone untouched")
	}

	// A bare "c" (no preceding "t") must not toggle hideDone either.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.hideDone {
		t.Fatalf("expected a bare 'c' to leave hideDone untouched")
	}
}

func TestModel_RRefreshesDataFromDisk(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket absent before it's written, got:\n%s", m.View().Content)
	}

	// Simulate an edit made outside gx (e.g. in $EDITOR): a new ticket file
	// appears on disk after the tab already loaded.
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a refresh cmd from pressing R")
	}
	m = deliverCmd(t, m, cmd)

	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket visible after R refresh, got:\n%s", m.View().Content)
	}
}

// deliverCmd runs cmd (recursively unwrapping tea.BatchMsg) and feeds every
// resulting message back through Update, mirroring what the bubbletea
// runtime does for a batched command.
func deliverCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = deliverCmd(t, m, c)
		}
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func findEpic(t *testing.T, m Model, name string) tickets.Epic {
	t.Helper()
	return m.epics[indexOfEpic(t, m, name)]
}

func indexOfEpic(t *testing.T, m Model, name string) int {
	t.Helper()
	for i, e := range m.epics {
		if e.Name == name {
			return i
		}
	}
	t.Fatalf("epic %q not found", name)
	return -1
}

func writeTicket(t *testing.T, root, epic, filename, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(LegacyTicketToFrontmatter(filename, content)), 0644); err != nil {
		t.Fatal(err)
	}
}

// LegacyTicketToFrontmatter converts this package's many pre-existing test
// fixtures (plain "Status: x\nBlocked by: y, z\n\nBody" lines, predating
// ticket 04's retirement of old-format parsing) into a minimal valid
// frontmatter ticket, deriving id from filename so blocked_by
// cross-references between fixture tickets keep resolving. Exported (despite
// living in a _test.go file, so it never ships in production builds) so the
// tickets_test black-box package can share it instead of duplicating it.
func LegacyTicketToFrontmatter(filename, content string) string {
	id := filename
	if idx := strings.Index(id, "-"); idx >= 0 {
		id = id[:idx]
	}

	status, typ := "open", "task"
	var blockedBy []string
	lines := strings.Split(content, "\n")
	bodyStart := 0
	for _, line := range lines {
		matched := true
		switch {
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimPrefix(line, "Status: ")
		case strings.HasPrefix(line, "Blocked by: "):
			blockedBy = strings.Split(strings.TrimPrefix(line, "Blocked by: "), ", ")
		case strings.HasPrefix(line, "Type: "):
			typ = strings.TrimPrefix(line, "Type: ")
		default:
			matched = false
		}
		if !matched {
			break
		}
		bodyStart++
	}
	body := strings.TrimPrefix(strings.Join(lines[bodyStart:], "\n"), "\n")

	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: %s\n", id, status, typ)
	if len(blockedBy) > 0 {
		fmt.Fprintf(&b, "blocked_by: [\"%s\"]\n", strings.Join(blockedBy, "\", \""))
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}

func writeMap(t *testing.T, root, epic, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "map.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewModel_RendersBeforeSizing(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.New(nil))
	// Never hidden: the tab must render something even before a WindowSizeMsg
	// arrives (mirrors the "reachable and visually present" acceptance
	// criterion from ticket 01).
	content := m.View().Content
	if strings.TrimSpace(content) == "" {
		t.Fatal("expected non-empty view even before sizing")
	}
}
