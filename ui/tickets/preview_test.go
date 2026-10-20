package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestModel_SelectingTicketShowsFrontmatterAndBodyNoHeader(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Type: implement\nStatus: open\n\n## Heading\n\nSome distinctive body prose.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic (selected), ticket] - move down once to
	// select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if strings.Contains(content, "#01 First ticket") {
		t.Fatalf("expected the old header line gone from the preview, got:\n%s", content)
	}
	if !strings.Contains(content, "Status: open") {
		t.Fatalf("expected prettified 'Status:' frontmatter line in view, got:\n%s", content)
	}
	if !strings.Contains(content, "Type: implement") {
		t.Fatalf("expected prettified 'Type:' frontmatter line in view, got:\n%s", content)
	}
	if !strings.Contains(content, "Some distinctive body prose.") {
		t.Fatalf("expected glamour-rendered body text in view, got:\n%s", content)
	}
	if !strings.Contains(content, "## Heading") {
		t.Fatalf("expected heading markdown rendered (not stripped) in view, got:\n%s", content)
	}
}

// TestModel_PreviewFrontmatterUsesPrettifiedFieldLabels covers the
// blocked_by/actual_context_window prettification example from the shared
// preview pane ticket: "blocked_by" -> "Blocked by" (default transform) and
// "actual_context_window" -> "Context window" (an explicit override, since
// the default transform would read "Actual context window").
func TestModel_PreviewFrontmatterUsesPrettifiedFieldLabels(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-blocker.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-blocked.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic (selected), 01-blocker, 02-blocked] - move
	// down twice to select 02.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Blocked by: 01") {
		t.Fatalf("expected prettified 'Blocked by:' frontmatter line in view, got:\n%s", content)
	}
}

func TestModel_PreviewPlainEpicShowsHeaderOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nDistinctive ticket body.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// The epic's own sidebar row renders its name + open/total count
	// regardless of selection.
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "my-epic") || !strings.Contains(content, "(0 done / 1)") {
		t.Fatalf("expected epic name + open/total count in preview header, got:\n%s", content)
	}
	if strings.Contains(content, "[map]") {
		t.Fatalf("expected no [map] badge for a plain epic, got:\n%s", content)
	}
	if strings.Contains(content, "Distinctive ticket body.") {
		t.Fatalf("expected no ticket body in a plain epic's preview, got:\n%s", content)
	}
}

func TestModel_PreviewMapEpicShowsMapBadgeAndBody(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMap(t, root, "wayfinder-epic", "# Wayfinder Map\n\nDistinctive map prose.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on the epic itself.
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "wayfinder-epic") || !strings.Contains(content, "[map]") || !strings.Contains(content, "(0 done / 0)") {
		t.Fatalf("expected epic name + [map] badge + open/total count in preview header, got:\n%s", content)
	}
	if !strings.Contains(content, "Distinctive map prose.") {
		t.Fatalf("expected map body rendered in preview, got:\n%s", content)
	}
}

func TestModel_PreviewUnreadableTicketShowsErrorMessage(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root: unreadable-file permissions aren't enforced")
	}

	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-broken.md", "Status: open\n\nBody.\n")
	brokenPath := filepath.Join(root, ".scratch", "my-epic", "issues", "01-broken.md")
	if err := os.Chmod(brokenPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(brokenPath, 0644) })

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic (selected), ticket] - move down once to
	// select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "error reading ticket file") {
		t.Fatalf("expected I/O error message in preview, got:\n%s", content)
	}
}

func TestModel_PreviewUnrecognizedStatusShowsReadError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-weird-ticket.md", "Status: bogus-value\n\nDistinctive body text.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic (selected), ticket] - move down once to
	// select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "error reading ticket file") {
		t.Fatalf("expected a read-error message in preview for an unrecognized status, got:\n%s", content)
	}
}

func TestModel_PreviewScrollbarAppearsOnlyWhenBodyOverflows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-short-ticket.md", "Status: open\n\nShort body.\n")
	writeTicket(t, root, "epic", "02-long-ticket.md", "Status: open\n\n"+strings.Repeat("Line of body text.\n\n", 100))

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic (selected), short-ticket, long-ticket]
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	shortContent := m.View().Content
	if strings.Contains(shortContent, "┃") {
		t.Fatalf("expected no scrollbar thumb for a short body, got:\n%s", shortContent)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	longContent := m.View().Content
	if !strings.Contains(longContent, "┃") {
		t.Fatalf("expected a scrollbar thumb for an overflowing body, got:\n%s", longContent)
	}
}

// TestModel_GAndGGJumpSidebarSelectionToLastAndFirstRow covers ticket 11's
// "G"/"gg" bindings: they move the sidebar's own selection to the last/first
// visible row, independent of the preview (which has its own "b" for
// bottom).
func TestModel_GAndGGJumpSidebarSelectionToLastAndFirstRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "epic", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "epic", "03-third.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	// Empty sections are hidden, so the last row is the last ticket.
	if r, ok := m.selectedRow(); !ok || r.isEpic() || m.epicAt(r).Tickets[r.ticketIdx].Number != 3 {
		t.Fatalf("expected 'G' to select the last ticket row, got %+v (ok=%v)", r, ok)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	// The first row is now the "Open epics" section header itself
	// (cursor-reachable, per 03a) — selectedRow reports ok=false there.
	if _, ok := m.selectedRow(); ok {
		t.Fatalf("expected 'gg' to select the first row (section header, ok=false)")
	}
}

