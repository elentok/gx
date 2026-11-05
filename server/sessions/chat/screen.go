package chat

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// screenRead serves a design session's screen (session=) or a loop ticket
// agent's (ticket=).
func (c *Chat) screenRead(w http.ResponseWriter, r *http.Request) {
	if address := r.URL.Query().Get("ticket"); address != "" {
		c.ticketScreenRead(w, address)
		return
	}
	dir, meta, ok := c.readable(w, r.URL.Query().Get("session"))
	if !ok {
		return
	}
	writeScreen(w, dir, meta.Status.State == wire.SessionExited)
}

func (c *Chat) ticketScreenRead(w http.ResponseWriter, address string) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		http.Error(w, "no ticket "+address, http.StatusNotFound)
		return
	}
	dir, ok := c.TicketPtyDir(addr)
	if !ok {
		http.Error(w, "no screen for "+address, http.StatusNotFound)
		return
	}
	// The holder is gone once the agent ended, and then only the saved screen is
	// left. It can also go between the dial and the read, so any failure to read
	// the live screen falls back to the saved one.
	rows, err := liveRows(dir)
	if err != nil {
		rows, err = savedRows(dir)
	}
	writeRows(w, rows, err)
}

// TicketPtyDir is the agent directory of addr's latest run, when that agent
// ran in a pty. Other runners have no screen.
func (c *Chat) TicketPtyDir(addr tickets.Address) (string, bool) {
	label, _, _ := worktree.IterationIdentity(addr.Epic, addr.ID, "")
	dir := filepath.Join(nativerunner.AgentsRoot(c.cfg.StateDir, addr.Project), label)
	d, err := nativerunner.OpenDir(dir)
	if err != nil || !d.PTY() {
		return "", false
	}
	return dir, true
}

// savedRows is the screen a pty agent saved when it ended; none yet gives no rows.
func savedRows(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, holder.ScreenFile))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if text := strings.TrimSuffix(string(data), "\n"); text != "" {
		return strings.Split(text, "\n"), nil
	}
	return []string{}, nil
}

// writeScreen answers with dir's saved screen when saved is set, else with the
// live holder's. A holder that is not up yet (Starting) has no screen.
func writeScreen(w http.ResponseWriter, dir string, saved bool) {
	rows, err := []string{}, error(nil)
	if saved {
		rows, err = savedRows(dir)
	} else if c, derr := holder.DialHolder(dir); derr == nil {
		rows, _, err = c.Screen(0)
	}
	writeRows(w, rows, err)
}

// liveRows is the screen of the holder serving dir.
func liveRows(dir string) ([]string, error) {
	c, err := holder.DialHolder(dir)
	if err != nil {
		return nil, err
	}
	rows, _, err := c.Screen(0)
	return rows, err
}

func writeRows(w http.ResponseWriter, rows []string, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, wire.SessionScreenResponse{Rows: rows})
}
