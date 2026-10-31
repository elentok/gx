package server

import (
	"net/http"
	"time"
)

// Usage modes: limits once the limits store holds any window, else spending.
// The store never expires a window, so the mode never goes back.
const (
	UsageModeLimits   = "limits"
	UsageModeSpending = "spending"
)

// UsageSpend is the spend block of the usage summary, in dollars. A *Short
// flag is set when the ledger starts after that window's start day, so the
// client writes "since <FirstDay>". FirstDay is omitted for an empty ledger.
type UsageSpend struct {
	Today            float64 `json:"today"`
	SinceSunday      float64 `json:"since_sunday"`
	SinceSundayShort bool    `json:"since_sunday_short,omitempty"`
	SinceFirst       float64 `json:"since_first"`
	SinceFirstShort  bool    `json:"since_first_short,omitempty"`
	FirstDay         string  `json:"first_day,omitempty"`
}

// UsageSummary is the one read the web menu fetches. The spend block is always
// filled; the client picks the block to show by Mode.
type UsageSummary struct {
	Mode   string       `json:"mode"`
	Limits LimitsReport `json:"limits"`
	Spend  UsageSpend   `json:"spend"`
}

func (s *Server) usageSummaryRead(w http.ResponseWriter, _ *http.Request) {
	limits := s.limits.read()
	mode := UsageModeSpending
	if limits.FiveHour != nil || limits.SevenDay != nil {
		mode = UsageModeLimits
	}
	sw := computeSpendWindows(time.Now(), s.ledger.daysCopy())
	writeJSON(w, UsageSummary{
		Mode:   mode,
		Limits: limits,
		Spend: UsageSpend{
			Today:            sw.Today.Total,
			SinceSunday:      sw.Week.Total,
			SinceSundayShort: sw.Week.Since != "",
			SinceFirst:       sw.Month.Total,
			SinceFirstShort:  sw.Month.Since != "",
			FirstDay:         sw.First,
		},
	})
}
