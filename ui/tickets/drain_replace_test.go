package tickets

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestModel_DrainReplaceKeyBlockedWhenNothingChecked covers 02a's "D" guard:
// a live epic with nothing checked has no execution plan to replace with.
func TestModel_DrainReplaceKeyBlockedWhenNothingChecked(t *testing.T) {
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
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || !strings.Contains(notifyMsg.Message, "check at least one ticket") {
		t.Fatalf("cmd() = %#v, want a \"check at least one ticket\" notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation with nothing checked")
	}
}

// TestModel_DrainReplaceKeyOpensConfirmationNamingEpic covers 02a's "D"
// happy path: a live epic with a checked selection opens the confirmation,
// naming the epic, without touching the registry or the queue yet.
func TestModel_DrainReplaceKeyOpensConfirmationNamingEpic(t *testing.T) {
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
		t.Fatal("expected no cmd until the confirmation is accepted")
	}
	if !m.confirm.IsOpen {
		t.Fatal("expected the confirmation modal to open")
	}
	if !strings.Contains(m.confirm.View(80), `"alpha"`) {
		t.Fatalf("confirm view = %q, want it to name the epic", m.confirm.View(80))
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
		t.Fatal("expected no cmd until the confirmation is accepted")
	}
	if !m.confirm.IsOpen {
		t.Fatal("expected confirmation modal to open")
	}

	agent, ok := ralphLoopRegistry.agentFor("alpha")
	if !ok {
		t.Fatal("expected alpha's driving agent still captured while it's live")
	}

	confirmedMsg := cmdConfirmDrainReplace(m.worktreeRoot, "alpha", agent)().(drainReplaceConfirmedMsg)
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
