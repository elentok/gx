package tickets_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets"
)

const (
	flatTermWidth  = 120
	flatTermHeight = 40
	flatWait       = 3 * time.Second
)

func writeFlatTicket(t *testing.T, root, epic, filename, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func startFlatTUI(t *testing.T, root, epicName string) *teatest.TestModel {
	t.Helper()
	m := tickets.NewFlatModel(root, epicName, ui.Settings{})
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
}

func waitForFlatText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(flatWait))
}

func flatKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func flatKeySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestFlatTUI_LaunchRendersFlatTicketList(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nSecond body.\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "First ticket")
	waitForFlatText(t, tm, "Second ticket")

	frame := tm.CurrentFrame()
	if bytes.Contains(frame, []byte("Open epics")) || bytes.Contains(frame, []byte("Closed epics")) {
		t.Fatalf("expected a flat ticket list with no epic-of-epics grouping, got:\n%s", frame)
	}
}

func TestFlatTUI_NavigationAndPreviewRendering(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nFirstbodymarker\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nSecondbodymarker\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "Firstbodymarker")

	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Secondbodymarker")

	tm.Send(flatKeyRune('l'))
	tm.Send(flatKeyRune('h'))
	waitForFlatText(t, tm, "Secondbodymarker")
}

func TestFlatTUI_RefreshAndQuit(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	tm := startFlatTUI(t, root, "my-epic")

	waitForFlatText(t, tm, "First ticket")

	tm.Send(tea.KeyPressMsg{Code: 'R', Text: "R"})
	waitForFlatText(t, tm, "refreshed")

	tm.Send(flatKeyRune('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestFlatTUI_EditChordCancels(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "First ticket")

	tm.Send(flatKeyRune('e'))
	tm.Send(flatKeySpecial(tea.KeyEsc)) // cancels the "e"-prefix chord

	// The list must still respond to plain navigation after the cancel.
	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Second ticket")
}
