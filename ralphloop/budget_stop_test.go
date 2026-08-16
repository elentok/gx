package ralphloop

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// stopDeps builds a Deps whose AgentSendKeys/TabClose/Sleep are recorded and
// controllable, for StopIterationAndMarkNeedsRepair's tests.
func stopDeps(sendErr, closeErr error) (d Deps, sentKeys *[]string, closedTabs *[]string, slept *[]time.Duration) {
	sentKeys = &[]string{}
	closedTabs = &[]string{}
	slept = &[]time.Duration{}
	d = Deps{
		AgentSendKeys: func(target string, keys ...string) error {
			*sentKeys = append(*sentKeys, target)
			return sendErr
		},
		TabClose: func(tabID string) error {
			*closedTabs = append(*closedTabs, tabID)
			return closeErr
		},
		Sleep: func(dur time.Duration) { *slept = append(*slept, dur) },
	}
	return d, sentKeys, closedTabs, slept
}

func TestStopIterationAndMarkNeedsRepair_QuietIteration_ClosedAndMarked(t *testing.T) {
	t.Parallel()
	path := writeFrontmatterTicket(t, "claimed")
	ticket := tickets.Ticket{Identifier: "01", Path: path}
	d, sentKeys, closedTabs, slept := stopDeps(nil, nil)

	grace := 15 * time.Second
	if err := StopIterationAndMarkNeedsRepair(d, ticket, "pane-1", "tab-1", grace, "budget hard limit reached"); err != nil {
		t.Fatalf("StopIterationAndMarkNeedsRepair: %v", err)
	}

	if got := *sentKeys; len(got) != 1 || got[0] != "pane-1" {
		t.Errorf("AgentSendKeys targets = %v, want [pane-1]", got)
	}
	if got := *closedTabs; len(got) != 1 || got[0] != "tab-1" {
		t.Errorf("TabClose ids = %v, want [tab-1]", got)
	}
	if got := *slept; len(got) != 1 || got[0] != grace {
		t.Errorf("Sleep durations = %v, want [%v]", got, grace)
	}
	if got := mustParse(t, path).Status; got != schema.StatusNeedsRepair {
		t.Errorf("Status = %q, want %q", got, schema.StatusNeedsRepair)
	}
}

func TestStopIterationAndMarkNeedsRepair_StuckIteration_ForceClosedAndMarked(t *testing.T) {
	t.Parallel()
	path := writeFrontmatterTicket(t, "claimed")
	ticket := tickets.Ticket{Identifier: "01", Path: path}
	// AgentSendKeys fails to quiet the pane down; the seam still waits out
	// grace and closes the pane unconditionally.
	d, _, closedTabs, _ := stopDeps(errors.New("pane never quieted"), nil)

	if err := StopIterationAndMarkNeedsRepair(d, ticket, "pane-1", "tab-1", 15*time.Second, "budget hard limit reached"); err == nil {
		t.Fatal("StopIterationAndMarkNeedsRepair: want error surfaced from AgentSendKeys, got nil")
	}

	if got := *closedTabs; len(got) != 1 || got[0] != "tab-1" {
		t.Errorf("TabClose ids = %v, want [tab-1] even though AgentSendKeys failed", got)
	}
	if got := mustParse(t, path).Status; got != schema.StatusNeedsRepair {
		t.Errorf("Status = %q, want %q", got, schema.StatusNeedsRepair)
	}
}

func TestStopIterationAndMarkNeedsRepair_LandedDuringGrace_ClosedButLeftDone(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: done\nactual_cost: 0.42\ntype: task\n---\n# Ticket\n\nBody text.\n")
	ticket := tickets.Ticket{Identifier: "01", Path: path}
	d, _, closedTabs, _ := stopDeps(nil, nil)

	if err := StopIterationAndMarkNeedsRepair(d, ticket, "pane-1", "tab-1", 15*time.Second, "budget hard limit reached"); err != nil {
		t.Fatalf("StopIterationAndMarkNeedsRepair: %v", err)
	}

	if got := *closedTabs; len(got) != 1 || got[0] != "tab-1" {
		t.Errorf("TabClose ids = %v, want [tab-1]", got)
	}
	if got := mustParse(t, path).Status; got != schema.StatusDone {
		t.Errorf("Status = %q, want %q (landed ticket must not be overwritten)", got, schema.StatusDone)
	}
}
