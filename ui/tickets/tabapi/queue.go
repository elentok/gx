package tabapi

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/viewmodel"
)

// QueueAddFunc is the queue-add call of the server client.
type QueueAddFunc func(ctx context.Context, address, agent string) (wire.QueueResult, error)

// QueueAddresses sends one queue-add per address, carrying on past refusals so
// one blocked ticket doesn't keep the rest out. It returns how many the server
// took and the refusals as "address: reason".
func QueueAddresses(ctx context.Context, add QueueAddFunc, addrs []string) (queued int, refusals []string) {
	for _, addr := range addrs {
		res, err := add(ctx, addr, "")
		switch {
		case err != nil:
			refusals = append(refusals, addr+": "+err.Error())
		case res.Refused:
			refusals = append(refusals, addr+": "+res.Message)
		default:
			queued++
		}
	}
	return queued, refusals
}

// QueueEpic queues, in address order, the open tickets of epic ("project:epic")
// found in snap, with QueueAddresses' counts.
func QueueEpic(ctx context.Context, snap wire.Snapshot, add QueueAddFunc, epic string) (queued int, refusals []string) {
	prefix := epic + "/"
	var addrs []string
	for _, t := range snap.Tickets {
		if strings.HasPrefix(t.Address, prefix) && t.Status == string(schema.StatusOpen) {
			addrs = append(addrs, t.Address)
		}
	}
	slices.Sort(addrs)
	return QueueAddresses(ctx, add, addrs)
}

// QueuedToast is the toast for a finished queue run: a warning listing the
// refusals, else a success.
func QueuedToast(queued int, refusals []string) tea.Cmd {
	text := fmt.Sprintf("queued %d, refused %d", queued, len(refusals))
	if len(refusals) == 0 {
		return notify.Success(text)
	}
	return notify.Warning(text + " (" + strings.Join(refusals, "; ") + ")")
}

// QueueStatus is the queue at a glance, for the title row: its mode and the
// done/total ticket counts of the epics it is working on.
type QueueStatus struct {
	// Mode is wire.QueueModeRunning or QueueModePaused.
	Mode        wire.QueueMode
	Done, Total int
}

// QueueStatusOf reads the queue's status from the server state; ok is false
// when there is nothing to show: no run is live and the queue is not held
// (paused) with work still in it.
func QueueStatusOf(vm viewmodel.State) (s QueueStatus, ok bool) {
	claimedAt := map[string]time.Time{}
	for _, t := range vm.Tickets {
		if !t.ClaimedAt.IsZero() {
			claimedAt[t.Address] = t.ClaimedAt
		}
	}
	running := false
	for _, epic := range EpicsFromViewModel(vm, viewmodel.Scope{}) {
		live := EpicRunning(epic, claimedAt)
		queued := slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool { return slices.Contains(vm.Queue, t.Path) })
		if !live && !queued {
			continue
		}
		running = running || live
		s.Done += epic.DoneCount()
		s.Total += epic.TotalCount()
	}
	s.Mode = vm.Mode
	if s.Mode == "" {
		s.Mode = wire.QueueModeRunning
	}
	held := s.Mode != wire.QueueModeRunning && len(vm.Queue) > 0
	return s, s.Total > 0 && (running || held)
}

// EpicRunning reports whether the server holds a run for one of epic's
// tickets: a claimed ticket with a claim time.
func EpicRunning(epic gxtickets.Epic, claimedAt map[string]time.Time) bool {
	return slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool {
		return ServerRunning(epic, t, claimedAt)
	})
}

// ServerRunning reports whether the server runs t. No claim time means the
// server holds no run for it: a person claimed it (a grilling session, say),
// so nothing is implementing.
func ServerRunning(epic gxtickets.Epic, t gxtickets.Ticket, claimedAt map[string]time.Time) bool {
	if rowfmt.TicketStatus(epic, t) != gxtickets.StatusClaimed {
		return false
	}
	seen, ok := claimedAt[t.Path]
	return ok && !seen.IsZero()
}
