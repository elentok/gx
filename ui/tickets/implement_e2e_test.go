package tickets_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets"
)

func TestTicketsTUI_ImplementKeyNoopsWithNothingChecked(t *testing.T) {
	t.Parallel()
	root := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".scratch", "my-epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}

	m := tickets.NewModel(root, ui.Settings{}, keys.New(nil))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForTicketsText(t, tm, "my-epic")

	tm.Send(tea.KeyPressMsg{Code: 'r', Text: "r"})

	frame := tm.CurrentFrame()
	if bytes.Contains(frame, []byte("Choose the agent")) || bytes.Contains(frame, []byte("Open the execution plan")) {
		t.Fatalf("expected no modal with nothing checked: %s", frame)
	}
}

// TestTicketsTUI_ImplementKeyOpensQueueDirectlyWithNoActiveLoop covers ticket
// 10: with no ralph-loop running, "r" ("Replace queue", renamed from "i")
// shows no confirmation — the tab-switch command it now returns instead is
// verified separately at the Model level (implement_test.go), since this
// isolated harness has no app shell to route it.
func TestTicketsTUI_ImplementKeyOpensQueueDirectlyWithNoActiveLoop(t *testing.T) {
	t.Parallel()
	root := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".scratch", "my-epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".scratch", "my-epic", "issues", "01-first.md"), []byte("Status: open\n\nBody.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := tickets.NewModel(root, ui.Settings{}, keys.New(nil))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForTicketsText(t, tm, "my-epic")

	tm.Send(tea.KeyPressMsg{Code: 'j', Text: "j"})
	tm.Send(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})

	tm.Send(tea.KeyPressMsg{Code: 'r', Text: "r"})

	frame := tm.CurrentFrame()
	if bytes.Contains(frame, []byte("Open the execution plan")) {
		t.Fatalf("expected no confirm modal with no active loop: %s", frame)
	}
	if bytes.Contains(frame, []byte("Choose the agent")) {
		t.Fatalf("expected no agent picker on the plain 'i' path: %s", frame)
	}
}

func waitForTicketsText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(12*time.Second))
}
