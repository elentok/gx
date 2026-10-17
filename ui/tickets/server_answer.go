package tickets

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// focusIterationTab is a package-level seam so tests can fake herdr.
var focusIterationTab = func(tabID string) error {
	_, err := herdr.TabFocus(tabID)
	return err
}

// serverIteration is address's live iteration from the server's iterations read.
func (m Model) serverIteration(address string) (server.IterationInfo, bool) {
	list, err := m.serverAPI.Iterations(context.Background())
	if err != nil {
		return server.IterationInfo{}, false
	}
	for _, it := range list {
		if it.Address == address {
			return it, true
		}
	}
	return server.IterationInfo{}, false
}

// ticketFile locates address's ticket file in the store.
func ticketFile(store, address string) (string, error) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return "", err
	}
	matches, _ := filepath.Glob(filepath.Join(store, a.Project, a.Epic, "issues", a.ID+"-*.md"))
	if len(matches) != 1 {
		return "", fmt.Errorf("no ticket file for %s under %s", address, store)
	}
	return matches[0], nil
}

// resolveTicketPath maps a row's Path, an address in server mode and a file
// otherwise, to its ticket file. address is "" for a file.
func resolveTicketPath(store, path string) (file, address string) {
	if f, err := ticketFile(store, path); err == nil {
		return f, path
	}
	return path, ""
}

// serverAnswerActionCmd dispatches the "m" menu's server-mode entries. Resume
// is the server's unpark; Answer… edits the file directly and pings the server
// before resuming; Watch opens a live native agent's log, else focuses the
// iteration tab the iterations read names. ok is false for any other action.
func (m Model) serverAnswerActionCmd(result actionsMenuResult) (tea.Cmd, bool) {
	address := result.Path
	switch result.Action {
	case actionResumeAnswered:
		return m.cmdServerWrite(serverVerbUnpark, address), true
	case actionAnswer:
		path, err := ticketFile(m.settings.TicketStore, address)
		if err != nil {
			return notify.Error("answer: " + err.Error()), true
		}
		return cmdAnswerThen(m.worktreeRoot, m.settings, path, m.cmdServerResumeAnswered(address)), true
	case actionWatch:
		// Liveness shells out to ps, so it is resolved inside the Cmd.
		watch, focus := cmdWatchAgent(address), m.cmdServerFocusPane(address)
		return func() tea.Msg {
			if nativeAgentLive(address) {
				return watch()
			}
			return focus()
		}, true
	}
	return nil, false
}

// cmdServerResumeAnswered pings the server about the answer just written, then
// unparks.
func (m Model) cmdServerResumeAnswered(address string) tea.Cmd {
	api := m.serverAPI
	unpark := m.cmdServerWrite(serverVerbUnpark, address)
	return func() tea.Msg {
		_ = api.TicketChanged(context.Background(), address)
		return unpark()
	}
}

func (m Model) cmdServerFocusPane(address string) tea.Cmd {
	return func() tea.Msg {
		it, live := m.serverIteration(address)
		if !live || it.Tab == "" {
			return notify.Warning("the agent is no longer live")()
		}
		if err := focusIterationTab(it.Tab); err != nil {
			return notify.Error("focus pane: " + err.Error())()
		}
		return nil
	}
}
