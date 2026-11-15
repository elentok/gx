package wire

// BacklogList is the Backlog page's read model: each project with at least one
// backlog, with the epics' open ticket counts. GET /v1/backlog.
type BacklogList struct {
	Projects []BacklogProject `json:"projects"`
}

// BacklogProject is one project's epics. Workspaces are those a Follow-ups card
// gets; the page filters by the active workspace.
type BacklogProject struct {
	Project    string        `json:"project"`
	Workspaces []string      `json:"workspaces"`
	Epics      []BacklogEpic `json:"epics"`
}

// BacklogEpic is one backlog. Open counts its draft and open tickets, the ones a
// Start Design move can take.
type BacklogEpic struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Open    int    `json:"open"`
	// Unfinished is the backlog's unfinished Start Design move, when it has one.
	Unfinished *BacklogUnfinishedMove `json:"unfinished,omitempty"`
}

// BacklogUnfinishedMove is a charting map from a partial Start Design run. Map
// is its address; ToMove counts its listed tickets still in the backlog.
type BacklogUnfinishedMove struct {
	Map    string `json:"map"`
	ToMove int    `json:"to_move"`
}
