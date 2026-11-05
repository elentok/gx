package chat

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the chat endpoints. The getter is called per request, so a
// route can be listed before its Chat is built.
func Routes(get func() *Chat) []wire.Route {
	read := func(h func(*Chat, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(get(), w, r) }
	}
	return []wire.Route{
		{Pattern: "GET /v1/sessions/chat", Web: wire.WebRead, Handler: read((*Chat).read)},
		{Pattern: "GET /v1/sessions/chat/archive", Web: wire.WebRead, Handler: read((*Chat).archiveRead)},
		{Pattern: "GET /v1/sessions/transcript", Handler: read((*Chat).transcriptRead)},
		{Pattern: "GET /v1/sessions/screen", Web: wire.WebRead, Handler: read((*Chat).screenRead)},
		{Pattern: "GET /v1/sessions/commands", Web: wire.WebRead, Handler: read((*Chat).commandsRead)},
		{Pattern: "GET /v1/sessions/permission", Web: wire.WebRead, Handler: read((*Chat).permissionRead)},
	}
}
