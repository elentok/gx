package server

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// AgentEnv names the agent ("claude" or "codex") a ralph-loop run launched
// under, so `gx agent model` answers for that agent.
const AgentEnv = "GX_AGENT"

// requestOrigin is the one place handlers learn a write's origin. web is true
// on the web mux, whose requests never get to claim another origin.
func requestOrigin(r *http.Request, web bool) string {
	switch {
	case web:
		return wire.OriginWeb
	case r.Header.Get(wire.OriginHeader) == wire.OriginTUI:
		return wire.OriginTUI
	default:
		return wire.OriginCLI
	}
}
