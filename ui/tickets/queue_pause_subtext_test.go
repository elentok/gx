package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tree"
)

// writePausedFrontmatterTicket writes a paused ticket with
// both a real Parent field (writeFrontmatterTicket doesn't support one) and a
// pause-reason body section (writeTicket doesn't support Parent) — ticket 16
// needs both at once to test a paused ticket that also has children.
// pauseReason is a pause_reason frontmatter value such as "blocked-pane".
func writePausedFrontmatterTicket(t *testing.T, root, epic, filename, id, pauseReason, parent, reason string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(filepath.Dir(path)))
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: paused\npause_reason: %s\ntype: implement\n", id, pauseReason)
	if parent != "" {
		fmt.Fprintf(&b, "parent: %q\n", parent)
	}
	heading := tickets.LegacyAnswerHeading
	if pauseReason == string(schema.PauseSpinning) {
		heading = tickets.LegacyRepairHeading
	}
	fmt.Fprintf(&b, "---\n\n%s\n\n%s\n", heading, reason)
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestQueueModel_DraftRowUnaffectedByPauseRendering is acceptance criterion 7:
// a draft-status ticket's row must stay visually unchanged by this ticket's
// work, even when it happens to carry a leftover "## Paused" heading.
func TestQueueModel_DraftRowUnaffectedByPauseRendering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md",
		"Status: draft\n\n## Paused\n\nLeftover text that must not surface.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	lines := m.queueBody(80)
	rowText := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(rowText, "Leftover text that must not surface.") {
		t.Fatalf("expected draft row to omit pause subtext:\n%s", rowText)
	}
}

// TestQueueModel_PausedTicketWithChildren_ReasonRendersDirectlyUnderItself is
// ticket 16's regression case: a paused ticket that also has children must
// show its reason as a second physical line on its own entry, immediately
// before its children in tree order — not as a trailing sibling node after
// every child (the bug ticket 16 fixes), and without granting the ticket a
// fold triangle it wouldn't otherwise have (it already has one, since it has
// real children).
func TestQueueModel_PausedTicketWithChildren_ReasonRendersDirectlyUnderItself(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePausedFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "blocked-pane", "", "Parent needs a decision.")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-a.md"): true,
		tickettest.TicketPath(root, "alpha", "02-b.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	entries := m.rows.Tree.Entries()
	var parent tree.Entry[queue.Node]
	parentIdx := -1
	for i, e := range entries {
		if e.Value.Kind == queue.NodeTicket && e.Value.Ticket.Ticket.Identifier == "01" {
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
	if parentIdx+1 >= len(entries) || entries[parentIdx+1].Value.Ticket.Ticket.Identifier != "02" {
		t.Fatalf("expected ticket 02 to immediately follow ticket 01's entry, got %+v", entries)
	}
}

// TestQueueModel_PausedLeafTicket_HasChildrenStaysFalse guards the rejected
// approach this ticket's doc explicitly ruled out: reparenting the reason
// text under the ticket would make a paused leaf falsely report
// hasChildren==true (via childrenOf[path] > 0), granting it a fold triangle
// that could collapse to hide its own reason. Folding the reason into the
// entry's Body instead must never affect HasChildren.
func TestQueueModel_PausedLeafTicket_HasChildrenStaysFalse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePausedFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "blocked-pane", "", "Leaf needs a decision.")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-a.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	entries := queueTicketEntries(m)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one ticket entry, got %+v", entries)
	}
	if entries[0].HasChildren {
		t.Fatalf("expected paused leaf ticket to report hasChildren=false, got %+v", entries[0])
	}
	if len(entries[0].Body) != 1 || !strings.Contains(entries[0].Body[0], "Leaf needs a decision.") {
		t.Fatalf("expected reason to still render as Body, got %+v", entries[0])
	}
}
