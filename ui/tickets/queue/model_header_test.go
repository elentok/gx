package queue

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

func TestQueueHeaderHasNoPausedLineWhenRunning(t *testing.T) {
	t.Parallel()
	m := NewQueueModel("", ui.Settings{}, keys.Manager{})
	if got := ansi.Strip(m.header().BodyLines()[0]); strings.HasPrefix(got, "Paused.") {
		t.Errorf("header = %q, want no paused line while the queue runs", got)
	}
}

func TestQueueSpinnerFollowsNerdFontSetting(t *testing.T) {
	t.Parallel()
	plain := NewQueueModel("", ui.Settings{UseNerdFontIcons: false}, keys.Manager{})
	nerd := NewQueueModel("", ui.Settings{UseNerdFontIcons: true}, keys.Manager{})
	if plain.implementSpinner.Spinner.Frames[0] != ui.ASCIIProgressSpinner.Frames[0] {
		t.Fatalf("plain spinner frame = %q, want ASCII frames", plain.implementSpinner.Spinner.Frames[0])
	}
	if nerd.implementSpinner.Spinner.Frames[0] != ui.ProgressSpinner.Frames[0] {
		t.Fatalf("nerd spinner frame = %q, want Nerd frames", nerd.implementSpinner.Spinner.Frames[0])
	}
}

func TestQueueModelEpicHeaderRendersStatusAndContextLines(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\nactual_context_window: 12000\nelapsed_time: 754\ncompactions: 2\n---\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: implement\nactual_context_window: 8000\nelapsed_time: 100\ncompactions: 1\n---\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := ansi.Strip(m.View().Content)

	if !strings.Contains(content, "Awaiting merge") || strings.Contains(content, "took") {
		t.Fatalf("expected the epic status line to say Awaiting merge without a took duration:\n%s", content)
	}
	if !strings.Contains(content, "Status: ") || !strings.Contains(content, " · 3 compacts") {
		t.Fatalf("expected the epic Status line with its compacts:\n%s", content)
	}
	if strings.Contains(content, "Context window") {
		t.Fatalf("expected the context-window row to be gone:\n%s", content)
	}
}

// TestQueueModelListRowsIndentMatchesHeaderIndent covers ticket 03's header
// indent and ticket 10's fixed-width triangle-column reservation: a
// childless row's pre-icon indent is the 2-char header indent plus the
// reserved triangle-glyph slot (blank, since this leaf ticket shows no
// triangle), not the header's bare 2-char indent.
func TestQueueModelListRowsIndentMatchesHeaderIndent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	m := selectFirstQueueTicketRow(t, loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked)))

	var headerLine, rowLine string
	for _, line := range m.queueBody(80) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "alpha") && headerLine == "" {
			headerLine = plain
		}
		if strings.Contains(plain, "First") {
			rowLine = plain
		}
	}
	if headerLine == "" || rowLine == "" {
		t.Fatalf("expected both an epic header line and a ticket row line:\n%v", m.queueBody(80))
	}
	headerIndent := len(headerLine) - len(strings.TrimLeft(headerLine, " "))
	// rowLine's first rune is the selection mark column (a space when
	// unselected, "▌" when this row holds the cursor, as it does here), which strings.TrimLeft's space-only trim can't skip past
	// on its own.
	rowRunes := []rune(rowLine)
	rowRest := string(rowRunes[1:])
	rowIndent := 1 + len(rowRest) - len(strings.TrimLeft(rowRest, " "))
	triangleColumn := rowfmt.TriangleColumnWidth(m.icons()) + 1
	if headerIndent != 2 || rowIndent != headerIndent+triangleColumn {
		t.Fatalf("got headerIndent=%d rowIndent=%d, want headerIndent=2 rowIndent=%d", headerIndent, rowIndent, 2+triangleColumn)
	}
}

// TestQueueEpicHeaderOmitsCostForNonRunningEpic covers the "no cost data"
// case: an incomplete local-mode epic gets nothing appended.
func TestQueueEpicHeaderOmitsCostForNonRunningEpic(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	headerLine := epicHeaderLine(t, m, "alpha")
	if strings.Contains(headerLine, "$") {
		t.Fatalf("expected no per-epic cost text for an incomplete local-mode epic: %q", headerLine)
	}
}

// TestQueueEpicHeaderAppendsSummedCostForCompletedEpic covers the completed-
// epic cost summary: an AllDone epic with no live cost data gets its final
// ActualCost total appended alongside "took Xm", summed across its own
// tickets.
func TestQueueEpicHeaderAppendsSummedCostForCompletedEpic(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\nelapsed_time: 60\nactual_cost: 30\n---\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: implement\nelapsed_time: 60\nactual_cost: 13\n---\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	statusLine := epicStatusLineText(t, m)
	if !strings.Contains(statusLine, "$43.00") {
		t.Fatalf("epic Status line missing summed cost \"$43.00\" for a completed epic: %q", statusLine)
	}
}

