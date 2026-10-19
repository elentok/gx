package tickets

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
)

// actionStartSession hands a map epic or one of its open tickets to the
// Agents tab's create form. Handled by dispatchMenuAction;
// starting a session writes no ticket status.
const actionStartSession = "start-session"

const startSessionLabel = "Start session…"

// StartSessionMsg asks the app shell to open the Agents tab's create form
// pre-filled for a map epic or ticket. The fields mirror agents.CreatePrefill,
// which this package can't import.
type StartSessionMsg struct {
	Project, Dir, Name, Prompt, Ticket string
}

// startSessionCmd builds the request for address ("project:epic" or
// "project:epic/NN"); the session is named after the short address and runs in
// the project's trunk checkout, resolved from storePath off the UI thread.
func startSessionCmd(storePath, address string) tea.Cmd {
	project, short, _ := strings.Cut(address, ":")
	return func() tea.Msg {
		return StartSessionMsg{
			Project: project,
			Dir:     tickets.TrunkCheckout(storePath, project),
			Name:    short,
			Prompt:  "/gx-design " + address,
			Ticket:  address,
		}
	}
}
