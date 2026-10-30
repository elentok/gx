package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// SessionScreenResponse is a session's screen as plain text rows, without
// colours: the live holder's, or the one saved when the session exited.
type SessionScreenResponse struct {
	Rows []string `json:"rows"`
}

// sessionScreenRead serves a design session's screen (session=) or a loop
// ticket agent's (ticket=).
func (s *Server) sessionScreenRead(w http.ResponseWriter, r *http.Request) {
	if address := r.URL.Query().Get("ticket"); address != "" {
		s.ticketScreenRead(w, address)
		return
	}
	id := r.URL.Query().Get("session")
	dir, meta, res, err := s.readableSession(id)
	if res.Refused {
		status := http.StatusBadRequest
		if res.Reason == ReasonSessionUnknown {
			status = http.StatusNotFound
		}
		http.Error(w, res.Message, status)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeScreen(w, dir, meta.Status.State == SessionExited)
}

func (s *Server) ticketScreenRead(w http.ResponseWriter, address string) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		http.Error(w, "no ticket "+address, http.StatusNotFound)
		return
	}
	dir, ok := s.ticketPtyDir(addr)
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

// ticketPtyDir is the agent directory of addr's latest run, when that agent
// ran in a pty. Other runners have no screen.
func (s *Server) ticketPtyDir(addr tickets.Address) (string, bool) {
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	dir := filepath.Join(nativerunner.AgentsRoot(s.cfg.StateDir, addr.Project), label)
	d, err := nativerunner.OpenDir(dir)
	if err != nil || !d.PTY() {
		return "", false
	}
	return dir, true
}

// savedRows is the screen a pty agent saved when it ended; none yet gives no rows.
func savedRows(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, nativerunner.ScreenFile))
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
	} else if c, derr := nativerunner.DialHolder(dir); derr == nil {
		rows, _, err = c.Screen(0)
	}
	writeRows(w, rows, err)
}

// liveRows is the screen of the holder serving dir.
func liveRows(dir string) ([]string, error) {
	c, err := nativerunner.DialHolder(dir)
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
	writeJSON(w, SessionScreenResponse{Rows: rows})
}