// epicStatusLineText returns m.queueBody's plain-text "Status:" line.
func epicStatusLineText(t *testing.T, m QueueModel) string {
	t.Helper()
	for _, line := range m.queueBody(80) {
		if plain := ansi.Strip(line); strings.Contains(plain, "Status:") {
			return plain
		}
	}
	t.Fatalf("no epic Status line found")
	return ""
}

// epicHeaderLine returns m.queueBody's plain-text epic header line naming
// epicName (the line rendered by queue_view.go's nodeEpicName case), for
// tests that need to inspect just that line rather than the whole panel
// (which also carries queueHeaderTitle's own live-total suffix).
func epicHeaderLine(t *testing.T, m QueueModel, epicName string) string {
	t.Helper()
	for _, line := range m.queueBody(80) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, epicName) {
			return plain
		}
	}
	t.Fatalf("no epic header line found naming %q", epicName)
	return ""
}

// layoutBody loads one done and one open ticket per epic name at the given
// window width and returns the plain Queue body lines.
func layoutBody(t *testing.T, width int, epicNames ...string) (QueueModel, []string) {
	t.Helper()
	root := t.TempDir()
	checked := map[string]bool{}
	for _, name := range epicNames {
		tickettest.WriteTicket(t, root, name, "01-first.md", "Status: open\n\nBody.\n")
		tickettest.WriteTicket(t, root, name, "02-second.md", "Status: open\n\nBody.\n")
		tickettest.WriteRawQueueTicket(t, root, name, "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\ncompactions: 1\n---\n\nBody.\n")
		checked[tickettest.TicketPath(root, name, "01-first.md")] = true
		checked[tickettest.TicketPath(root, name, "02-second.md")] = true
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = updated.(QueueModel)
	sidebarW, _ := m.preview.PanelWidths(m.width)
	var lines []string
	for _, line := range m.queueBody(sidebarW - 2) {
		lines = append(lines, strings.TrimRight(ansi.Strip(line), " "))
	}
	return m, lines
}

func TestQueueEpicHeaderWideLayoutEndsWithCounterAndBar(t *testing.T) {
	t.Parallel()
	m, lines := layoutBody(t, 140, "alpha")
	sidebarW, _ := m.preview.PanelWidths(m.width)
	var name, status string
	for i, l := range lines {
		if strings.Contains(l, "alpha") && name == "" {
			name = l
			status, _ = lines[i+1], 0
		}
	}
	if !strings.HasSuffix(name, "(1/2) ▰▰▰▰▰▰▱▱▱▱▱▱") {
		t.Fatalf("wide name line should end with counter and half-full bar: %q", name)
	}
	if got, want := lipgloss.Width(name), sidebarW-2-3+1; got != want {
		t.Fatalf("name line width (with selection mark) = %d, want %d", got, want)
	}
	if !strings.HasPrefix(strings.TrimSpace(status), "Status: ") || !strings.Contains(status, "1 compact") || strings.Contains(status, "1 compacts") {
		t.Fatalf("expected Status line with singular compact: %q", status)
	}
}

func TestQueueEpicHeaderNarrowSwitchesEveryEpic(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 60, "alpha", "a-very-long-epic-name-that-cannot-fit")
	var bars []string
	for _, l := range lines {
		if strings.Contains(l, "▰") || strings.Contains(l, "▱") {
			bars = append(bars, l)
		}
	}
	if len(bars) != 2 {
		t.Fatalf("expected a bar line per epic, got %q", bars)
	}
	for _, b := range bars {
		if !strings.HasPrefix(strings.TrimLeft(b, "▌ "), "▰▰▰▰▰▰▱▱▱▱▱▱ (1/2)") || strings.Index(b, "▰") != strings.Index(bars[0], "▰") {
			t.Fatalf("bar lines should share one column: %q", bars)
		}
	}
}

func TestQueueEpicHeaderVeryNarrowKeepsOnlyCounter(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 20, "alpha")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "▰") || strings.Contains(joined, "▱") {
		t.Fatalf("very narrow layout should drop the bar:\n%s", joined)
	}
	if !strings.Contains(joined, " (1/2)") {
		t.Fatalf("very narrow layout should keep the counter:\n%s", joined)
	}
}

func TestQueueEpicHeaderNarrowCutsLongNameWithEllipsis(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 40, "an-extremely-long-epic-name-that-overflows-the-pane")
	found := false
	for _, l := range lines {
		if strings.Contains(l, "an-extremely") && strings.HasSuffix(l, "…") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the long name cut with an ellipsis:\n%s", strings.Join(lines, "\n"))
	}
}

func TestQueueEpicHeaderHasSpacerAfterStatusLine(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 140, "alpha")
	for i, l := range lines {
		if strings.Contains(l, "Status:") {
			if strings.TrimSpace(lines[i+1]) != "" || !strings.Contains(lines[i+2], "First") {
				t.Fatalf("expected a blank spacer then the first ticket after the Status line: %q", lines[i:i+3])
			}
			return
		}
	}
	t.Fatalf("no Status line found")
}
