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
	"GET /v1/handshake":                     "server status",
	"GET /v1/snapshot":                      "server snapshot",
	"GET /v1/events":                        "server tickets follow",
	"GET /v1/projects":                      "project list",
	"POST /v1/projects/add":                 "project add",
	"POST /v1/projects/remove":              "project remove",
	"POST /v1/projects/set-path":            "project set-path",
	"GET /v1/locks":                         "server locks",
	"GET /v1/budget":                        "budget status",
	"POST /v1/budget/override":              "budget override",
	"POST /v1/config/reload":                "server reload-config",
	"GET /v1/config/status":                 "server config-status",
	"POST /v1/budget/increase":              "budget increase",
	"POST /v1/afk":                          "afk",
	"GET /v1/tickets/history":               "server tickets history",
	"GET /v1/tickets/explain":               "server tickets explain",
	"GET /v1/tickets/detail":                "server tickets detail",
	"GET /v1/iterations":                    "server iterations",
	"POST /v1/tickets/changed":              "server tickets changed",
	"POST /v1/cpu-spike":                    "server cpu-spike",
	"GET /v1/queue":                         "server queue list",
	"GET /v1/queue/items":                   "server queue items",
	"POST /v1/queue/add":                    "server queue add",
	"POST /v1/oneoff":                       "server one-off",
	"POST /v1/queue/remove":                 "server queue remove",
	"POST /v1/queue/move":                   "server queue move",
	"POST /v1/queue/replace":                "server queue replace",
	"POST /v1/queue/pause":                  "server queue pause",
	"POST /v1/queue/resume":                 "server queue resume",
	"POST /v1/queue/drain":                  "server queue drain",
	"POST /v1/tickets/merge":                "server tickets merge",
	"POST /v1/tickets/start-over":           "server tickets start-over",
	"POST /v1/tickets/resume":               "server tickets resume",
	"POST /v1/tickets/unpark":               "server tickets unpark",
	"POST /v1/tickets/pause":                "server tickets pause",
	"POST /v1/tickets/verify":               "server tickets verify",
	"POST /v1/tickets/cancel":               "server tickets cancel",
	"POST /v1/tickets/park":                 "server tickets park",
	"POST /v1/tickets/unmute":               "server tickets unmute",
	"POST /v1/tickets/approve":              "server tickets approve",
	"POST /v1/tickets/relaunch":             "server tickets relaunch",
	"POST /v1/tickets/nudge":                "server tickets nudge",
	"POST /v1/tickets/answer":               "tickets answer",
	"POST /v1/tickets/ask":                  "tickets ask",
	"POST /v1/agents/prompt":                "server agents prompt",
	"POST /v1/agents/interrupt":             "server agents interrupt",
	"POST /v1/agents/answer":                "server agents answer",
	"POST /v1/epics/merge":                  "server epics merge",
	"GET /v1/sessions":                      "agents list",
	"GET /v1/sessions/chat":                 "agents chat",
	"GET /v1/sessions/screen":               "agents screen",
	"POST /v1/sessions/create":              "agents create",
	"POST /v1/sessions/stop":                "agents stop",
	"POST /v1/sessions/remove":              "agents remove",
	"POST /v1/sessions/mark-read":           "agents mark-read",
	"POST /v1/sessions/focus":               "agents report-focus",
	"POST /v1/sessions/show":                "agents focus",
	"POST /v1/sessions/actionable":          "agent actionable",
	"POST /v1/sessions/link":                "agent link",
	"POST /v1/sessions/step":                "agent step",
	"POST /v1/sessions/run-action":          "agents run-action",
	"POST /v1/sessions/resume":              "agents resume",
	"POST /v1/sessions/prompt":              "agents prompt",
	"POST /v1/sessions/interrupt":           "agents interrupt",
	"POST /v1/sessions/auto-advance":        "agents auto-advance",
	"POST /v1/sessions/auto-advance/cancel": "agents auto-advance-cancel",
	"POST /v1/designs/open":                 "designs open",
	"POST /v1/designs/delete":               "designs delete",
	"POST /v1/tickets/fix-agent":            "agents fix",
	"GET /v1/epics":                         "server epics",
	"GET /v1/epics/detail":                  "server epics detail",
	"GET /v1/stats":                         "server stats",
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
