package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// dispatchMenuAction runs an accepted "m" menu action for either tab, keyed by
// the row's address (result.Path). Orchestration writes go only through the
// server.
func dispatchMenuAction(api tabapi.ServerAPI, admin tabapi.AdminAPI, reads tabapi.ReadsAPI, sessions tabapi.SessionsAPI, tix tabapi.TicketsAPI, worktreeRoot string, settings ui.Settings, width int, result actionsMenuResult) tea.Cmd {
	if word, ok := rowactions.RecoveryWord(result.Action); ok {
		return cmdServerWrite(admin, tix, word, result.Path)
	}
	switch result.Action {
	case rowactions.ActionResume, rowactions.ActionUnmuteReopen, serverVerbPause, serverVerbCancel:
		return cmdServerWrite(admin, tix, result.Action, result.Path)
	case rowactions.ActionMerge:
		if tix == nil {
			return notify.Warning(rowactions.ServerNotRunning)
		}
		return rowactions.CmdEpicMerge(tix, result.EpicName)
	case rowactions.ActionStashMerge:
		if tix == nil {
			return notify.Warning(rowactions.ServerNotRunning)
		}
		return rowactions.CmdEpicStashMerge(tix, result.EpicName)
	case rowactions.ActionStartSession:
		return rowactions.StartSessionCmd(sessions, settings.TicketStore, result.Path)
	case rowactions.ActionInvestigate:
		return rowactions.InvestigateCmd(api, sessions, result.Path)
	case rowactions.ActionTranscript:
		file, address := rowactions.ResolveTicketPath(settings.TicketStore, result.Path)
		return rowactions.CmdTranscript(settings.Dirs, file, address, width)
	case rowactions.ActionWatch:
		return rowactions.CmdWatchAgent(reads, settings.Dirs, rowactions.AgentAddress(result.Path, result.TicketID))
	case rowactions.ActionGoToPane:
		return rowactions.CmdServerFocusPane(reads, result.Path)
	}
	return nil
}
