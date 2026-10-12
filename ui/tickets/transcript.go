package tickets

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
)

// actionTranscript opens the ticket's latest session in the Watch agent
// modal. Offered on every row; dispatched by the menu handlers, which know
// the modal's width.
const actionTranscript = "transcript"

// lookupSession is a package-level seam so tests can fake the session lookup.
var lookupSession = func(agentDir, id string) (sessionlookup.Session, error) {
	return sessionlookup.Lookup(sessionlookup.DefaultRoots(), agentDir, id)
}

// cmdTranscript renders the latest of file's session_ids as markdown. The
// ticket is re-read so a session claimed since the last refresh shows.
// address ("" or unparseable skips it) locates the native agent dir.
func cmdTranscript(file, address string, width int) tea.Cmd {
	return func() tea.Msg {
		t, err := schema.ParseTicket(file)
		if err != nil {
			return notify.Error("transcript: " + err.Error())()
		}
		if len(t.SessionIDs) == 0 {
			return notify.Info("no sessions for this ticket")()
		}
		id := t.SessionIDs[len(t.SessionIDs)-1]
		s, err := lookupSession(nativeAgentDir(address), id)
		if err != nil {
			return notify.Error("transcript: " + err.Error())()
		}
		if s.Source == sessionlookup.Missing {
			return notify.Warning("no transcript found for session " + id)()
		}
		md := renderTicketMarkdown(s.Markdown, watchContentWidth(width))
		return watchLoadedMsg{title: "Transcript " + id, lines: strings.Split(md, "\n")}
	}
}

// nativeAgentDir is the native runner's agent dir for address, "" when it
// can't be located.
func nativeAgentDir(address string) string {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return ""
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return ""
	}
	label, _, _ := ralphloop.IterationIdentity(a.Epic, a.ID, "")
	return filepath.Join(nativerunner.AgentsRoot(stateDir, a.Project), label)
}
