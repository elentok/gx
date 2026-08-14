package tickets

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// TestModel_DrainReplaceKeyBlockedWhenCursorEpicNotRunning covers 02a's "D"
// guard: an epic under the cursor with no live run can't be drained, mirroring
// TestModel_ImplementKeyBlockedWhenCursorEpicRunning's guard-path style.
func TestModel_DrainReplaceKeyBlockedWhenCursorEpicNotRunning(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{"/alpha/01.md": true}}

	updated, cmd := m.handleDrainReplaceKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || !strings.Contains(notifyMsg.Message, "isn't running") {
		t.Fatalf("cmd() = %#v, want an \"isn't running\" notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation for a non-running epic")
	}
}

// TestModel_DrainReplaceKeyOpensMenuWithOnlyDrainOnlyWhenNothingChecked
// covers the drain-choice menu's omission rule: a live epic with nothing
// checked opens the menu, but "Drain and replace..." isn't in it at all.
func TestModel_DrainReplaceKeyOpensMenuWithOnlyDrainOnlyWhenNothingChecked(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{}}

	updated, cmd := m.handleDrainReplaceKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd when opening the menu, got %#v", cmd)
	}
	if !m.drainMenuOpen {
		t.Fatal("expected the drain menu to open")
	}
	if len(m.drainMenu.Items) != 1 || m.drainMenu.Items[0].Value != drainMenuValueDrainOnly {
		t.Fatalf("menu items = %#v, want only \"Drain only\"", m.drainMenu.Items)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation prompt; the menu replaces it")
	}
}

// TestModel_DrainReplaceKeyOpensMenuNamingEpicWithBothItemsWhenChecked
// covers the drain-choice menu's happy path: a live epic with a checked
// selection opens the menu naming the epic in its header, offering both
// items, without touching the registry or the queue yet.
func TestModel_DrainReplaceKeyOpensMenuNamingEpicWithBothItemsWhenChecked(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	r.setAgent("alpha", ralphloop.AgentCodex)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{"/alpha/01.md": true}}

	updated, cmd := m.handleDrainReplaceKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd when opening the menu, got %#v", cmd)
	}
	if !m.drainMenuOpen {
		t.Fatal("expected the drain menu to open")
	}
	if len(m.drainMenu.Items) != 2 {
		t.Fatalf("menu items = %#v, want both \"Drain only\" and \"Drain and replace...\"", m.drainMenu.Items)
	}
	if !strings.Contains(m.drainMenuView(), `"alpha"`) {
		t.Fatalf("drain menu view = %q, want it to name the epic", m.drainMenuView())
	}
}

// TestModel_DrainMenuEscCancelsWithoutDraining covers esc/q's cancel
// behavior on the open menu: nothing is drained, nothing about the queue
// changes.
func TestModel_DrainMenuEscCancelsWithoutDraining(t *testing.T) {
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{drainMenuOpen: true, drainMenuEpic: "alpha", drainMenu: newDrainMenu(true)}

	updated, cmd := m.handleDrainMenuKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd from cancelling, got %#v", cmd)
	}
	if m.drainMenuOpen {
		t.Fatal("expected the drain menu to close")
	}
	gate := r.gateFor("alpha")
	if gate.IsDraining() {
		t.Fatal("expected cancelling the menu not to drain anything")
	}
}

// TestModel_DrainMenuSelectDrainOnlyDrainsWithoutReplacing covers the
// "Drain only" item: it drains the epic's Gate and does nothing else —
// no queue replace, no launch.
func TestModel_DrainMenuSelectDrainOnlyDrainsWithoutReplacing(t *testing.T) {
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{drainMenuOpen: true, drainMenuEpic: "alpha", drainMenu: newDrainMenu(true)}

	updated, cmd := m.handleDrainMenuKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.drainMenuOpen {
		t.Fatal("expected the drain menu to close")
	}
	if cmd == nil {
		t.Fatal("expected a drain cmd")
	}
	msg := cmd()
	if _, ok := msg.(notify.NotifyMsg); !ok {
		t.Fatalf("cmd() = %#v, want a notify message", msg)
	}
	gate := r.gateFor("alpha")
	if !gate.IsDraining() {
		t.Fatal("expected \"Drain only\" to drain alpha's gate")
	}
}

// TestCmdConfirmDrainReplaceDrainsRegisteredGate covers 02a's core mechanism:
// accepting "D"'s confirmation drains the epic's live Gate (blocking further
// claims) without ending the run outright, mirroring loop_registry_test.go's
// same-package access to epicRun/Gate state for assertions.
func TestCmdConfirmDrainReplaceDrainsRegisteredGate(t *testing.T) {
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	gate := r.gateFor("alpha")
	if gate.IsDraining() {
		t.Fatal("precondition: gate should not be draining yet")
	}

	msg := cmdConfirmDrainReplace("/root", "alpha", ralphloop.AgentClaude)()
	confirmedMsg, ok := msg.(drainReplaceConfirmedMsg)
	if !ok || confirmedMsg.epicName != "alpha" || confirmedMsg.worktreeRoot != "/root" {
		t.Fatalf("cmdConfirmDrainReplace() = %#v, want drainReplaceConfirmedMsg{worktreeRoot: /root, epicName: alpha}", msg)
	}
	if !gate.IsDraining() {
		t.Fatal("expected accepting the confirmation to drain alpha's gate")
	}
	if !r.isRunningEpic("alpha") {
		t.Fatal("draining should not immediately end the run")
	}
}

