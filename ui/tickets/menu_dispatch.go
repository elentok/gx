package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

// serverNotRunning is the toast for an orchestration write with no server.
const serverNotRunning = "server not running"

// dispatchMenuAction runs an accepted "m" menu action for either tab, keyed by
// the row's address (result.Path). Orchestration writes go only through the
// server.
func dispatchMenuAction(api ServerAPI, sessions SessionsAPI, tix TicketsAPI, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
	if word, ok := recoveryWord(result.Action); ok {
		return cmdServerWrite(api, tix, word, result.Path)
	}
	switch result.Action {
	case actionResume, actionUnmuteReopen, serverVerbPause, serverVerbCancel:
		return cmdServerWrite(api, tix, result.Action, result.Path)
	case actionMerge:
		if tix == nil {
			return notify.Warning(serverNotRunning)
		}
		return cmdEpicMerge(tix, result.EpicName)
	case actionStashMerge:
		if tix == nil {
			return notify.Warning(serverNotRunning)
		}
		return cmdEpicStashMerge(tix, result.EpicName)
	case actionStartSession:
		return startSessionCmd(sessions, settings.TicketStore, result.Path)
	case actionInvestigate:
		return investigateCmd(api, sessions, result.Path)
	case actionTranscript:
		file, address := resolveTicketPath(settings.TicketStore, result.Path)
		return cmdTranscript(settings.Dirs, file, address, width)
	case actionWatch:
		return cmdWatchAgent(api, settings.Dirs, agentAddress(result.Path, result.TicketID))
	case actionGoToPane:
		return cmdServerFocusPane(api, result.Path)
	}
	return nil
}
