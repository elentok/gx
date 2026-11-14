package wire

// BacklogList is the Backlog page's read model: each project with at least one
// area, with the areas' open ticket counts. GET /v1/backlog.
type BacklogList struct {
	Projects []BacklogProject `json:"projects"`
}

// BacklogProject is one project's areas. Workspaces are those a Follow-ups card
// gets; the page filters by the active workspace.
type BacklogProject struct {
	Project    string        `json:"project"`
	Workspaces []string      `json:"workspaces"`
	Areas      []BacklogArea `json:"areas"`
}

// BacklogArea is one area. Open counts its draft and open tickets, the ones a
// Start Design move can take.
type BacklogArea struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Open    int    `json:"open"`
	// Unfinished is the area's unfinished Start Design move, when it has one.
	Unfinished *BacklogUnfinishedMove `json:"unfinished,omitempty"`
}

// BacklogUnfinishedMove is a charting map from a partial Start Design run. Map
// is its address; ToMove counts its listed tickets still in the area.
type BacklogUnfinishedMove struct {
	Map    string `json:"map"`
	ToMove int    `json:"to_move"`
}
