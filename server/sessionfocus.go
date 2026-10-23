package server

import (
	"cmp"
	"encoding/json"
	"net/http"
	"sync"
)

// ReasonFocusNotConnected refuses a focus report from a client with no open
// event stream: the report would never expire.
const ReasonFocusNotConnected = "not-connected"

// EventSessionFocus asks a TUI to show a session; its Address is the session
// id and its Client the TUI.
const EventSessionFocus = "session-focus"

// FocusReport is a TUI's terminal focus, the session it has on screen (empty
// for none) and its terminal app. Client names the TUI's event stream
// (/v1/events?client=); the report counts only while that stream is open.
type FocusReport struct {
	Client      string `json:"client"`
	Focused     bool   `json:"focused,omitempty"`
	Session     string `json:"session,omitempty"`
	TermProgram string `json:"term_program,omitempty"`
	// Web marks a web tab's report. Only the web mux sets it, never the body.
	Web bool `json:"-"`
}

// focusReports holds the latest report of each connected TUI.
type focusReports struct {
	mu sync.Mutex
	// streams counts each client's open event streams; a reconnect can
	// overlap the stream it replaces.
	streams map[string]int
	reports map[string]FocusReport
	// lastFocused is the client focused most recently; banner clicks go to it.
	lastFocused string
	// lastTerm outlives its client's disconnect, so a banner sent with no TUI
	// still activates the terminal the person last used.
	lastTerm string
}

func newFocusReports() *focusReports {
	return &focusReports{streams: map[string]int{}, reports: map[string]FocusReport{}}
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
func (f *focusReports) report(r FocusReport) bool {
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
	var req FocusReport
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
func (s *Server) reportFocus(req FocusReport) (SessionResult, error) {
	if req.Client == "" {
		return sessionRefusal(ReasonSessionBadRequest, "a focus report needs a client"), nil
	}
	if req.Session != "" {
		if _, _, res, err := s.sessionDir(req.Session); res.Refused || err != nil {
			return res, err
		}
	}
	if !s.focus.report(req) {
		return sessionRefusal(ReasonFocusNotConnected, "client "+req.Client+" has no open event stream"), nil
	}
	if !req.Focused || req.Session == "" {
		return SessionResult{}, nil
	}
	return s.markSessionRead(SessionRequest{ID: req.Session})
}

// focusSession asks the most recently focused TUI to show the session. With no
// TUI connected it does nothing: the banner click has already brought the
// terminal forward.
func (s *Server) focusSession(req SessionRequest) (SessionResult, error) {
	_, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	info := meta.info()
	client := s.focus.focusedClient()
	if client != "" {
		s.events.publishEvent(Event{Type: EventSessionFocus, Address: req.ID, Client: client})
	}
	return SessionResult{Session: &info, Client: client}, nil
}

// markSessionRead clears the session's Unread.
func (s *Server) markSessionRead(req SessionRequest) (SessionResult, error) {
	dir, _, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	var info SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		changed := m.Unread != SessionUnread{}
		m.Unread = SessionUnread{}
		info = m.info()
		return changed
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}
