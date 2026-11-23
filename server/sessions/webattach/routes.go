package webattach

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// heartbeat keeps an idle event stream from being dropped by a proxy.
const heartbeat = 20 * time.Second

// Routes lists the web attach endpoints. All are web only: an attach belongs
// to a browser tab, so there is no CLI verb. The getter is called per
// request, so a route can be listed before its Attaches is built.
func Routes(get func() *Attaches) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/sessions/attach/input", Web: wire.WebWrite, WebHandler: func(w http.ResponseWriter, r *http.Request) { get().inputWrite(w, r) }},
		{Pattern: "POST /v1/sessions/attach/takeover", Web: wire.WebWrite, WebHandler: func(w http.ResponseWriter, r *http.Request) { get().takeOverWrite(w, r) }},
		{Pattern: "POST /v1/sessions/attach/detach", Web: wire.WebWrite, WebHandler: func(w http.ResponseWriter, r *http.Request) { get().detachWrite(w, r) }},
		{Pattern: "GET /v1/sessions/attach/events/", Web: wire.WebReadPrefix, WebHandler: func(w http.ResponseWriter, r *http.Request) { get().eventsRead(w, r) }},
	}
}

func decode(w http.ResponseWriter, r *http.Request) (wire.SessionAttachRequest, bool) {
	var req wire.SessionAttachRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func (as *Attaches) inputWrite(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	var (
		res wire.SessionResult
		err error
	)
	switch {
	case req.Key != nil && req.Paste == "":
		res, err = as.Key(req.ID, req.Client, screen.Key{Code: rune(req.Key.Code), Text: req.Key.Text, Mod: screen.KeyMod(req.Key.Mod)})
	case req.Paste != "" && req.Key == nil:
		res, err = as.Paste(req.ID, req.Client, req.Paste)
	default:
		res = sessions.Refusal(wire.ReasonSessionBadRequest, "send exactly one of key and paste")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (as *Attaches) takeOverWrite(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	res, err := as.TakeOver(req.ID, req.Client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (as *Attaches) detachWrite(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	as.Detach(req.ID, req.Client)
	wire.WriteJSON(w, wire.SessionResult{})
}

// eventsRead streams the client's attach events as server-sent events until
// the attach ends or the page goes away. It opens the attach if the client has
// none, and ends with a "closed" event when the attach is gone.
func (as *Attaches) eventsRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	at, res, err := as.open(q.Get("id"), q.Get("client"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if res.Refused {
		http.Error(w, res.Message, http.StatusConflict)
		return
	}
	defer as.watch(at)()
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	beat := time.NewTicker(heartbeat)
	defer beat.Stop()
	write := func(s string) bool {
		if _, err := io.WriteString(w, s); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		case ev := <-at.events:
			data, _ := json.Marshal(ev)
			if !write(fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data)) {
				return
			}
		case <-at.done:
			// The reader queues an exit before it ends; send it first.
			select {
			case ev := <-at.events:
				data, _ := json.Marshal(ev)
				write(fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data))
			default:
			}
			write("event: closed\ndata: {}\n\n")
			return
		}
	}
}
