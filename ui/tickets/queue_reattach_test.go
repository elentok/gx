package tickets

import (
	"os"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/keys"
)

func TestCmdCheckDetachedLive_DetachedWithLiveClaimedTicket_ReturnsMsg(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	withFakeReattachHerdr(t, func(label string) (string, error) {
		return "ws1", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "epic-iter-01"}}, nil
	})

	msg := cmdCheckDetachedLive(root)()
	got, ok := msg.(queueDetachedLiveMsg)
	if !ok {
		t.Fatalf("cmdCheckDetachedLive() = %#v, want queueDetachedLiveMsg", msg)
	}
	if got.total != 1 || got.alive != 1 {
		t.Fatalf("got total=%d alive=%d, want total=1 alive=1", got.total, got.alive)
	}
	if len(got.epicNames) != 1 || got.epicNames[0] != "epic" {
		t.Fatalf("epicNames = %v, want [epic]", got.epicNames)
	}
}

func TestCmdCheckDetachedLive_ForeignAttached_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	foreignPID := 5252525
	withFakeProcessStartTime(t, map[int]string{foreignPID: "foreign-start-1"})
	scratchDir := scratchDirFor(root)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttachLockFile(t, scratchDir, attachLockInfo{PID: foreignPID, StartTime: "foreign-start-1"})
	withFakeReattachHerdr(t, func(label string) (string, error) {
		return "ws1", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "iter-01"}}, nil
	})

	if msg := cmdCheckDetachedLive(root)(); msg != nil {
		t.Fatalf("cmdCheckDetachedLive() = %#v, want nil while foreign-attached", msg)
	}
}

func TestCmdCheckDetachedLive_DetachedButNothingClaimed_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: open\n\nBody.\n")
	withFakeReattachHerdr(t, func(label string) (string, error) {
		t.Fatal("findWorkspace should not be called when there are no claimed/needs-attention tickets")
		return "", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	})

	if msg := cmdCheckDetachedLive(root)(); msg != nil {
		t.Fatalf("cmdCheckDetachedLive() = %#v, want nil when nothing is claimed/needs-attention", msg)
	}
}

func TestCmdCheckDetachedLive_DetachedButNothingLive_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	withFakeReattachHerdr(t, func(label string) (string, error) {
		return "", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	})

	if msg := cmdCheckDetachedLive(root)(); msg != nil {
		t.Fatalf("cmdCheckDetachedLive() = %#v, want nil when no tab is live", msg)
	}
}

func TestHandleDetachedLiveDetected_OpensConfirm(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{}, keys.Manager{}))

	updated, _ := m.handleDetachedLiveDetected(queueDetachedLiveMsg{epicNames: []string{"epic"}, total: 1, alive: 1})
	nm := updated.(QueueModel)
	if !nm.confirm.IsOpen {
		t.Fatal("handleDetachedLiveDetected: want confirm modal opened")
	}
}

func TestHandleDetachedLiveDetected_DoesNotClobberOpenConfirm(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{}, keys.Manager{}))
	m.confirm = m.confirm.Open(confirm.Options{Prompt: "already open"})

	updated, _ := m.handleDetachedLiveDetected(queueDetachedLiveMsg{epicNames: []string{"epic"}, total: 1, alive: 1})
	nm := updated.(QueueModel)
	if !nm.confirm.IsOpen {
		t.Fatal("handleDetachedLiveDetected: want confirm still open")
	}
}

func TestHandleDetachedLiveConfirmed_QueuesDynamicPlanAndDefaultsAgent(t *testing.T) {
	// Zero available slots so startAvailableEpics (called at the end of
	// handleDetachedLiveConfirmed) can't immediately drain the plan this test
	// asserts on back out of m.pendingEpics.
	previousRegistry := ralphLoopRegistry
	ralphLoopRegistry = newLoopRegistry(0)
	t.Cleanup(func() { ralphLoopRegistry = previousRegistry })

	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "epic", "02-second.md", "Status: open\n\nBody.\n")
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{}, keys.Manager{}))

	updated, _ := m.handleDetachedLiveConfirmed(detachedLiveConfirmedMsg{epicNames: []string{"epic"}})
	nm := updated.(QueueModel)

	if len(nm.pendingEpics) != 1 {
		t.Fatalf("pendingEpics = %d, want 1", len(nm.pendingEpics))
	}
	plan := nm.pendingEpics[0]
	if plan.epic.Name != "epic" || !plan.dynamic {
		t.Fatalf("plan = %+v, want dynamic plan for epic", plan)
	}
	if len(plan.ticketIDs) != 2 {
		t.Fatalf("ticketIDs = %v, want both tickets", plan.ticketIDs)
	}
	if nm.runningAgent != ralphloop.AgentClaude {
		t.Fatalf("runningAgent = %q, want default Claude", nm.runningAgent)
	}
}

