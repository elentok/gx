package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// actionMerge triggers the server's manual epic merge, the TUI's
// `gx server epics merge`. Its menu result carries the epic address as
// EpicName.
const actionMerge = "merge"

// cmdEpicMerge asks the server to merge epic ("project:epic") and toasts the
// outcome: a direct merge, the forked merge ticket, a pause, or the refusal.
func cmdEpicMerge(api ServerAPI, epic string) tea.Cmd {
	return func() tea.Msg {
		res, err := api.EpicMerge(context.Background(), epic)
		switch {
		case err != nil:
			return notify.Error("merge " + epic + ": " + err.Error())()
		case res.Refused:
			return notify.Error("merge refused: " + res.Message)()
		case res.Paused:
			return notify.Error("merge paused: " + res.Message)()
		case res.MergeTicket != "":
			return notify.Success("merge ticket " + res.MergeTicket + " forked")()
		}
		return notify.Success(epic + " merged")()
	}
}

// actionStashMerge is the server's "Stash, merge and restore" on an epic paused
// on a dirty trunk. Its menu result carries the epic address as EpicName.
const actionStashMerge = "stash-merge-epic"

// cmdEpicStashMerge asks the server to stash, merge epic ("project:epic") and
// restore the stash, and toasts the outcome.
func cmdEpicStashMerge(api ServerAPI, epic string) tea.Cmd {
	return func() tea.Msg {
		res, err := api.EpicStashMerge(context.Background(), epic)
		switch {
		case err != nil:
			return notify.Error("stash-merge " + epic + ": " + err.Error())()
		case res.Refused:
			return notify.Error("stash-merge refused: " + res.Message)()
		case res.Paused:
			return notify.Error("stash-merge paused: " + res.Message)()
		case res.TrunkConflict != "":
			return notify.Warning("merged; conflicts left in " + res.TrunkConflict)()
		}
		return notify.Success(epic + " merged")()
	}
}

// isPausedRoot is a queued epic whose tickets are all done: its merge paused,
// so the server kept the root in the queue.
func isPausedRoot(epic tickets.Epic, queued map[string]bool) bool {
	if !epic.AllDone() {
		return false
	}
	for _, t := range epic.Tickets {
		if queued[t.Path] {
			return true
		}
	}
	return false
}
