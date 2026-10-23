package server

import "net/http"

// Where a write came from. The web mux decides it itself; the socket reads
// OriginHeader, which only the TUI's API client sets.
const (
	OriginHeader = "X-Gx-Origin"
	OriginWeb    = "web"
	OriginTUI    = "tui"
	OriginCLI    = "cli"
)

// requestOrigin is the one place handlers learn a write's origin. web is true
// on the web mux, whose requests never get to claim another origin.
func requestOrigin(r *http.Request, web bool) string {
	switch {
	case web:
		return OriginWeb
	case r.Header.Get(OriginHeader) == OriginTUI:
		return OriginTUI
	default:
		return OriginCLI
	}
}
