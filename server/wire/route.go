package wire

import (
	"encoding/json"
	"net/http"
)

// WebMode says how the web mux treats a Route.
type WebMode int

const (
	// WebNone: the web mux does not serve the route.
	WebNone WebMode = iota
	// WebRead: a read the web mux serves with Handler.
	WebRead
	// WebWrite: a write the web mux serves with WebHandler, or Handler when
	// WebHandler is nil.
	WebWrite
	// WebPublic: a web-only route served with WebHandler before the login check.
	WebPublic
)

// Route is one endpoint a subsystem serves. The root server gathers them.
type Route struct {
	Pattern    string           // "METHOD /path"
	Handler    http.HandlerFunc // the socket mux; nil for a web-only route
	Web        WebMode
	WebHandler http.HandlerFunc // the web variant of a write, or nil
}

// WriteJSON writes v as a JSON response body.
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
