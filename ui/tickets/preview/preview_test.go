package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

// A server-mode ticket carries no body, only its file: the preview reads the
// body from there.
func TestRenderTicket_ReadsBodyFromFileWhenMissing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "20-a.md")
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: draft\ntype: research\n---\n\n# 20 — A\n\nThe body text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ticket := tickets.Ticket{Identifier: "20", Title: "A", Path: "gx:alpha/20", File: file, Status: "draft"}

	content, _, _ := RenderTicket(tickets.Epic{Tickets: []tickets.Ticket{ticket}}, ticket, 80)

	if !strings.Contains(ansi.Strip(content), "The body text.") {
		t.Errorf("preview has no body:\n%s", ansi.Strip(content))
	}
}

func TestRenderTicket_PausedAnswerAppendsPauseRounds(t *testing.T) {
	file := filepath.Join(t.TempDir(), "20-a.md")
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: paused\npause_reason: blocked-pane\ntype: research\n---\n\n# 20 — A\n\nThe body text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.AppendLog(file, tickets.LogEvent{Type: tickets.EventPauseQuestion, Summary: "Which store?", Text: "Details."}); err != nil {
		t.Fatal(err)
	}
	rendered := tickets.StatusPausedAnswer
	ticket := tickets.Ticket{Identifier: "20", Title: "A", Path: "gx:alpha/20", File: file, Status: string(schema.StatusPaused), PauseReason: schema.PauseBlockedPane, Rendered: &rendered}

	content, target, ok := RenderTicket(tickets.Epic{Tickets: []tickets.Ticket{ticket}}, ticket, 80)

	lines := strings.Split(ansi.Strip(content), "\n")
	if !ok || target >= len(lines) || lines[target] != "1. open: Which store?" {
		t.Fatalf("ok=%v target=%d, want the latest round line in:\n%s", ok, target, strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), rowfmt.PausedAnswerHint) {
		t.Errorf("preview lacks the answer hint:\n%s", strings.Join(lines, "\n"))
	}
}

// TestHighlightPauseSection_PausedRepairRed covers the preview's paused (repair)
// coloring: the heading is red, and paused (answer) is left to the rounds block.
func TestHighlightPauseSection_PausedRepairRed(t *testing.T) {
	t.Parallel()
	rendered := "intro\n" + rowfmt.PausedRepairHeading + "\nsome reason\n"
	out, target, ok := highlightPauseSection(rendered, tickets.StatusPausedRepair)
	if !ok || target != 1 {
		t.Fatalf("highlightPauseSection() ok=%v target=%d, want ok=true target=1", ok, target)
	}
	wantRed := lipgloss.NewStyle().Foreground(ui.ColorRed).Render(rowfmt.PausedRepairHeading)
	if !strings.Contains(out, wantRed) {
		t.Fatalf("expected paused (repair) heading styled red in:\n%s", out)
	}
	if _, _, ok := highlightPauseSection(rendered, tickets.StatusPausedAnswer); ok {
		t.Fatal("paused (answer) must not highlight a body section")
	}
}

func TestRenderPauseRoundsBlock_OnlyLatestRoundIsOrange(t *testing.T) {
	t.Parallel()
	events := []tickets.LogEvent{
		{Seq: 1, Type: tickets.EventPauseQuestion, Summary: "First?"},
		{Seq: 2, Type: tickets.EventPauseAnswer, Question: 1, Choice: "yes"},
		{Seq: 3, Type: tickets.EventPauseQuestion, Summary: "Second?", Text: "detail"},
	}
	block, line, ok := renderPauseRoundsBlock(events, 80)
	if !ok {
		t.Fatal("found = false, want true")
	}
	lines := strings.Split(block, "\n")
	if got := ansi.Strip(lines[line]); got != "2. open: Second?" {
		t.Errorf("latest line = %q (index %d), want the second round", got, line)
	}
	orange := lipgloss.NewStyle().Foreground(ui.ColorOrange)
	if !strings.Contains(lines[line], orange.Render("2. open: Second?")) ||
		!strings.Contains(lines[line+1], orange.Render("   detail")) {
		t.Errorf("latest round not orange:\n%q", lines[line:])
	}
	if strings.Contains(strings.Join(lines[:line], "\n"), orange.Render("1.")) {
		t.Errorf("earlier rounds must stay plain:\n%q", lines[:line])
	}
	if !strings.Contains(lines[1], rowfmt.PausedAnswerHint) {
		t.Errorf("hint missing under the heading: %q", lines[1])
	}
}

func TestRenderPauseRoundsBlock_NoRoundsIsNotFound(t *testing.T) {
	t.Parallel()
	if _, _, ok := renderPauseRoundsBlock(nil, 80); ok {
		t.Error("found = true, want false")
	}
}

func TestPane_HidingFocusedPreviewReturnsFocusToList(t *testing.T) {
	t.Parallel()
	p := NewPane()
	p.Focus = FocusPreview
	p.Toggle()
	if p.Focus != FocusSidebar {
		t.Fatalf("expected focus back on the list, got %v", p.Focus)
	}
}
