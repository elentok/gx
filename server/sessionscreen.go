package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/nativerunner"
)

// SessionScreenResponse is a session's screen as plain text rows, without
// colours: the live holder's, or the one saved when the session exited.
type SessionScreenResponse struct {
	Rows []string `json:"rows"`
}

func (s *Server) sessionScreenRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	dir, meta, res, err := s.sessionDir(id)
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
	rows := []string{}
	if meta.Status.State == SessionExited {
		data, err := os.ReadFile(filepath.Join(dir, nativerunner.ScreenFile))
		if err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if text := strings.TrimSuffix(string(data), "\n"); text != "" {
			rows = strings.Split(text, "\n")
		}
	} else {
		// A Starting session's holder has no socket yet, so it has no screen.
		if c, err := nativerunner.DialHolder(dir); err == nil {
			if rows, _, err = c.Screen(0); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	writeJSON(w, SessionScreenResponse{Rows: rows})
}
