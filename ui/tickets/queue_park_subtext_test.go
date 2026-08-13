package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tree"
)

// writeParkedFrontmatterTicket writes a needs-answer/needs-repair ticket with
// both a real Parent field (writeFrontmatterTicket doesn't support one) and a
// park-reason body section (writeTicket doesn't support Parent) — ticket 16
// needs both at once to test a parked ticket that also has children.
func writeParkedFrontmatterTicket(t *testing.T, root, epic, filename, id, status, parent, reason string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: task\n", id, status)
	if parent != "" {
		fmt.Fprintf(&b, "parent: %q\n", parent)
	}
	heading := "## Needs Answer"
	if status == "needs-repair" {
		heading = "## Needs Repair"
	}
	fmt.Fprintf(&b, "---\n\n%s\n\n%s\n", heading, reason)
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestQueueModel_ParkedRowSubtextComesFromDiskNotEventPayload is the ticket's
// primary test seam: reduceLiveEvent's Reason and the on-disk ticket body
// deliberately disagree, so a row showing the on-disk text (not
// event.Reason) proves drainPendingReload's re-read wins.
func TestQueueModel_ParkedRowSubtextComesFromDiskNotEventPayload(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md",
		"Status: needs-answer\n\n## Needs Answer\n\nOn-disk reason, not the event payload.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketNeedsHuman, Identifier: "01",
		Status: "needs-answer", Reason: "stale event reason - should not appear",
	})

	updated, cmd := m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	lines := m.queueBody(80)
	rowText := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(rowText, "On-disk reason, not the event payload.") {
		t.Fatalf("expected row subtext sourced from disk:\n%s", rowText)
	}
	if strings.Contains(rowText, "stale event reason") {
		t.Fatalf("expected row subtext NOT sourced from event.Reason:\n%s", rowText)
	}
}

// TestQueueModel_DraftRowUnaffectedByParkRendering is acceptance criterion 7:
// a draft-status ticket's row must stay visually unchanged by this ticket's
// work, even when it happens to carry a leftover "## Needs Answer" heading.
func TestQueueModel_DraftRowUnaffectedByParkRendering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md",
		"Status: draft\n\n## Needs Answer\n\nLeftover text that must not surface.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	lines := m.queueBody(80)
	rowText := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(rowText, "Leftover text that must not surface.") {
		t.Fatalf("expected draft row to omit park subtext:\n%s", rowText)
	}
}

// TestHighlightParkSection_NeedsAnswerAndNeedsRepairUseDistinctColors covers
// the preview's severity coloring: needs-answer highlights orange,
// needs-repair red - distinct from each other and from the row-icon colors.
func TestHighlightParkSection_NeedsAnswerAndNeedsRepairUseDistinctColors(t *testing.T) {
	t.Parallel()
	rendered := "intro\n" + needsAnswerHeading + "\nsome reason\n"
	out, target, ok := highlightParkSection(rendered, tickets.StatusNeedsAnswer)
	if !ok || target != 1 {
		t.Fatalf("highlightParkSection() ok=%v target=%d, want ok=true target=1", ok, target)
	}
	wantOrange := lipgloss.NewStyle().Foreground(ui.ColorOrange).Render(needsAnswerHeading)
	if !strings.Contains(out, wantOrange) {
		t.Fatalf("expected needs-answer heading styled orange in:\n%s", out)
	}

	rendered = "intro\n" + needsRepairHeading + "\nsome reason\n"
	out, target, ok = highlightParkSection(rendered, tickets.StatusNeedsRepair)
	if !ok || target != 1 {
		t.Fatalf("highlightParkSection() ok=%v target=%d, want ok=true target=1", ok, target)
	}
	wantRed := lipgloss.NewStyle().Foreground(ui.ColorRed).Render(needsRepairHeading)
	if !strings.Contains(out, wantRed) {
		t.Fatalf("expected needs-repair heading styled red in:\n%s", out)
	}
	if wantOrange == wantRed {
		t.Fatal("expected needs-answer and needs-repair to use distinct colors")
	}
}

