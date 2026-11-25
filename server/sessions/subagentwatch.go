package sessions

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
)

// subagentWatches are the open subagent page watches, by the client that
// opened them.
type subagentWatches struct {
	mu sync.Mutex
	by map[string]map[[2]string]*chat.SubagentWatch // client -> (session, call)
}

// put keeps w for the client, ending the watch it replaces.
func (s *subagentWatches) put(client string, key [2]string, w *chat.SubagentWatch) {
	s.mu.Lock()
	if s.by == nil {
		s.by = map[string]map[[2]string]*chat.SubagentWatch{}
	}
	if s.by[client] == nil {
		s.by[client] = map[[2]string]*chat.SubagentWatch{}
	}
	old := s.by[client][key]
	s.by[client][key] = w
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func (s *subagentWatches) end(client string, key [2]string) {
	s.mu.Lock()
	w := s.by[client][key]
	delete(s.by[client], key)
	s.mu.Unlock()
	if w != nil {
		w.Close()
	}
}

// endClient ends every watch of a client whose last event stream closed.
func (s *subagentWatches) endClient(client string) {
	s.mu.Lock()
	ws := s.by[client]
	delete(s.by, client)
	s.mu.Unlock()
	for _, w := range ws {
		w.Close()
	}
}

// SubagentWatchWriteWeb opens or ends the watch of one subagent page.
func (mg *Manager) SubagentWatchWriteWeb(w http.ResponseWriter, r *http.Request) {
	var req subagent.WatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wire.WriteJSON(w, mg.subagentWatch(req))
}

func (mg *Manager) subagentWatch(req subagent.WatchRequest) wire.SessionResult {
	if req.Client == "" || req.Session == "" || req.CallID == "" {
		return Refusal(wire.ReasonSessionBadRequest, "a subagent watch needs a client, a session and a call")
	}
	key := [2]string{req.Session, req.CallID}
	if req.Closed {
		mg.subagentWatches.end(req.Client, key)
		return wire.SessionResult{}
	}
	dir, _, res, err := mg.SessionDir(req.Session)
	if res.Refused || err != nil {
		return res
	}
	if !mg.focus.connected(req.Client) {
		return Refusal(wire.ReasonSubagentWatchUnknownClient, "client "+req.Client+" has no open event stream")
	}
	watch, known := chat.WatchSubagent(mg.chat, req.Session, dir, req.CallID, func() {
		mg.events.PublishEvent(wire.Event{Type: wire.EventSessionSubagentChanged, Address: req.Session, Call: req.CallID, Client: req.Client})
	})
	if !known {
		return Refusal(wire.ReasonUnknownSubagent, "the session has no subagent call "+req.CallID)
	}
	if watch == nil {
		mg.subagentWatches.end(req.Client, key)
		return wire.SessionResult{}
	}
	mg.subagentWatches.put(req.Client, key, watch)
	// The stream may have closed while the watch started.
	if !mg.focus.connected(req.Client) {
		mg.subagentWatches.endClient(req.Client)
	}
	return wire.SessionResult{}
}
