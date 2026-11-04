package wire

// SessionCommandEntry is one row of the command menu. Name has no leading
// "/" or "$".
type SessionCommandEntry struct {
	Name        string   `json:"name"`
	ArgHint     string   `json:"arg_hint"`
	Description string   `json:"description"`
	Source      string   `json:"source"` // built-in, skill or mcp
	Aliases     []string `json:"aliases"`
}

// SessionCommandsResponse is the menu for one session or project dir. Degraded
// means the harness did not answer and Entries are only the built-ins.
type SessionCommandsResponse struct {
	Entries  []SessionCommandEntry `json:"entries"`
	Degraded bool                  `json:"degraded"`
}
