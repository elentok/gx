package tickets

// Epic is one immediate subdirectory of `.scratch/`. Discovery is dumb: an
// epic is counted regardless of which files exist inside it (spec.md,
// map.md, only issues/, or nothing yet).
type Epic struct {
	Name    string
	Path    string
	IsMap   bool   // has a map.md (wayfinder map)
	MapBody string // map.md's raw content, only set when IsMap
	Tickets []Ticket

	// WorktreeName is the owning worktree's directory name, set only in
	// `gx tickets --all` aggregation (empty for the single-worktree view).
	WorktreeName string
}

// TotalCount is the epic's total ticket count.
func (e Epic) TotalCount() int {
	return len(e.Tickets)
}

// OpenCount is how many of the epic's tickets are not done.
func (e Epic) OpenCount() int {
	open := 0
	for _, t := range e.Tickets {
		if !t.IsDone() {
			open++
		}
	}
	return open
}

// DoneCount is how many of the epic's tickets are done.
func (e Epic) DoneCount() int {
	return e.TotalCount() - e.OpenCount()
}

// AllDone reports whether every one of the epic's tickets is done. A
// zero-ticket epic is not considered "all done" — it starts expanded, not
// collapsed, since "nothing here yet" is distinct from "everything closed".
func (e Epic) AllDone() bool {
	if len(e.Tickets) == 0 {
		return false
	}
	return e.OpenCount() == 0
}
