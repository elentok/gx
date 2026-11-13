package catalog

import "github.com/elentok/gx/server/servertest"

// cacheGateEpics is the one epic that gives the disk a project to clean.
var cacheGateEpics = []epic{{Project: project, Name: "gate-epic", Tickets: []ticket{
	{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}},
}}}

// The cache-gate scenarios put <gate dir>/bin first on the server's PATH. A
// flow does the same for the gx it starts.
var (
	// cacheGate cleans through the disk control files.
	cacheGate = scenario{Epics: cacheGateEpics, DiskLow: true, FakeGo: true}
	// cacheGateCustom has no project cleanup command, so a
	// --config disk-space.cleanup-command override is the command that runs.
	cacheGateCustom = scenario{Epics: cacheGateEpics, FakeGo: true}
)