// TestModel_DrainReplacePollFlowLaunchesReplacementWithCapturedAgent covers
// 02a's full "D" combo end-to-end: draining a live epic, waiting out its
// completion via the poll loop, then replacing the queue with the checked
// selection and launching it automatically with the agent captured at
// confirm-open time — no manual action from the user in between. Mirrors
// queue_test.go's runRalphLoop-stubbing/waitForEpicToFinish pattern, and
// drives the poll loop by constructing drainReplacePollMsg directly rather
// than waiting on real tea.Tick timers (implementPollMsg tests' approach).
func TestModel_DrainReplacePollFlowLaunchesReplacementWithCapturedAgent(t *testing.T) {
	root := testutil.TempRepo(t)
	alphaPath := filepath.Join(root, ".scratch", "alpha", "issues", "01-first.md")
	betaPath := filepath.Join(root, ".scratch", "beta", "issues", "01-first.md")
	alpha := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: alphaPath, Status: "open"},
	}}
	beta := tickets.Epic{Name: "beta", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: betaPath, Status: "open"},
	}}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	starts := make(chan ralphloop.RunOptions, 2)
	releases := map[string]chan struct{}{
		"alpha": make(chan struct{}),
		"beta":  make(chan struct{}),
	}
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		starts <- opts
		<-releases[opts.EpicName]
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1) // one slot: beta must wait for alpha's drain to finish
	t.Cleanup(func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	m := Model{
		worktreeRoot: root,
		queueStore:   store,
		epics:        []tickets.Epic{alpha, beta},
		checked:      map[string]bool{betaPath: true},
		checkOrder:   map[string]uint64{betaPath: 1},
	}

	// alpha is already running, as if launched earlier via "i".
	startCmd := m.cmdStartImplement("alpha", ralphloop.AgentCodex, 0, 1)
	if msg, ok := startCmd().(implementFailedMsg); ok {
		t.Fatalf("cmdStartImplement(alpha) failed: %v", msg.err)
	}
	if opts := <-starts; opts.EpicName != "alpha" {
		t.Fatalf("first start = %q, want alpha", opts.EpicName)
	}

	updated, cmd := m.handleDrainReplaceKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no cmd until a menu item is chosen")
	}
	if !m.drainMenuOpen {
		t.Fatal("expected the drain menu to open")
	}
	if len(m.drainMenu.Items) != 2 || m.drainMenu.Items[1].Value != drainMenuValueReplace {
		t.Fatalf("menu items = %#v, want \"Drain and replace...\" as the second item", m.drainMenu.Items)
	}

	agent, ok := ralphLoopRegistry.agentFor("alpha")
	if !ok {
		t.Fatal("expected alpha's driving agent still captured while it's live")
	}

	m.drainMenu.Cursor = 1
	updated, cmd = m.handleDrainMenuKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.drainMenuOpen {
		t.Fatal("expected the drain menu to close")
	}
	if cmd == nil {
		t.Fatal("expected the drain-replace cmd to be armed")
	}
	confirmedMsg := cmd().(drainReplaceConfirmedMsg)
	updated, cmd = m.handleDrainReplaceConfirmed(confirmedMsg)
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected the poll cmd to be armed")
	}

	pollMsg := drainReplacePollMsg{worktreeRoot: root, epicName: "alpha", agent: agent}

	// alpha is still running: the poll loop just re-ticks.
	updated, cmd = m.handleDrainReplacePoll(pollMsg)
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected another poll tick while alpha drains")
	}
	select {
	case opts := <-starts:
		t.Fatalf("epic %q launched before alpha finished draining", opts.EpicName)
	default:
	}

	close(releases["alpha"])
	waitForEpicToFinish(t, "alpha")

	updated, cmd = m.handleDrainReplacePoll(pollMsg)
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected launch cmds once alpha finishes draining")
	}
	m = deliverCmd(t, m, cmd)

	select {
	case opts := <-starts:
		if opts.EpicName != "beta" {
			t.Fatalf("launched epic = %q, want beta", opts.EpicName)
		}
		if opts.Agent != agent {
			t.Fatalf("launched agent = %v, want captured agent %v", opts.Agent, agent)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for beta to launch after alpha's drain completed")
	}
	close(releases["beta"])

	status := store.Snapshot().Status
	if _, ok := status[betaPath]; !ok {
		t.Fatalf("expected beta's checked ticket queued after the drain-replace, status = %v", status)
	}
}
