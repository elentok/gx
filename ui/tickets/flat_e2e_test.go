package tickets_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
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

// TestFlatTUI_LiveEventsDriveRowState feeds synthetic ralphloop.LiveEvents
// through WithLiveEvents and asserts each of ticket 04a's row states renders
// distinctly: a running ticket's spinner+label, a paused ticket's badge+
// reason, a needs-attention ticket's own badge+reason, and a done ticket's
// unchanged (ticket 03) dimmed rendering.
func TestFlatTUI_LiveEventsDriveRowState(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-paused-ticket.md", "Status: open\n\nSecond body.\n")
	writeFlatTicket(t, root, "my-epic", "03-attention-ticket.md", "Status: open\n\nThird body.\n")
	writeFlatTicket(t, root, "my-epic", "04-done-ticket.md", "Status: done\n\nFourth body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-02",
		PauseKind: ralphloop.PauseSmartZone, Reason: "context budget exceeded",
	}
	waitForFlatText(t, tm, "context budget exceeded")

	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketStillNeedsAttention, Identifier: "03",
	}
	waitForFlatText(t, tm, "no live iteration to reattach to")

	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("iter-01")) {
		t.Fatalf("expected running ticket's spinner row to still show its label, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("context budget exceeded")) {
		t.Fatalf("expected paused ticket's reason to render, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("no live iteration to reattach to")) {
		t.Fatalf("expected needs-attention ticket's reason to render, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("Done ticket")) {
		t.Fatalf("expected the done ticket's title to still render, got:\n%s", frame)
	}
}

// TestFlatTUI_LivePreviewMetadataAndTranscript feeds synthetic transcript-line
// events (ticket 01's EventSink.TranscriptLine) through WithLiveEvents and
// asserts ticket 04b's preview-pane additions: a running ticket's preview
// gains a metadata line (herdr tab id) and a live-updating transcript tail,
// while a done ticket's preview stays ticket 03's unchanged shape.
func TestFlatTUI_LivePreviewMetadataAndTranscript(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirstbodymarker\n")
	writeFlatTicket(t, root, "my-epic", "02-done-ticket.md", "Status: done\n\nSecondbodymarker\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Firstbodymarker")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventTranscriptLine, Label: "iter-01", Line: "transcriptlinemarker"}
	waitForFlatText(t, tm, "transcriptlinemarker")

	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("tab iter-01")) {
		t.Fatalf("expected preview metadata line with herdr tab id, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("transcriptlinemarker")) {
		t.Fatalf("expected preview transcript tail to show the live line, got:\n%s", frame)
	}

	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Secondbodymarker")

	frame = tm.CurrentFrame()
	if bytes.Contains(frame, []byte("tab iter-01")) {
		t.Fatalf("expected done ticket's preview to have no metadata line, got:\n%s", frame)
	}
	if bytes.Contains(frame, []byte("transcriptlinemarker")) {
		t.Fatalf("expected done ticket's preview to have no transcript tail, got:\n%s", frame)
	}
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
