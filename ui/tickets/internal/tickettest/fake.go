// Package tickettest holds the test fixtures shared by the ui/tickets packages:
// store writers and a fake server. It must not import ui/tickets.
package tickettest

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// FakeServerAPI is a recording fake of the tab's server client. The sub-client
// wrappers below share its state.
type FakeServerAPI struct {
	Adds *[]wire.QueueItem
	// AddRes is what Queue.Add returns per address.
	AddRes map[string]wire.QueueResult
	// Calls records the ticket write verbs as "verb address" and the
	// maintenance verbs by name.
	Calls *[]string
	// Replaced records the QueueReplace items; ReplaceRes is what it returns.
	Replaced   *[]wire.QueueItem
	ReplaceRes wire.QueueResult
	RemoveRes  wire.QueueResult
	MergeRes   wire.QueueResult
	// DetailRes and DetailErr are what Detail returns for any address.
	DetailRes     wire.TicketDetail
	DetailErr     error
	BudgetRes     wire.BudgetStatus
	IterationsRes []wire.IterationInfo
	// DesignWorktreeRes and DesignWorktreeErr are what DesignWorktree returns.
	DesignWorktreeRes wire.DesignWorktreeResult
	DesignWorktreeErr error
	// FixAgentRes is what FixAgent returns.
	FixAgentRes wire.FixAgentResult
	// Snap and QueueItems are what Snapshot and Queue.Items return.
	Snap       wire.Snapshot
	QueueItems []wire.QueueItem
	// Harness is what DefaultHarness returns; HarnessSets records the
	// SetDefaultHarness requests and HarnessRes is what it returns.
	Harness     wire.DefaultHarnessStatus
	HarnessSets *[]wire.SetDefaultHarnessRequest
	HarnessRes  wire.SetDefaultHarnessResult
}

// FakeAdmin is the Admin sub-client over the same FakeServerAPI state.
type FakeAdmin struct{ FakeServerAPI }

func (f FakeAdmin) DefaultHarness(context.Context) (wire.DefaultHarnessStatus, error) {
	return f.Harness, nil
}

func (f FakeAdmin) SetDefaultHarness(_ context.Context, req wire.SetDefaultHarnessRequest) (wire.SetDefaultHarnessResult, error) {
	if f.HarnessSets != nil {
		*f.HarnessSets = append(*f.HarnessSets, req)
	}
	return f.HarnessRes, nil
}

// verb records the maintenance verbs ("pause", "resume", "override",
// "remove <addr>") so a test can assert which one a key reached.
func (f FakeServerAPI) verb(v string) {
	if f.Calls != nil {
		*f.Calls = append(*f.Calls, v)
	}
}

// FakeQueue and FakeReads are the Queue and Reads sub-clients over the same
// FakeServerAPI state; Pause differs from the Tickets one, so they can't share a type.
type FakeQueue struct{ FakeServerAPI }

type FakeReads struct{ FakeServerAPI }

func (f FakeQueue) Remove(_ context.Context, a string) (wire.QueueResult, error) {
	f.verb("remove " + a)
	return f.RemoveRes, nil
}

func (f FakeQueue) Pause(context.Context) (wire.QueueResult, error) {
	f.verb("pause")
	return wire.QueueResult{Mode: "paused"}, nil
}

func (f FakeQueue) Resume(context.Context) (wire.QueueResult, error) {
	f.verb("resume")
	return wire.QueueResult{Mode: "running"}, nil
}

func (f FakeReads) Budget(context.Context) (wire.BudgetStatus, error) { return f.BudgetRes, nil }

func (f FakeAdmin) BudgetOverride(context.Context) (wire.BudgetResult, error) {
	f.verb("override")
	return wire.BudgetResult{}, nil
}

func (f FakeQueue) Replace(_ context.Context, _ string, items []wire.QueueItem) (wire.QueueResult, error) {
	if f.Replaced != nil {
		*f.Replaced = append(*f.Replaced, items...)
	}
	return f.ReplaceRes, nil
}

func (f FakeQueue) Add(_ context.Context, address, agent string) (wire.QueueResult, error) {
	if f.Adds != nil {
		*f.Adds = append(*f.Adds, wire.QueueItem{Address: address, Agent: agent})
	}
	return f.AddRes[address], nil
}

func (f FakeServerAPI) Pause(_ context.Context, address, _ string) (wire.QueueResult, error) {
	f.record("pause " + address)
	return wire.QueueResult{}, nil
}

func (f FakeServerAPI) Cancel(_ context.Context, address string, _ bool) (wire.QueueResult, error) {
	f.record("cancel " + address)
	return wire.QueueResult{}, nil
}

func (f FakeServerAPI) Relaunch(_ context.Context, address string, _ bool) (wire.QueueResult, error) {
	f.record("relaunch " + address)
	return wire.QueueResult{}, nil
}

func (f FakeServerAPI) Unmute(_ context.Context, address string) (wire.QueueResult, error) {
	f.record("unmute " + address)
	return wire.QueueResult{}, nil
}

func (f FakeServerAPI) Approve(_ context.Context, address string) (wire.QueueResult, error) {
	f.record("approve " + address)
	return wire.QueueResult{}, nil
}

func (f FakeServerAPI) EpicMerge(_ context.Context, address string) (wire.QueueResult, error) {
	f.record("merge " + address)
	return f.MergeRes, nil
}

func (f FakeServerAPI) EpicStashMerge(_ context.Context, address string) (wire.QueueResult, error) {
	f.record("stash-merge " + address)
	return wire.QueueResult{}, nil
}

func (f FakeReads) Detail(context.Context, string) (wire.TicketDetail, error) {
	return f.DetailRes, f.DetailErr
}

func (f FakeAdmin) Repair(_ context.Context, verb string, req wire.RepairRequest) (wire.RepairResult, error) {
	f.record(verb + " " + req.Address)
	return wire.RepairResult{}, nil
}

func (f FakeServerAPI) DesignWorktree(_ context.Context, req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error) {
	f.record("design-worktree " + req.Epic)
	return f.DesignWorktreeRes, f.DesignWorktreeErr
}

func (f FakeServerAPI) FixAgent(_ context.Context, req wire.FixAgentRequest) (wire.FixAgentResult, error) {
	f.record("fix-agent " + req.Address)
	return f.FixAgentRes, nil
}

func (f FakeServerAPI) record(call string) {
	if f.Calls != nil {
		*f.Calls = append(*f.Calls, call)
	}
}

func (f FakeServerAPI) Snapshot(context.Context) (wire.Snapshot, error) {
	return f.Snap, nil
}
func (FakeServerAPI) Events(context.Context, uint64) (<-chan wire.Event, error) { return nil, nil }
func (f FakeQueue) Items(context.Context) ([]wire.QueueItem, error)             { return f.QueueItems, nil }

func (f FakeServerAPI) Changed(_ context.Context, address string) error {
	f.record("changed " + address)
	return nil
}

func (f FakeReads) Iterations(context.Context) ([]wire.IterationInfo, error) {
	return f.IterationsRes, nil
}

// CountingIterationsAPI counts Iterations calls through Calls.
type CountingIterationsAPI struct {
	FakeReads
	Calls *int
}

func (c CountingIterationsAPI) Iterations(ctx context.Context) ([]wire.IterationInfo, error) {
	*c.Calls++
	return c.FakeReads.Iterations(ctx)
}
