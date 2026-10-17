package tickets

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/components"
)

// actionStartSession hands a map epic or one of its open tickets to the
// Agents tab's create form. Handled by handleActionsMenuKey;
// starting a session writes no ticket status.
const actionStartSession = "start-session"

var startSessionMenuItem = components.MenuItem{Label: "Start session…", Value: actionStartSession}

// StartSessionMsg asks the app shell to open the Agents tab's create form
// pre-filled for a map epic or ticket. The fields mirror agents.CreatePrefill,
// which this package can't import.
type StartSessionMsg struct {
	Project, Name, Prompt, Ticket string
}

// startSessionCmd builds the request for address ("project:epic" or
// "project:epic/NN"); the session is named after the short address.
func startSessionCmd(address string) tea.Cmd {
	project, short, _ := strings.Cut(address, ":")
	return func() tea.Msg {
		return StartSessionMsg{Project: project, Name: short, Prompt: "/gx-design " + address, Ticket: address}
	}
}
