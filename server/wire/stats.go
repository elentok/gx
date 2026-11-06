package wire

// The ranges the stats page offers, in days. The web UI reads them from the
// generated types. They stay separate declarations: tygo turns a const block
// into a union type named after the block.
const StatsRangeWeek = 7

const StatsRangeMonth = 30

const StatsRangeQuarter = 90

const StatsDefaultRange = StatsRangeWeek

// StatsAmount is one day of a token or spend series. NoData marks a day that
// had finished items and none carried a stamp; Value is then 0.
type StatsAmount struct {
	Value  float64 `json:"value"`
	NoData bool    `json:"no_data,omitempty"`
}

// StatsThroughput is the Throughput section. Every series has one point per
// entry of Stats.Days.
type StatsThroughput struct {
	TicketsMerged     []int `json:"tickets_merged"`
	EpicsMerged       []int `json:"epics_merged"`
	DesignsCompleted  []int `json:"designs_completed"`
	LinesAdded        []int `json:"lines_added"`
	LinesRemoved      []int `json:"lines_removed"`
	FollowUpsCreated  []int `json:"follow_ups_created"`
	FollowUpsFinished []int `json:"follow_ups_finished"`
	WaitingForDesign  []int `json:"waiting_for_design"`
}

// StatsHarnessAmount is one harness's share of a group's day, for tooltips.
type StatsHarnessAmount struct {
	Tokens  int64   `json:"tokens"`
	Dollars float64 `json:"dollars"`
}

// StatsCostGroup is one agent group's series, one point per entry of
// Stats.Days. Harness is empty on stamp days: stamps do not record it.
type StatsCostGroup struct {
	Group   string                          `json:"group"`
	Tokens  []StatsAmount                   `json:"tokens"`
	Spend   []StatsAmount                   `json:"spend"`
	Harness []map[string]StatsHarnessAmount `json:"harness"`
}

// StatsCost is the Tokens and spend section: Queue, Design, One-off and Other.
// Sources names each day's source (live, rebuilt or stamp). Partial is set when
// any day is not live.
type StatsCost struct {
	Groups  []StatsCostGroup `json:"groups"`
	Sources []string         `json:"sources"`
	Partial bool             `json:"partial"`
}

// StatsFriction is the Friction section. The per-day maps are keyed by
// reason, event type or ending source.
type StatsFriction struct {
	// PauseReasons lists the top reasons of the range, then "other" when more
	// exist. PausesByReason keeps every reason, so the tooltip can list them and
	// the chart folds the ones outside PauseReasons into "other".
	PauseReasons    []string         `json:"pause_reasons"`
	PausesByReason  []map[string]int `json:"pauses_by_reason"`
	RunEvents       []map[string]int `json:"run_events"`
	PauseEndings    []map[string]int `json:"pause_endings"`
	WebShare        *float64         `json:"web_share,omitempty"`
	MedianResumeSec *int             `json:"median_resume_secs,omitempty"`
}

// StatsLaunch is the Launch section: launches of finished tickets, each on the
// day of its own claim time. Medians are milliseconds and absent (nil, no_data)
// when the range holds no launch, never zero. Days has one point per entry of
// Stats.Days; StepMedianMs is keyed by launch step.
type StatsLaunch struct {
	Count         int              `json:"count"`
	MedianMs      *int64           `json:"median_ms,omitempty"`
	ShareUnderTgt *float64         `json:"share_under_target,omitempty"`
	TargetMs      int64            `json:"target_ms"`
	Days          []StatsAmount    `json:"days"`
	StepMedianMs  map[string]int64 `json:"step_median_ms"`
}

// Stats is the /v1/stats payload.
type Stats struct {
	Range      int             `json:"range"`
	Project    string          `json:"project,omitempty"`
	Days       []string        `json:"days"`
	Throughput StatsThroughput `json:"throughput"`
	Cost       StatsCost       `json:"cost"`
	Friction   StatsFriction   `json:"friction"`
	Launch     StatsLaunch     `json:"launch"`
	Grilling   StatsGrilling   `json:"grilling"`
	LimitBurn  StatsLimitBurns `json:"limit_burn"`
}

// StatsGrilling is the Grilling section: words and questions per day of the
// counted Design conversations. Days before the first counted item are
// no_data.
type StatsGrilling struct {
	Words     []StatsAmount `json:"words"`
	Questions []StatsAmount `json:"questions"`
}

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
