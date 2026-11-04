package wire

import (
	"time"
)

// FinishedEpic is one entry of the snapshot's Recently finished list.
type FinishedEpic struct {
	Address     string    `json:"address"`
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Done        int       `json:"done"`
	Total       int       `json:"total"`
	CompletedAt time.Time `json:"completed_at"`
	// OneOff marks a ticket entry: Address is the ticket's, Done and Total are 0.
	OneOff bool `json:"one_off"`
}
