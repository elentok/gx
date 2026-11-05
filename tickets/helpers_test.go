package tickets

import "github.com/elentok/gx/tickets/tickettest"

// Aliases for the shared helpers, so the many call sites in this package's
// tests stay unchanged. Tests moving to other packages call tickettest directly.
var (
	writeFile                       = tickettest.WriteFile
	requireWaitsForProjectMigration = tickettest.RequireWaitsForProjectMigration
	storeProjectWithEpic            = tickettest.StoreProjectWithEpic
)