// TestQueueModel_SelectingParkedRowScrollsPreviewToParkSection covers the
// auto-scroll acceptance criterion: selecting a parked row positions the
// shared preview viewport so the park section heading is within view rather
// than left at the top.
func TestQueueModel_SelectingParkedRowScrollsPreviewToParkSection(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	var padding strings.Builder
	for range 80 {
		padding.WriteString("filler line\n\n")
	}
	writeTicket(t, root, "alpha", "01-first.md",
		"Status: needs-answer\n\n"+padding.String()+"\n## Needs Answer\n\nneeds a decision\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketNeedsHuman, Identifier: "01",
		Status: "needs-answer", Reason: "needs a decision",
	})

	updated, cmd := m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	m.View() // populate m.queueTree.Entries()
	m = selectFirstQueueTicketRow(t, m)

	if m.previewVP.YOffset() == 0 {
		t.Fatal("expected preview viewport to scroll past the top toward the park section")
	}
}

// TestQueueModel_ParkedTicketWithChildren_ReasonRendersDirectlyUnderItself is
// ticket 16's regression case: a parked ticket that also has children must
// show its reason as a second physical line on its own entry, immediately
// before its children in tree order — not as a trailing sibling node after
// every child (the bug ticket 16 fixes), and without granting the ticket a
// fold triangle it wouldn't otherwise have (it already has one, since it has
// real children).
func TestQueueModel_ParkedTicketWithChildren_ReasonRendersDirectlyUnderItself(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeParkedFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "needs-answer", "", "Parent needs a decision.")
	writeFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-a.md"): true,
		ticketPath(root, "alpha", "02-b.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	entries := m.queueTree.Entries()
	var parent tree.Entry[queueNode]
	parentIdx := -1
	for i, e := range entries {
		if e.Value.kind == nodeQueueTicket && e.Value.ticket.ticket.Identifier == "01" {
			parent, parentIdx = e, i
			break
		}
	}
	if parentIdx == -1 {
		t.Fatalf("expected to find ticket 01's entry, got %+v", entries)
	}
	if len(parent.Body) != 1 || !strings.Contains(parent.Body[0], "Parent needs a decision.") {
		t.Fatalf("expected ticket 01's own entry to carry the reason as its Body, got %+v", parent)
	}
	if !parent.HasChildren || !parent.Expanded {
		t.Fatalf("expected ticket 01 to still report hasChildren+expanded, got %+v", parent)
	}
	if parentIdx+1 >= len(entries) || entries[parentIdx+1].Value.ticket.ticket.Identifier != "02" {
		t.Fatalf("expected ticket 02 to immediately follow ticket 01's entry, got %+v", entries)
	}
}

// TestQueueModel_ParkedLeafTicket_HasChildrenStaysFalse guards the rejected
// approach this ticket's doc explicitly ruled out: reparenting the reason
// text under the ticket would make a parked leaf falsely report
// hasChildren==true (via childrenOf[path] > 0), granting it a fold triangle
// that could collapse to hide its own reason. Folding the reason into the
// entry's Body instead must never affect HasChildren.
func TestQueueModel_ParkedLeafTicket_HasChildrenStaysFalse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeParkedFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "needs-answer", "", "Leaf needs a decision.")
	checked := map[string]bool{ticketPath(root, "alpha", "01-a.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	entries := queueTicketEntries(m)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one ticket entry, got %+v", entries)
	}
	if entries[0].HasChildren {
		t.Fatalf("expected parked leaf ticket to report hasChildren=false, got %+v", entries[0])
	}
	if len(entries[0].Body) != 1 || !strings.Contains(entries[0].Body[0], "Leaf needs a decision.") {
		t.Fatalf("expected reason to still render as Body, got %+v", entries[0])
	}
}
