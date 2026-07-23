package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestModel_SelectingTicketShowsHeaderMetaAndBody(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Type: task\nStatus: open\n\n## Heading\n\nSome distinctive body prose.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [epic, ticket] - move down once to select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "#1 First ticket") {
		t.Fatalf("expected header line with number+title in view, got:\n%s", content)
	}
	if !strings.Contains(content, "open") {
		t.Fatalf("expected rendered status word in view, got:\n%s", content)
	}
	if !strings.Contains(content, "task") {
		t.Fatalf("expected ticket type in view, got:\n%s", content)
	}
	if !strings.Contains(content, "Some distinctive body prose.") {
		t.Fatalf("expected glamour-rendered body text in view, got:\n%s", content)
	}
	if !strings.Contains(content, "## Heading") {
		t.Fatalf("expected heading markdown rendered (not stripped) in view, got:\n%s", content)
	}
}

func TestModel_PreviewBlockedBySuffixOmittedOnceResolved(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-blocked-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows sorted by group: [epic, blocker-ticket(open), blocked-ticket(blocked)]
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "(blocked by 1)") {
		t.Fatalf("expected blocked-by suffix in preview, got:\n%s", content)
	}

	// Resolve the blocker and reload: the suffix should disappear.
	writeTicket(t, root, "my-epic", "01-blocker-ticket.md", "Status: done\n\nBody.\n")
	m = deliverLoad(t, m)
	content = ansi.Strip(m.View().Content)
	if strings.Contains(content, "blocked by") {
		t.Fatalf("expected no blocked-by suffix in preview once blocker resolves, got:\n%s", content)
	}
}

func TestModel_PreviewPlainEpicShowsHeaderOnly(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nDistinctive ticket body.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Default selection (row 0) is the epic row itself.
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "my-epic") || !strings.Contains(content, "(1/1)") {
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
	root := t.TempDir()
	writeMap(t, root, "wayfinder-epic", "# Wayfinder Map\n\nDistinctive map prose.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Default selection (row 0) is the epic row itself.
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "wayfinder-epic") || !strings.Contains(content, "[map]") || !strings.Contains(content, "(0/0)") {
		t.Fatalf("expected epic name + [map] badge + open/total count in preview header, got:\n%s", content)
	}
	if !strings.Contains(content, "Distinctive map prose.") {
		t.Fatalf("expected map.md body rendered in preview, got:\n%s", content)
	}
}

func TestModel_PreviewUnreadableTicketShowsErrorMessage(t *testing.T) {
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

	// rows: [epic, ticket] - move down once to select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "error reading ticket file") {
		t.Fatalf("expected I/O error message in preview, got:\n%s", content)
	}
}

func TestModel_PreviewUnrecognizedStatusRendersBodyNormally(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-weird-ticket.md", "Status: bogus-value\n\nDistinctive body text.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [epic, ticket] - move down once to select the ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Distinctive body text.") {
		t.Fatalf("expected ticket body rendered normally despite unrecognized status, got:\n%s", content)
	}
	if !strings.Contains(content, "error") {
		t.Fatalf("expected rendered status word 'error' in preview, got:\n%s", content)
	}
}

func TestModel_PreviewScrollbarAppearsOnlyWhenBodyOverflows(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-short-ticket.md", "Status: open\n\nShort body.\n")
	writeTicket(t, root, "epic", "02-long-ticket.md", "Status: open\n\n"+strings.Repeat("Line of body text.\n\n", 100))

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [epic, short-ticket, long-ticket]
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
