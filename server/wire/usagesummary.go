package wire

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
