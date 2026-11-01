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

// UsageLimitWindow is a limit window as the menu shows it. Chip and Forecast
// are set on weekly limits only; Chip is absent until a full day of history
// exists.
type UsageLimitWindow struct {
	LimitWindow `tstype:",extends"`
	Chip        *LimitBurnChip `json:"chip,omitempty"`
	Forecast    *LimitForecast `json:"forecast,omitempty"`
}

// UsageLimits is the 5-hour and weekly window of one provider.
type UsageLimits struct {
	FiveHour *UsageLimitWindow `json:"five_hour,omitempty"`
	SevenDay *UsageLimitWindow `json:"seven_day,omitempty"`
}

// UsageSummary is the one read the web menu fetches. The spend block is always
// filled; the client picks the block to show by Mode. Codex is set only once
// the history holds a Codex sample.
type UsageSummary struct {
	Mode   string       `json:"mode"`
	Limits UsageLimits  `json:"limits"`
	Codex  *UsageLimits `json:"codex,omitempty"`
	Spend  UsageSpend   `json:"spend"`
}

// usageLimitWindow builds a row from a window and, for a weekly limit, the
// history of limit id.
func (s *Server) usageLimitWindow(w LimitWindow, id string, weekly bool, now time.Time) *UsageLimitWindow {
	row := &UsageLimitWindow{LimitWindow: w}
	if !weekly {
		return row
	}
	samples := s.limitHistory.Samples(id)
	row.Chip = limitBurnChip(samples, now)
	latest, ok := s.limitHistory.Latest(id)
	if !ok {
		latest = LimitSample{UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt, ReceivedAt: w.ReceivedAt}
	}
	f := limitForecast(samples, latest, now)
	row.Forecast = &f
	return row
}

func (s *Server) codexSummary(now time.Time) *UsageLimits {
	var out UsageLimits
	for _, c := range []struct {
		id     string
		weekly bool
		dst    **UsageLimitWindow
	}{
		{LimitIDCodex5h, false, &out.FiveHour},
		{LimitIDCodexWeekly, true, &out.SevenDay},
	} {
		latest, ok := s.limitHistory.Latest(c.id)
		if !ok {
			continue
		}
		w := LimitWindow{UsedPercentage: latest.UsedPercentage, ResetsAt: latest.ResetsAt, ReceivedAt: latest.ReceivedAt}
		*c.dst = s.usageLimitWindow(w, c.id, c.weekly, now)
	}
	if out.FiveHour == nil && out.SevenDay == nil {
		return nil
	}
	return &out
}

func (s *Server) usageSummaryRead(w http.ResponseWriter, _ *http.Request) {
	limits := s.limits.read()
	mode := UsageModeSpending
	if limits.FiveHour != nil || limits.SevenDay != nil {
		mode = UsageModeLimits
	}
	now := time.Now()
	var claude UsageLimits
	if limits.FiveHour != nil {
		claude.FiveHour = s.usageLimitWindow(*limits.FiveHour, LimitIDClaude5h, false, now)
	}
	if limits.SevenDay != nil {
		claude.SevenDay = s.usageLimitWindow(*limits.SevenDay, LimitIDClaudeWeekly, true, now)
	}
	sw := computeSpendWindows(now, s.ledger.daysCopy())
	writeJSON(w, UsageSummary{
		Mode:   mode,
		Limits: claude,
		Codex:  s.codexSummary(now),
		Spend: UsageSpend{
			Today:            sw.Today.Total,
			SinceSunday:      sw.Week.Total,
			SinceSundayShort: sw.Week.Short,
			SinceFirst:       sw.Month.Total,
			SinceFirstShort:  sw.Month.Short,
			FirstDay:         sw.First,
		},
	})
}