func TestCmdCheckStrandedPending_CheckedEpicWithNoRunOrClaim_ReturnsMsg(t *testing.T) {
	// No claimed/needs-attention ticket, and an empty registry (as a freshly
	// started gx process would have): this epic was checked/queued but never
	// got its turn before the process that queued it exited. Simulates the
	// exact "tickets-tree" scenario — a checked epic with only ready-for-agent
	// tickets, silently dropped by a restart because m.pendingEpics is
	// in-memory only.
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: ready-for-agent\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "epic", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	msg := m.cmdCheckStrandedPending()()
	got, ok := msg.(queueStrandedPendingMsg)
	if !ok {
		t.Fatalf("cmdCheckStrandedPending() = %#v, want queueStrandedPendingMsg", msg)
	}
	if len(got.epicNames) != 1 || got.epicNames[0] != "epic" {
		t.Fatalf("epicNames = %v, want [epic]", got.epicNames)
	}
}

func TestCmdCheckStrandedPending_ClaimedTicket_ReturnsNil(t *testing.T) {
	// A claimed ticket belongs to cmdCheckDetachedLive's explicit
	// reattach-confirmation flow instead — cmdCheckStrandedPending must steer
	// clear of it rather than risk double-supervising a still-live herdr
	// session.
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "epic", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	if msg := m.cmdCheckStrandedPending()(); msg != nil {
		t.Fatalf("cmdCheckStrandedPending() = %#v, want nil (claimed ticket routes through cmdCheckDetachedLive instead)", msg)
	}
}

func TestCmdCheckStrandedPending_FullyDoneEpic_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: done\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "epic", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	if msg := m.cmdCheckStrandedPending()(); msg != nil {
		t.Fatalf("cmdCheckStrandedPending() = %#v, want nil (epic already fully done)", msg)
	}
}

func TestCmdCheckStrandedPending_NothingChecked_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: ready-for-agent\n\nBody.\n")
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{}, keys.Manager{}))

	if msg := m.cmdCheckStrandedPending()(); msg != nil {
		t.Fatalf("cmdCheckStrandedPending() = %#v, want nil (nothing checked)", msg)
	}
}

func TestHandleStrandedPendingConfirmed_QueuesCheckedPlanAndDefaultsAgent(t *testing.T) {
	// Zero available slots so startAvailableEpics (called at the end of
	// handleStrandedPendingConfirmed) can't immediately drain the plan this
	// test asserts on back out of m.pendingEpics.
	previousRegistry := ralphLoopRegistry
	ralphLoopRegistry = newLoopRegistry(0)
	t.Cleanup(func() { ralphLoopRegistry = previousRegistry })

	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: ready-for-agent\n\nBody.\n")
	writeTicket(t, root, "epic", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "epic", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.handleStrandedPendingConfirmed(strandedPendingConfirmedMsg{epicNames: []string{"epic"}})
	nm := updated.(QueueModel)

	if len(nm.pendingEpics) != 1 {
		t.Fatalf("pendingEpics = %d, want 1", len(nm.pendingEpics))
	}
	plan := nm.pendingEpics[0]
	// Only the checked ticket resumes — unlike handleDetachedLiveConfirmed's
	// full-epic dynamic plan, a merely-stranded (never-claimed) epic should
	// only pick back up exactly what the user had actually checked.
	if plan.epic.Name != "epic" || plan.dynamic {
		t.Fatalf("plan = %+v, want non-dynamic plan for epic", plan)
	}
	if len(plan.ticketIDs) != 1 {
		t.Fatalf("ticketIDs = %v, want just the checked ticket", plan.ticketIDs)
	}
	if nm.runningAgent != ralphloop.AgentClaude {
		t.Fatalf("runningAgent = %q, want default Claude", nm.runningAgent)
	}
}

func TestHandleDetachedLiveConfirmed_DoesNotOverwriteExistingAgent(t *testing.T) {
	previousRegistry := ralphLoopRegistry
	ralphLoopRegistry = newLoopRegistry(0)
	t.Cleanup(func() { ralphLoopRegistry = previousRegistry })

	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{}, keys.Manager{}))
	m.runningAgent = "codex"

	updated, _ := m.handleDetachedLiveConfirmed(detachedLiveConfirmedMsg{epicNames: []string{"epic"}})
	nm := updated.(QueueModel)
	if nm.runningAgent != "codex" {
		t.Fatalf("runningAgent = %q, want unchanged codex", nm.runningAgent)
	}
}
