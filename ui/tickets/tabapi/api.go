// Package tabapi is the exported surface of the Tickets and Queue tabs that the
// app shell and the Agents tab use: the server client interfaces, the link
// state, the messages the tabs send up, and the view-model bridge. It is a
// leaf: it imports no tickets tab package except rowfmt.
package tabapi

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// ServerAPI is the slice of apiclient.Client the tab uses in server mode: the
// snapshot and event stream. A nil ServerAPI means the tab reads the store.
type ServerAPI interface {
	Snapshot(ctx context.Context) (wire.Snapshot, error)
	Events(ctx context.Context, since uint64) (<-chan wire.Event, error)
}

// AdminAPI is the slice of *apiclient.Admin the tickets and queue tabs use:
// the budget override, the repair verbs and the Default harness.
type AdminAPI interface {
	BudgetOverride(ctx context.Context) (wire.BudgetResult, error)
	Repair(ctx context.Context, verb string, req wire.RepairRequest) (wire.RepairResult, error)
	DefaultHarness(ctx context.Context) (wire.DefaultHarnessStatus, error)
	SetDefaultHarness(ctx context.Context, req wire.SetDefaultHarnessRequest) (wire.SetDefaultHarnessResult, error)
}

// QueueAPI is the slice of *apiclient.Queue the tickets and queue tabs use.
type QueueAPI interface {
	Items(ctx context.Context) ([]wire.QueueItem, error)
	Add(ctx context.Context, address, agent string) (wire.QueueResult, error)
	Replace(ctx context.Context, project string, items []wire.QueueItem) (wire.QueueResult, error)
	Remove(ctx context.Context, address string) (wire.QueueResult, error)
	Pause(ctx context.Context) (wire.QueueResult, error)
	Resume(ctx context.Context) (wire.QueueResult, error)
}

// ReadsAPI is the slice of *apiclient.Reads the tickets and queue tabs use.
type ReadsAPI interface {
	Budget(ctx context.Context) (wire.BudgetStatus, error)
	Detail(ctx context.Context, address string) (wire.TicketDetail, error)
	Iterations(ctx context.Context) ([]wire.IterationInfo, error)
}

// TicketsAPI is the slice of *apiclient.Tickets the tickets and queue tabs use:
// the ticket and epic writes behind the "m" menu and the status changes.
type TicketsAPI interface {
	Pause(ctx context.Context, address, reason string) (wire.QueueResult, error)
	Cancel(ctx context.Context, address string, stop bool) (wire.QueueResult, error)
	Relaunch(ctx context.Context, address string, stop bool) (wire.QueueResult, error)
	Unmute(ctx context.Context, address string) (wire.QueueResult, error)
	Approve(ctx context.Context, address string) (wire.QueueResult, error)
	EpicMerge(ctx context.Context, address string) (wire.QueueResult, error)
	EpicStashMerge(ctx context.Context, address string) (wire.QueueResult, error)
	Changed(ctx context.Context, address string) error
}

// SessionsAPI is the slice of *apiclient.Sessions the tickets and queue tabs
// use: the design worktree behind "start session" and the Fix agent.
type SessionsAPI interface {
	DesignWorktree(ctx context.Context, req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error)
	FixAgent(ctx context.Context, req wire.FixAgentRequest) (wire.FixAgentResult, error)
}

// OlderServerNotice replaces every tab's ticket data when the server predates
// rendered status and epic summaries. There is no client-side fallback: the TUI
// and the server ship in one binary.
const OlderServerNotice = "older server — update the server"

// ServerLink is how the Tickets tab currently reaches the server. The zero
// value is "no restriction" (up).
type ServerLink int

const (
	ServerLinkUp ServerLink = iota
	// ServerLinkDown: the server is unreachable.
	ServerLinkDown
	// ServerLinkReadOnly: API version mismatch; the client must not write.
	ServerLinkReadOnly
)

// DisabledReason is why server keys are off; empty when they are usable.
func (l ServerLink) DisabledReason() string {
	switch l {
	case ServerLinkDown:
		return "server is down"
	case ServerLinkReadOnly:
		return "server is read-only — restart gx"
	}
	return ""
}

// ProbeRequestedMsg asks the app shell, which owns the connection, to check the
// server now (the Queue tab's "R" and a finished server start).
type ProbeRequestedMsg struct{}

// ResnapshotRequestedMsg asks the app shell, which owns the server stream, for
// a fresh snapshot (the tab's "R" and the reloads after a write).
type ResnapshotRequestedMsg struct{}

// StoreChangedMsg tells the tab the ticket store changed on disk; the app shell
// sends it while the server is down.
type StoreChangedMsg struct{}

// StartSessionMsg asks the app shell to open the Agents tab's create form
// pre-filled for a map epic or ticket. The fields mirror create.Prefill,
// which this package can't import.
type StartSessionMsg struct {
	Project, Dir, Name, Prompt, Ticket string
	// RebaseNote and MadeWorktree come from the design-worktree route: the
	// session shows the note and gx answers Claude's trust dialog in Dir.
	RebaseNote   string
	MadeWorktree bool
}

// GoToSessionMsg asks the app shell to switch to the Agents tab with session ID
// selected.
type GoToSessionMsg struct{ ID string }
