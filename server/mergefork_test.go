package server_test

import "github.com/elentok/gx/server/servertest"

// atMergeCap adds two done merge tickets to an epic's tickets: its run has
// used both, so its next non-ff parks the root instead of forking.
func atMergeCap(opts map[string]servertest.TicketOpts) map[string]servertest.TicketOpts {
	opts["02"] = servertest.TicketOpts{Type: "merge", Status: "done"}
	opts["03"] = servertest.TicketOpts{Type: "merge", Status: "done"}
	return opts
}
