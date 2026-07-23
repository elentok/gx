package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

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
	if !strings.Contains(content, "my-epic") || !strings.Contains(content, "(1/2)") {
		t.Fatalf("expected epic row with name + (1/2) count, got:\n%s", content)
	}
	if !strings.Contains(content, "First ticket") || !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected ticket titles in view, got:\n%s", content)
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

func TestNewModel_TicketsGroupedByStatusWithinEpic(t *testing.T) {
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
	wantOrder := []string{"Open ticket", "Blocked ticket", "Needs info ticket", "Done ticket"}
	lastIdx := -1
	for _, title := range wantOrder {
		idx := strings.Index(content, title)
		if idx == -1 {
			t.Fatalf("expected %q in view, got:\n%s", title, content)
		}
		if idx < lastIdx {
			t.Fatalf("expected %q to render after previous group, got:\n%s", title, content)
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
	if !strings.Contains(content, "(blocked by 1)") {
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
	if !strings.Contains(content, "(blocked by 1)") {
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
	writeTicket(t, root, "done-epic", "02-second-ticket.md", "Status: resolved\n\nBody.\n")
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

func TestModel_EnterTogglesEpicCollapse(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible before collapse, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket hidden after collapsing epic via enter, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible again after re-toggling, got:\n%s", m.View().Content)
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

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]

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
