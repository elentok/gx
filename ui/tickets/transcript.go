package tickets

import (
	"fmt"
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
var lookupSession = func(dirs config.Dirs, agentDir, id string) (sessionlookup.Session, error) {
	return sessionlookup.Lookup(sessionlookup.RootsFrom(dirs.Environ()), agentDir, id)
}

// cmdTranscript renders the latest of file's session_ids as markdown.
// address ("" or unparseable skips it) locates the native agent dir.
func cmdTranscript(dirs config.Dirs, file, address string, width int) tea.Cmd {
	return cmdTranscriptAt(dirs, file, address, width, -1)
}

// cmdTranscriptAt renders file's session_ids[index], the latest when index is
// out of range. The ticket is re-read on every step so a session claimed
// since the last refresh shows.
func cmdTranscriptAt(dirs config.Dirs, file, address string, width, index int) tea.Cmd {
	return func() tea.Msg {
		t, err := schema.ParseTicket(file)
		if err != nil {
			return notify.Error("transcript: " + err.Error())()
		}
		n := len(t.SessionIDs)
		if n == 0 {
			return notify.Info("no sessions for this ticket")()
		}
		if index < 0 || index >= n {
			index = n - 1
		}
		id := t.SessionIDs[index]
		s, err := lookupSession(dirs, nativeAgentDir(dirs, address), id)
		if err != nil {
			return notify.Error("transcript: " + err.Error())()
		}
		if s.Source == sessionlookup.Missing {
			return notify.Warning("no transcript found for session " + id)()
		}
		md := renderTicketMarkdown(s.Markdown, watchContentWidth(width))
		step := func(delta int) tea.Cmd {
			next := index + delta
			if next < 0 || next >= n {
				return nil
			}
			return cmdTranscriptAt(dirs, file, address, width, next)
		}
		title := fmt.Sprintf("Transcript %s · session %d/%d", id, index+1, n)
		return watchLoadedMsg{title: title, lines: strings.Split(md, "\n"), step: step}
	}
}

// nativeAgentDir is the native runner's agent dir for address, "" when it
// can't be located.
func nativeAgentDir(dirs config.Dirs, address string) string {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return ""
	}
	stateDir, err := dirs.StateDir()
	if err != nil {
		return ""
	}
	label, _, _ := ralphloop.IterationIdentity(a.Epic, a.ID, "")
	return filepath.Join(nativerunner.AgentsRoot(stateDir, a.Project), label)
}
