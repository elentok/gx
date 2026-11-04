package wire

// LimitSample is one saved reading; times are epoch seconds. UsedPercentage is
// the raw number, not rounded.
type LimitSample struct {
	LimitID        string  `json:"limit_id"`
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
	ReceivedAt     int64   `json:"received_at"`
}
