package tickets

import (
	"context"
	"strings"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// actionStartSession hands a map epic or one of its open tickets to the
// Agents tab's create form. Handled by dispatchMenuAction;
// starting a session writes no ticket status.
const actionStartSession = "start-session"

const startSessionLabel = "Start session…"

const goToSessionLabel = "Go to design session…"

// latestSession is the ID of the most recently changed session whose Session
// ticket matches; "" when none does. Exited and Stopped sessions count.
func latestSession(sessions []wire.SessionInfo, matches func(ticket string) bool) string {
	var best *wire.SessionInfo
	for i := range sessions {
		s := &sessions[i]
		if s.Ticket != "" && matches(s.Ticket) && (best == nil || s.StatusChangedAt.After(best.StatusChangedAt)) {
			best = s
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

// epicSession is the session linked to the epic or any of its tickets.
func epicSession(sessions []wire.SessionInfo, epicAddress string) string {
	return latestSession(sessions, func(ticket string) bool {
		return ticket == epicAddress || strings.HasPrefix(ticket, epicAddress+"/")
	})
}

// ticketSession is the session linked to the ticket, or to its epic while the
// ticket is claimed.
func ticketSession(sessions []wire.SessionInfo, epicAddress string, ticket tickets.Ticket) string {
	claimed := strings.EqualFold(strings.TrimSpace(ticket.Status), "claimed")
	return latestSession(sessions, func(t string) bool {
		return t == ticket.Path || (claimed && t == epicAddress)
	})
}

// queueAgentOf is the Queue agent row running address's ticket, "" when none.
func queueAgentOf(sessions []wire.SessionInfo, address string) string {
	for _, s := range sessions {
		if s.Queue && s.Ticket == address {
			return s.ID
		}
	}
	return ""
}

func goToSessionItem(id string) contextmenu.Item[ticketTarget] {
	return contextmenu.Item[ticketTarget]{
		Label: goToSessionLabel, Group: contextmenu.Suggested,
		Run: func(ticketTarget) tea.Cmd { return func() tea.Msg { return tabapi.GoToSessionMsg{ID: id} } },
	}
}

// startSessionCmd builds the request for address ("project:epic" or
// "project:epic/NN"); the session is named after the short address. It runs in
// the map's design worktree when the server hands one out, else in the
// project's trunk checkout, resolved from storePath off the UI thread. A
// refusal opens no form.
func startSessionCmd(sessions tabapi.SessionsAPI, storePath, address string) tea.Cmd {
	project, short, _ := strings.Cut(address, ":")
	epic, _, _ := strings.Cut(address, "/")
	return func() tea.Msg {
		var dir, note, agent string
		madeWorktree := false
		if sessions != nil {
			res, err := sessions.DesignWorktree(context.Background(), wire.DesignWorktreeRequest{Epic: epic})
			switch {
			case err == nil && res.Refused:
				return notify.Error("start session refused: " + res.Message)()
			case err == nil:
				dir, note, agent, madeWorktree = res.Path, res.Note, res.Harness, res.Path != ""
			case !apiclient.IsNotRunning(err):
				return notify.Error("start session: " + err.Error())()
			}
		}
		if dir == "" {
			dir = ticketgit.TrunkCheckout(storePath, project)
		}
		return tabapi.StartSessionMsg{
			Project:      project,
			Dir:          dir,
			Name:         short,
			Prompt:       harness.SkillCommand(agent, "gx-design", address),
			Ticket:       address,
			RebaseNote:   note,
			MadeWorktree: madeWorktree,
		}
	}
}