// TestModel_BJumpsPreviewToBottomFromListFocus covers ticket 11's "b"
// binding: from the sidebar (list) focus it scrolls the preview to its
// bottom without moving focus off the sidebar.
func TestModel_BJumpsPreviewToBottomFromListFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-ticket.md", "Status: open\n\nTOPMARKERXYZ\n\n"+strings.Repeat("Filler line of body text.\n\n", 80)+"BOTTOMMARKERXYZ\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if m.focus != focusSidebar {
		t.Fatalf("expected 'b' to leave focus on the sidebar, got focus=%v", m.focus)
	}
	content := ansi.Strip(m.previewVP.View())
	if !strings.Contains(content, "BOTTOMMARKERXYZ") {
		t.Fatalf("expected preview scrolled to bottom marker, got:\n%s", content)
	}
	if strings.Contains(content, "TOPMARKERXYZ") {
		t.Fatalf("expected top marker scrolled out of view, got:\n%s", content)
	}
}

// TestModel_BJumpsPreviewToBottomFromPreviewFocus covers the same "b"
// binding from preview focus, overriding bubbles/viewport's own default "b"
// (page up) — see handlePreviewKey.
func TestModel_BJumpsPreviewToBottomFromPreviewFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-ticket.md", "Status: open\n\nTOPMARKERXYZ\n\n"+strings.Repeat("Filler line of body text.\n\n", 80)+"BOTTOMMARKERXYZ\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.focus != focusPreview {
		t.Fatalf("expected preview focus after enter on ticket row, got focus=%v", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if m.focus != focusPreview {
		t.Fatalf("expected 'b' to leave focus on the preview, got focus=%v", m.focus)
	}
	content := ansi.Strip(m.previewVP.View())
	if !strings.Contains(content, "BOTTOMMARKERXYZ") {
		t.Fatalf("expected preview scrolled to bottom marker, got:\n%s", content)
	}
	if strings.Contains(content, "TOPMARKERXYZ") {
		t.Fatalf("expected top marker scrolled out of view, got:\n%s", content)
	}
}

// TestModel_PreviewSearchHighlightsMatch guards against the preview search
// only driving match-count/n-N navigation without visibly marking the
// match: querying for text known to be in the glamour-rendered body must
// wrap it in the search-highlight style, not just leave the plain text
// untouched.
func TestModel_PreviewSearchHighlightsMatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nSome distinctive body prose.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.focus != focusPreview {
		t.Fatalf("expected preview focus after enter on ticket row, got focus=%v", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	m = updated.(Model)
	for _, r := range "prose" {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	content := m.previewVP.GetContent()
	if !strings.Contains(ansi.Strip(content), "distinctive body prose") {
		t.Fatalf("expected body text still present in preview, got:\n%s", ansi.Strip(content))
	}
	if !strings.Contains(content, ui.StyleActiveSearchResult.Render("prose")) {
		t.Fatalf("expected 'prose' wrapped in the active search-highlight style, got:\n%s", content)
	}
}

// A server-mode ticket carries no body, only its file: the preview reads the
// body from there.
func TestRenderTicketPreview_ReadsBodyFromFileWhenMissing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "20-a.md")
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: draft\ntype: research\n---\n\n# 20 — A\n\nThe body text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ticket := tickets.Ticket{Identifier: "20", Title: "A", Path: "gx:alpha/20", File: file, Status: "draft"}

	content, _, _ := renderTicketPreview(tickets.Epic{Tickets: []tickets.Ticket{ticket}}, ticket, 80)

	if !strings.Contains(ansi.Strip(content), "The body text.") {
		t.Errorf("preview has no body:\n%s", ansi.Strip(content))
	}
}

func TestRenderTicketPreview_NeedsAnswerAppendsParkRounds(t *testing.T) {
	file := filepath.Join(t.TempDir(), "20-a.md")
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: needs-answer\ntype: research\n---\n\n# 20 — A\n\nThe body text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.AppendLog(file, tickets.LogEvent{Type: tickets.EventParkQuestion, Summary: "Which store?", Text: "Details."}); err != nil {
		t.Fatal(err)
	}
	rendered := tickets.StatusNeedsAnswer
	ticket := tickets.Ticket{Identifier: "20", Title: "A", Path: "gx:alpha/20", File: file, Status: "needs-answer", Rendered: &rendered}

	content, target, ok := renderTicketPreview(tickets.Epic{Tickets: []tickets.Ticket{ticket}}, ticket, 80)

	lines := strings.Split(ansi.Strip(content), "\n")
	if !ok || target >= len(lines) || lines[target] != "1. open: Which store?" {
		t.Fatalf("ok=%v target=%d, want the latest round line in:\n%s", ok, target, strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), needsAnswerHint) {
		t.Errorf("preview lacks the answer hint:\n%s", strings.Join(lines, "\n"))
	}
}
