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

// dispatchMenuAction runs an accepted "m" menu action for either tab, keyed by
// the row's address (result.Path). Orchestration writes go only through the
// server; Answer… is the one local edit.
func dispatchMenuAction(api ServerAPI, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
	switch result.Action {
	case actionResume, actionUnmuteReopen, serverVerbPark, serverVerbCancel, serverVerbRelaunch:
		return cmdServerWrite(api, result.Action, result.Path)
	case actionMerge:
		if api == nil {
			return notify.Warning(serverNotRunning)
		}
		return cmdEpicMerge(api, result.EpicName)
	case actionAnswer:
		file, _ := resolveTicketPath(settings.TicketStore, result.Path)
		var resume tea.Cmd
		if api != nil {
			resume = cmdServerResumeAnswered(api, result.Path)
		}
		return cmdAnswerThen(worktreeRoot, settings, file, resume)
	case actionStartSession:
		return startSessionCmd(result.Path)
	case actionInvestigate:
		return cmdLaunchInvestigate(worktreeRoot, result.EpicName, result.TicketID)
	case actionTranscript:
		file, address := resolveTicketPath(settings.TicketStore, result.Path)
		return cmdTranscript(file, address, width)
	case actionWatch:
		return cmdWatchAgent(api, agentAddress(result.Path, result.TicketID))
	case actionGoToPane:
		return cmdServerFocusPane(api, result.Path)
	}
	return nil
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
