package schema

// FrictionSummary is how much a ticket (or, summed, an epic) got in the way of
// itself: pauses, run events and how long people kept it waiting. gx stamps it;
// the stats endpoint reuses the same type.
type FrictionSummary struct {
	// PausesByReason counts pauses by PauseReason.
	PausesByReason map[string]int `yaml:"pauses_by_reason,omitempty" json:"pauses_by_reason,omitempty"`
	// RunEvents counts ralph-loop run events by event type.
	RunEvents map[string]int `yaml:"run_events,omitempty" json:"run_events,omitempty"`
	// PauseEndings counts how pauses ended, by source (a person, gx itself).
	PauseEndings map[string]int `yaml:"pause_endings,omitempty" json:"pause_endings,omitempty"`
	// TotalPausedSecs is the time spent paused, summed over every pause.
	TotalPausedSecs int `yaml:"total_paused_secs,omitempty" json:"total_paused_secs,omitempty"`
	// PersonPauseSecs holds the duration of each pause a person ended.
	PersonPauseSecs []int `yaml:"person_pause_secs,omitempty" json:"person_pause_secs,omitempty"`
}
