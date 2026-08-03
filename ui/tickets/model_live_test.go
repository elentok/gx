package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestModel_LiveEventsHighlightRunningEpicInFullList(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: claimed\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	identifier := m.epics[0].Tickets[0].Identifier
	r := newLoopRegistry(1)
	r.tryStart("my-epic", 0, 1)
	r.reduceLiveEvent("my-epic", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: identifier})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("my-epic", nil)
		ralphLoopRegistry = previous
	})
	m.implementEpic = "my-epic"
	m.syncRunSnapshot("my-epic")

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
}

func TestModel_LiveEventDoesNotLeakAcrossEpicsWithSameTicketIdentifier(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "running-epic", "01-first-ticket.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "other-epic", "01-unrelated-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	r := newLoopRegistry(1)
	r.tryStart("running-epic", 0, 1)
	r.reduceLiveEvent("running-epic", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: "01"})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("running-epic", nil)
		ralphLoopRegistry = previous
	})
	m.implementEpic = "running-epic"
	m.syncRunSnapshot("running-epic")

	content := m.View().Content
	if strings.Count(content, "implementing...") != 1 {
		t.Fatalf("expected exactly one running ticket row, got:\n%s", content)
	}
	if !strings.Contains(content, "Unrelated ticket") {
		t.Fatalf("expected other-epic's ticket to keep its disk-based title, got:\n%s", content)
	}
}

func TestModel_OnPageActivatedRecoversTwoConcurrentEpicRuns(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first-ticket.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "epic-b", "01-first-ticket.md", "Status: claimed\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	r := newLoopRegistry(2)
	r.tryStart("epic-a", 0, 1)
	r.tryStart("epic-b", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01a", Identifier: "01"})
	r.reduceLiveEvent("epic-b", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01b", Identifier: "01"})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("epic-a", nil)
		r.finish("epic-b", nil)
		ralphLoopRegistry = previous
	})

	// Reactivating the tab (as a tab switch away and back would do) must
	// recover both concurrently running epics from the registry's snapshots
	// alone — this Model instance never launched either of them.
	cmd := m.OnPageActivated()
	if cmd == nil {
		t.Fatal("OnPageActivated: want a resync cmd with two epics running")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if !m.implementingEpics["epic-a"] || !m.implementingEpics["epic-b"] {
		t.Fatalf("implementingEpics after reactivation = %#v, want both epics tracked", m.implementingEpics)
	}
	content := m.View().Content
	if strings.Count(content, "implementing...") != 2 {
		t.Fatalf("expected both epics' ticket 01 rows running after reactivation, got:\n%s", content)
	}
	if !strings.Contains(content, "iter-01a") || !strings.Contains(content, "iter-01b") {
		t.Fatalf("expected each epic's own live label preserved after reactivation, got:\n%s", content)
	}

	// One epic finishes while the tab is (hypothetically) backgrounded again;
	// the next reactivation must notice epic-a alone finished without losing
	// epic-b's still-running progress.
	r.finish("epic-a", nil)
	cmd = m.OnPageActivated()
	if cmd == nil {
		t.Fatal("OnPageActivated: want a resync cmd after epic-a finished")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.implementingEpics["epic-a"] {
		t.Fatalf("implementingEpics after epic-a finished = %#v, want epic-a cleared", m.implementingEpics)
	}
	if !m.implementingEpics["epic-b"] {
		t.Fatalf("implementingEpics after epic-a finished = %#v, want epic-b still tracked", m.implementingEpics)
	}
}

func TestModel_LiveEventsScopedPerEpicWithConcurrentSameNumberedTickets(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first-ticket.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "epic-b", "01-first-ticket.md", "Status: claimed\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	r := newLoopRegistry(2)
	r.tryStart("epic-a", 0, 1)
	r.tryStart("epic-b", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01a", Identifier: "01"})
	r.reduceLiveEvent("epic-b", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01b", Identifier: "01"})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("epic-a", nil)
		r.finish("epic-b", nil)
		ralphLoopRegistry = previous
	})
	m.syncRunSnapshot("epic-a")
	m.syncRunSnapshot("epic-b")

	content := m.View().Content
	if strings.Count(content, "implementing...") != 2 {
		t.Fatalf("expected both epics' ticket 01 rows running independently, got:\n%s", content)
	}
	if !strings.Contains(content, "iter-01a") || !strings.Contains(content, "iter-01b") {
		t.Fatalf("expected each epic's own live label preserved, got:\n%s", content)
	}
}
