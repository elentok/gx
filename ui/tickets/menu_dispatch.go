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
func dispatchMenuAction(api ServerAPI, link ServerLink, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
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
		return cmdAnswerThen(worktreeRoot, settings, file, answerResume(api, link, result.Path))
	case actionStartSession:
		return startSessionCmd(settings.TicketStore, result.Path)
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

// answerResume is the resume step after an answer: nil with no server or the
// link down, so the answer is saved and nothing tries to unpark.
func answerResume(api ServerAPI, link ServerLink, address string) tea.Cmd {
	if api == nil || link == ServerLinkDown {
		return nil
	}
	return cmdServerResumeAnswered(api, address)
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
