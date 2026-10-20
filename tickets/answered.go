package tickets

import "github.com/elentok/gx/tickets/schema"

// Answered is the derived "answered" flag: the ticket is open and its latest
// question has an answer, so it waits for gx to claim it again. It is never
// stored; claiming clears it because the status leaves open.
func Answered(status schema.Status, events []LogEvent) bool {
	if status != schema.StatusOpen {
		return false
	}
	rounds := Rounds(events)
	n := len(rounds)
	return n > 0 && rounds[n-1].End != nil && rounds[n-1].End.Type == EventParkAnswer
}
