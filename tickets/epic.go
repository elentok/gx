package tickets

// Epic is one immediate subdirectory of `.scratch/`. Discovery is dumb: an
// epic is counted regardless of which files exist inside it (spec.md,
// map.md, only issues/, or nothing yet).
type Epic struct {
	Name    string
	Path    string
	IsMap   bool // has a map.md (wayfinder map)
	Tickets []Ticket
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
