package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

// routeVerbs maps every API route to the CLI verb that serves it. A route
// missing here fails TestRouteTableHasCLIVerbs: add the verb, then the entry.
var routeVerbs = map[string]string{
	"GET /v1/handshake":          "server status",
	"GET /v1/snapshot":           "server snapshot",
	"GET /v1/events":             "server tickets follow",
	"GET /v1/projects":           "project list",
	"POST /v1/projects/add":      "project add",
	"POST /v1/projects/remove":   "project remove",
	"POST /v1/projects/set-path": "project set-path",
	"GET /v1/locks":              "server locks",
	"GET /v1/budget":             "budget status",
	"POST /v1/budget/override":   "budget override",
	"POST /v1/budget/increase":   "budget increase",
	"GET /v1/tickets/history":    "server tickets history",
	"GET /v1/tickets/explain":    "server tickets explain",
	"GET /v1/iterations":         "server iterations",
	"POST /v1/tickets/changed":   "server tickets changed",
	"GET /v1/queue":              "server queue list",
	"GET /v1/queue/items":        "server queue items",
	"POST /v1/queue/add":         "server queue add",
	"POST /v1/oneoff":            "server one-off",
	"POST /v1/queue/remove":      "server queue remove",
	"POST /v1/queue/move":        "server queue move",
	"POST /v1/queue/replace":     "server queue replace",
	"POST /v1/queue/pause":       "server queue pause",
	"POST /v1/queue/resume":      "server queue resume",
	"POST /v1/queue/drain":       "server queue drain",
	"POST /v1/tickets/land":      "server tickets land",
	"POST /v1/tickets/reset":     "server tickets reset",
	"POST /v1/tickets/unpark":    "server tickets unpark",
	"POST /v1/tickets/verify":    "server tickets verify",
	"POST /v1/tickets/cancel":    "server tickets cancel",
	"POST /v1/tickets/park":      "server tickets park",
	"POST /v1/tickets/approve":   "server tickets approve",
	"POST /v1/tickets/relaunch":  "server tickets relaunch",
	"POST /v1/tickets/nudge":     "server tickets nudge",
	"POST /v1/agents/prompt":     "server agents prompt",
	"POST /v1/agents/interrupt":  "server agents interrupt",
	"POST /v1/agents/answer":     "server agents answer",
}

func TestRouteTableHasCLIVerbs(t *testing.T) {
	t.Parallel()
	root := newRootCmd(deps{})
	patterns := server.RoutePatterns()
	for _, p := range patterns {
		verb, ok := routeVerbs[p]
		if !ok {
			t.Errorf("route %q has no CLI verb in routeVerbs", p)
			continue
		}
		c, _, err := root.Find(strings.Fields(verb))
		if err != nil || c == root || c.CommandPath() != "gx "+verb {
			t.Errorf("route %q: verb %q not found in command tree", p, verb)
			continue
		}
		if c.Flags().Lookup("json") == nil {
			t.Errorf("route %q: verb %q has no --json flag", p, verb)
		}
	}
	for p := range routeVerbs {
		if !slices.Contains(patterns, p) {
			t.Errorf("routeVerbs entry %q is not a server route", p)
		}
	}
}
