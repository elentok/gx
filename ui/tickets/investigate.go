package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/notify"
)

// investigateCmd starts the server's Fix agent for address and lands on its
// Agents-tab row. A second start finds the existing session by its name; any
// other refusal is a toast.
func investigateCmd(api ServerAPI, address string) tea.Cmd {
	if api == nil {
		return notify.Warning(serverNotRunning)
	}
	return func() tea.Msg {
		ctx := context.Background()
		res, err := api.FixAgent(ctx, server.FixAgentRequest{Address: address})
		switch {
		case err != nil:
			return notify.Error("investigate: " + err.Error())()
		case res.Refused && res.Reason == server.ReasonSessionNameTaken:
			if id := fixAgentSession(ctx, api, address); id != "" {
				return GoToSessionMsg{ID: id}
			}
			return notify.Error("investigate: " + res.Message)()
		case res.Refused:
			return notify.Error("investigate: " + res.Message)()
		case res.Session == nil:
			return nil
		}
		return GoToSessionMsg{ID: res.Session.ID}
	}
}

// fixAgentSession is the live session fixing address, "" when none is found.
func fixAgentSession(ctx context.Context, api ServerAPI, address string) string {
	snap, err := api.Snapshot(ctx)
	if err != nil {
		return ""
	}
	name := server.FixAgentName(address)
	for _, s := range snap.Sessions {
		if s.Name == name && s.Status.State != server.SessionExited {
			return s.ID
		}
	}
	return ""
}
