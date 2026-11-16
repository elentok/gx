package wire

import "time"

// RestStatus is the Rest engine as a client shows it: one value for the
// snapshot and for every Rest route.
type RestStatus struct {
	// Enabled is the master toggle. Off means no Rest period and no warning.
	Enabled bool `json:"enabled"`
	// Counting is true while an active minute would count.
	Counting bool `json:"counting"`
	// WorkCount is the number of active minutes counted toward the next Break.
	WorkCount int `json:"work_count"`
	// Now is the server's time, so a countdown never uses the browser clock.
	Now time.Time `json:"now"`
	// Zone is the server's IANA zone name, empty when the server runs in an
	// unnamed local zone. ZoneOffsetSeconds is its offset at Now and is always
	// set; clock times print in the server's zone with Zone, or with the offset
	// when Zone is empty.
	Zone              string       `json:"zone,omitempty"`
	ZoneOffsetSeconds int          `json:"zone_offset_seconds"`
	Rule              RestRule     `json:"rule"`
	Period            *RestPeriod  `json:"period,omitempty"`
	Warning           *RestWarning `json:"warning,omitempty"`
}

// RestRule holds the values the rule note prints.
type RestRule struct {
	WorkMinutes  int    `json:"work_minutes"`
	BreakMinutes int    `json:"break_minutes"`
	BedtimeStart string `json:"bedtime_start"`
	BedtimeEnd   string `json:"bedtime_end"`
}

// RestPeriod is the running Rest period.
type RestPeriod struct {
	// Kind is RestKindBreak or RestKindBedtime.
	Kind       string    `json:"kind"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Message    string    `json:"message"`
	SubMessage string    `json:"sub_message,omitempty"`
}

// RestWarning is the one upcoming Rest period.
type RestWarning struct {
	Kind  string    `json:"kind"`
	Start time.Time `json:"start"`
	// BreakMinutes is the Break length, set for a Break warning.
	BreakMinutes int `json:"break_minutes,omitempty"`
}

// Kinds of Rest period.
const (
	RestKindBreak   = "break"
	RestKindBedtime = "bedtime"
)

// RestUnlockRequest carries the typed unlock sentence.
type RestUnlockRequest struct {
	Sentence string `json:"sentence"`
}

// RestResult is the Rest status after a write. A refused write changes nothing
// and has a stable Reason.
type RestResult struct {
	Status  RestStatus `json:"status"`
	Refused bool       `json:"refused,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
}

// Refusal reasons of an unlock. Clients match on them.
const (
	ReasonRestSentenceMismatch = "sentence-mismatch"
	ReasonRestNoPeriod         = "no-rest-period"
)
