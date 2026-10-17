package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

// serverNotRunning is the toast for an orchestration write with no server.
const serverNotRunning = "server not running"

// answerSavedNeedsServer follows a saved answer when there is no server to
// resume the ticket.
const answerSavedNeedsServer = "answer saved — resuming needs the server"

// dispatchMenuAction runs the "m" menu actions both tabs share, keyed by the
// row's address (result.Path). Orchestration writes go only through the
// server; Answer… is the one local edit. ok is false for any other action,
// which the tab dispatches itself.
func dispatchMenuAction(api ServerAPI, worktreeRoot string, settings ui.Settings, result actionsMenuResult) (tea.Cmd, bool) {
	switch result.Action {
	case actionResume, actionUnmuteReopen:
		return cmdServerWrite(api, result.Action, result.Path), true
	case actionMerge:
		if api == nil {
			return notify.Warning(serverNotRunning), true
		}
		return cmdEpicMerge(api, result.EpicName), true
	case actionAnswer:
		file, _ := resolveTicketPath(settings.TicketStore, result.Path)
		var resume tea.Cmd
		if api != nil {
			resume = cmdServerResumeAnswered(api, result.Path)
		}
		return cmdAnswerThen(worktreeRoot, settings, file, resume), true
	}
	return nil, false
}

// cmdServerResumeAnswered pings the server about the answer just written, then
// unparks.
func cmdServerResumeAnswered(api ServerAPI, address string) tea.Cmd {
	unpark := cmdServerWrite(api, serverVerbUnpark, address)
	return func() tea.Msg {
		_ = api.TicketChanged(context.Background(), address)
		return unpark()
	}
}
