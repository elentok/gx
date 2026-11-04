package wire

// StatsLimitBurn is one limit's burn over the range: one Days point per entry
// of Stats.Days. Sum and Average cover the days with data; StartDay is the first
// day with data and is set only when the range reaches before it. Forecast is
// set on weekly limits that have a reading.
type StatsLimitBurn struct {
	Days     []LimitBurnDay `json:"days"`
	Sum      float64        `json:"sum"`
	Average  float64        `json:"average"`
	StartDay string         `json:"start_day,omitempty"`
	Forecast *LimitForecast `json:"forecast,omitempty"`
}

// StatsLimitBurns is the Limit burn section. It is account-wide: the project
// filter does not change it.
type StatsLimitBurns struct {
	Claude5h     StatsLimitBurn `json:"claude_5h"`
	ClaudeWeekly StatsLimitBurn `json:"claude_weekly"`
	Codex5h      StatsLimitBurn `json:"codex_5h"`
	CodexWeekly  StatsLimitBurn `json:"codex_weekly"`
}
