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

// iterationsSnapshot is a ServerAPI whose Iterations read is answered from one
// earlier fetch, so a caller that looks up several things in a row hits the
// server once.
type iterationsSnapshot struct {
	ServerAPI
	list []server.IterationInfo
	err  error
}

func (s iterationsSnapshot) Iterations(context.Context) ([]server.IterationInfo, error) {
	return s.list, s.err
}

// snapshotIterations fetches api's iterations once. Nil stays nil: no server.
func snapshotIterations(api ServerAPI) ServerAPI {
	if api == nil {
		return nil
	}
	list, err := api.Iterations(context.Background())
	return iterationsSnapshot{ServerAPI: api, list: list, err: err}
}

// serverIteration is address's live iteration from the server's iterations read.
func serverIteration(api ServerAPI, address string) (server.IterationInfo, bool) {
	if api == nil {
		return server.IterationInfo{}, false
	}
	list, err := api.Iterations(context.Background())
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

// cmdServerFocusPane focuses the herdr tab the iterations read names for
// address.
func cmdServerFocusPane(api ServerAPI, address string) tea.Cmd {
	return func() tea.Msg {
		it, live := serverIteration(api, address)
		if !live || it.Tab == "" {
			return notify.Warning("the agent is no longer live")()
		}
		if err := focusIterationTab(it.Tab); err != nil {
			return notify.Error("focus pane: " + err.Error())()
		}
		return nil
	}
}
