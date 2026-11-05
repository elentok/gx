package tickets

// actionsMenuResult is an accepted Context menu action, keyed by the row's
// address (Path), for dispatchMenuAction.
type actionsMenuResult struct {
	Done     bool
	Accepted bool
	Path     string
	EpicName string
	TicketID string
	Action   string
}
