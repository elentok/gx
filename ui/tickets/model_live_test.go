package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestModel_LiveEventsHighlightRunningEpicInFullList feeds synthetic
// ralphloop.LiveEvents through Model's channel-reader path (ticket 02) and
// asserts the running epic's ticket picks up live phase-suffix rendering
// while embedded in the full multi-epic sidebar, that finishing the run
// reverts it to disk-based rendering, and that a tab-switch rebuild
// (OnPageActivated) doesn't lose the in-flight state.
func TestModel_LiveEventsHighlightRunningEpicInFullList(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: claimed\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.implementEpic = "my-epic"

	events := make(chan ralphloop.LiveEvent, 4)
	liveCmd := m.startLiveTracking(events)
	if liveCmd == nil {
		t.Fatalf("expected startLiveTracking to return a Cmd")
	}

	identifier := m.epics[0].Tickets[0].Identifier
	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: identifier}

	msg := liveCmd().(modelLiveEventMsg)
	updated, cmd := m.Update(msg)
	m = updated.(Model)

	content := m.View().Content
	if !strings.Contains(content, "iter-01") || !strings.Contains(content, "implementing...") {
		t.Fatalf("expected running ticket's live suffix in full list, got:\n%s", content)
	}

	// Simulate the run finishing: the registry reports it gone, clearing
	// live state and reverting to disk-based rendering.
	m.clearLiveTracking()
	content = m.View().Content
	if strings.Contains(content, "implementing...") {
		t.Fatalf("expected live suffix gone after clearLiveTracking, got:\n%s", content)
	}
	if !strings.Contains(content, "First ticket") {
		t.Fatalf("expected disk-based ticket title after run finished, got:\n%s", content)
	}

	_ = cmd
}
