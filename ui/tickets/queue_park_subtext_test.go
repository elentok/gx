package tickets

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

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
