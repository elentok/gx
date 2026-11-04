package server

import (
	"cmp"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/elentok/gx/server/wire"
)

// focusReports holds the latest report of each connected TUI.
type focusReports struct {
	mu sync.Mutex
	// streams counts each client's open event streams; a reconnect can
	// overlap the stream it replaces.
	streams map[string]int
	reports map[string]wire.FocusReport
	// lastFocused is the client focused most recently; banner clicks go to it.
	lastFocused string
	// lastTerm outlives its client's disconnect, so a banner sent with no TUI
	// still activates the terminal the person last used.
	lastTerm string
}

func newFocusReports() *focusReports {
	return &focusReports{streams: map[string]int{}, reports: map[string]wire.FocusReport{}}
}

func (f *focusReports) connect(client string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams[client]++
}

// disconnect drops the client's report with its last stream.
func (f *focusReports) disconnect(client string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams[client]--; f.streams[client] > 0 {
		return
	}
	delete(f.streams, client)
	delete(f.reports, client)
}

// report saves r, reporting false when its client has no open stream.
func (f *focusReports) report(r wire.FocusReport) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams[r.Client] == 0 {
		return false
	}
	f.reports[r.Client] = r
	// Web reports never move the click target.
	if r.Focused && !r.Web {
		f.lastFocused = r.Client
		f.lastTerm = cmp.Or(r.TermProgram, f.lastTerm)
	}
	return true
}

// focusedClient is the most recently focused TUI, or empty when it has
// disconnected.
func (f *focusReports) focusedClient() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, connected := f.reports[f.lastFocused]; !connected {
		return ""
	}
	return f.lastFocused
}

// clickTarget is the terminal of the most recently focused TUI and whether
// that TUI is still connected.
func (f *focusReports) clickTarget() (termProgram string, connected bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, connected = f.reports[f.lastFocused]
	return f.lastTerm, connected
}

// onScreen reports whether a focused TUI or web tab has the session on screen.
func (f *focusReports) onScreen(session string) bool {
	return f.anyFocused(session, false)
}

// webOnScreen reports whether a visible web tab has the session on screen.
func (f *focusReports) webOnScreen(session string) bool {
	return f.anyFocused(session, true)
}

func (f *focusReports) anyFocused(session string, webOnly bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reports {
		if r.Focused && r.Session == session && (r.Web || !webOnly) {
			return true
		}
	}
	return false
}

func (s *Server) sessionFocusWrite(w http.ResponseWriter, r *http.Request) {
	s.focusWrite(w, r, false)
}

// sessionFocusWriteWeb is sessionFocusWrite for the web mux: the report is a
// web tab's.
func (s *Server) sessionFocusWriteWeb(w http.ResponseWriter, r *http.Request) {
	s.focusWrite(w, r, true)
}

func (s *Server) focusWrite(w http.ResponseWriter, r *http.Request, web bool) {
	var req wire.FocusReport
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Web = web
	res, err := s.reportFocus(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// reportFocus saves a TUI's focus report; a focused TUI viewing a session
// reads it.
func (s *Server) reportFocus(req wire.FocusReport) (wire.SessionResult, error) {
	if req.Client == "" {
		return sessionRefusal(wire.ReasonSessionBadRequest, "a focus report needs a client"), nil
	}
	if req.Session != "" {
		if _, _, res, err := s.sessionDir(req.Session); res.Refused || err != nil {
			return res, err
		}
	}
	if !s.focus.report(req) {
		return sessionRefusal(wire.ReasonFocusNotConnected, "client "+req.Client+" has no open event stream"), nil
	}
	if !req.Focused || req.Session == "" {
		return wire.SessionResult{}, nil
	}
	return s.markSessionRead(wire.SessionRequest{ID: req.Session})
}

// focusSession asks the most recently focused TUI to show the session. With no
// TUI connected it does nothing: the banner click has already brought the
// terminal forward.
func (s *Server) focusSession(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	info := meta.info()
	client := s.focus.focusedClient()
	if client != "" {
		s.events.PublishEvent(wire.Event{Type: wire.EventSessionFocus, Address: req.ID, Client: client})
	}
	return wire.SessionResult{Session: &info, Client: client}, nil
}

// markSessionRead clears the session's Unread.
func (s *Server) markSessionRead(req wire.SessionRequest) (wire.SessionResult, error) {
	dir, _, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	var info wire.SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		changed := m.Unread != wire.SessionUnread{}
		m.Unread = wire.SessionUnread{}
		info = m.info()
		return changed
	}); err != nil {
		return wire.SessionResult{}, err
	}
	return wire.SessionResult{Session: &info}, nil
}
