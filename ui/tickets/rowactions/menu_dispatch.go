package rowactions

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// dispatchMenuAction runs an accepted "m" menu action for either tab, keyed by
// the row's address (result.Path). Orchestration writes go only through the
// server.
func dispatchMenuAction(api tabapi.ServerAPI, admin tabapi.AdminAPI, reads tabapi.ReadsAPI, sessions tabapi.SessionsAPI, tix tabapi.TicketsAPI, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
	if word, ok := RecoveryWord(result.Action); ok {
		return cmdServerWrite(admin, tix, word, result.Path)
	}
	switch result.Action {
	case ActionResume, ActionUnmuteReopen, serverVerbPause, serverVerbCancel:
		return cmdServerWrite(admin, tix, result.Action, result.Path)
	case ActionMerge:
		if tix == nil {
			return notify.Warning(ServerNotRunning)
		}
		return CmdEpicMerge(tix, result.EpicName)
	case ActionStashMerge:
		if tix == nil {
			return notify.Warning(ServerNotRunning)
		}
		return CmdEpicStashMerge(tix, result.EpicName)
	case ActionStartSession:
		return StartSessionCmd(sessions, settings.TicketStore, result.Path)
	case ActionInvestigate:
		return InvestigateCmd(api, sessions, result.Path)
	case ActionTranscript:
		file, address := ResolveTicketPath(settings.TicketStore, result.Path)
		return CmdTranscript(settings.Dirs, file, address, width)
	case ActionWatch:
		return CmdWatchAgent(reads, settings.Dirs, AgentAddress(result.Path, result.TicketID))
	case ActionGoToPane:
		return CmdServerFocusPane(reads, result.Path)
	}
	return nil
}
