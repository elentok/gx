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
func dispatchMenuAction(api ServerAPI, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
	if word, ok := recoveryWord(result.Action); ok {
		return cmdServerWrite(api, word, result.Path)
	}
	switch result.Action {
	case actionResume, actionUnmuteReopen, serverVerbPause, serverVerbCancel:
		return cmdServerWrite(api, result.Action, result.Path)
	case actionMerge:
		if api == nil {
			return notify.Warning(serverNotRunning)
		}
		return cmdEpicMerge(api, result.EpicName)
	case actionStashMerge:
		if api == nil {
			return notify.Warning(serverNotRunning)
		}
		return cmdEpicStashMerge(api, result.EpicName)
	case actionStartSession:
		return startSessionCmd(api, settings.TicketStore, result.Path)
	case actionInvestigate:
		return investigateCmd(api, result.Path)
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
