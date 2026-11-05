package explain_test

import "github.com/elentok/gx/server/servertest"

// Aliases for the servertest helpers the moved root tests call.
var (
	asHeadlessRun     = servertest.AsHeadlessRun
	leaveIteration    = servertest.LeaveIteration
	markWaiting       = servertest.MarkWaiting
	stampRun          = servertest.StampRun
	startTicketWrites = servertest.StartTicketWrites
	waitFor           = servertest.Await
)
