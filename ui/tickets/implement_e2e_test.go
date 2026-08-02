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

// waitForRunningIndicatorGone polls the rendered frame until the "running"
// suffix (added by implementStartedMsg, removed once the poll loop observes
// ralphLoopRegistry go idle) disappears — the tickets tab renders no other
// user-visible signal of
// completion in isolation, since the "ralph-loop finished" notify.Info
// message is only rendered by the app shell's notify bar, not by
// tickets.Model itself.
func waitForRunningIndicatorGone(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !bytes.Contains(tm.CurrentFrame(), []byte("running")) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("running indicator still present after 5s: %s", tm.CurrentFrame())
}

// TestTicketsTUI_ImplementTriggerLaunchesAndCompletes drives ticket 01's
// full trigger -> confirm -> launch -> completion path through a real
// ralphloop.Run call (a zero-ticket epic takes ralphloop's NoTicketsFound
// exit, so Run returns immediately without needing agent tooling or a
// worktree, while still exercising the real background-goroutine launch and
// completion plumbing end to end).
func TestTicketsTUI_ImplementTriggerLaunchesAndCompletes(t *testing.T) {
	root := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".scratch", "my-epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}

	m := tickets.NewModel(root, ui.Settings{}, keys.New(nil))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForTicketsText(t, tm, "my-epic")

	tm.Send(tea.KeyPressMsg{Code: 'i', Text: "i"})
	waitForTicketsText(t, tm, "Choose the agent for epic")
	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("Claude")) || !bytes.Contains(frame, []byte("Codex")) {
		t.Fatalf("agent menu missing Claude or Codex: %s", frame)
	}

	tm.Send(tea.KeyPressMsg{Code: 'l', Text: "l"})
	waitForTicketsText(t, tm, "Start implementing epic")
	if frame := tm.CurrentFrame(); !bytes.Contains(frame, []byte("with Claude")) {
		t.Fatalf("Claude confirmation missing selected agent: %s", frame)
	}

	tm.Send(tea.KeyPressMsg{Code: 'y', Text: "y"})
	waitForTicketsText(t, tm, "running")

	waitForRunningIndicatorGone(t, tm)
}

func TestTicketsTUI_ImplementAgentMenuCodexShortcut(t *testing.T) {
	root := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".scratch", "my-epic", "issues"), 0755); err != nil {
		t.Fatal(err)
	}

	m := tickets.NewModel(root, ui.Settings{}, keys.New(nil))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForTicketsText(t, tm, "my-epic")
	tm.Send(tea.KeyPressMsg{Code: 'i', Text: "i"})
	waitForTicketsText(t, tm, "Choose the agent for epic")

	tm.Send(tea.KeyPressMsg{Code: 'o', Text: "o"})
	waitForTicketsText(t, tm, "with Codex")
}

func waitForTicketsText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(5*time.Second))
}
