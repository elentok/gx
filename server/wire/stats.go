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

// StatsCost is the Tokens and spend section: Queue, Design, Triage, One-off and Other.
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

	WebPerformance StatsWebPerformance `json:"web_performance"`
}

// NewStatsWebPerformance is the empty section: every list is empty, not nil,
// so it encodes as [].
func NewStatsWebPerformance() StatsWebPerformance {
	return StatsWebPerformance{
		HardLoads: []StatsWebPerfCard{}, RouteChanges: []StatsWebPerfCard{},
		Trend: []StatsWebPerfTrend{}, SlowestRoutes: []StatsWebPerfRoute{},
		WhySlow: []StatsWebPerfWhySlow{}, PoorINP: StatsWebPerfPoorINP{TopActions: []StatsWebPerfAction{}},
	}
}

// StatsWebPerformance is the Web performance section. It is account-wide: the
// project filter does not change it. Timings are milliseconds, CLS is unitless.
// Nil values are absent (no samples, or no change to show).
type StatsWebPerformance struct {
	HardLoads     []StatsWebPerfCard  `json:"hard_loads"`
	RouteChanges  []StatsWebPerfCard  `json:"route_changes"`
	Trend         []StatsWebPerfTrend `json:"trend"`
	SlowestRoutes []StatsWebPerfRoute `json:"slowest_routes"`
	// BuildStrip is nil when no build has been seen.
	BuildStrip *StatsWebPerfBuildStrip `json:"build_strip"`
	WhySlow    []StatsWebPerfWhySlow   `json:"why_slow"`
	PoorINP    StatsWebPerfPoorINP     `json:"poor_inp"`
	// DevHidden is true when include_dev is off and dev samples exist in the
	// range.
	DevHidden bool `json:"dev_hidden"`
}

// StatsWebPerfBuild is one build with its p75s over the selected range.
type StatsWebPerfBuild struct {
	BuildID    string   `json:"build_id"`
	WebVersion string   `json:"web_version"`
	LCPP75     *float64 `json:"lcp_p75"`
	INPP75     *float64 `json:"inp_p75"`
	JSP75      *float64 `json:"js_p75"`
}

// StatsWebPerfBuildStrip compares the current build (latest first seen in the
// stored data) with the one first seen just before it. Previous is nil with no
// earlier build. The changes are percents and nil when either build has fewer
// than 20 samples of the metric.
type StatsWebPerfBuildStrip struct {
	Current      StatsWebPerfBuild  `json:"current"`
	Previous     *StatsWebPerfBuild `json:"previous"`
	LCPChangePct *float64           `json:"lcp_change_pct"`
	INPChangePct *float64           `json:"inp_change_pct"`
	JSChangePct  *float64           `json:"js_change_pct"`
}

// StatsWebPerfWhySlow is one Why slow? row. Metric is ttfb, api_latency,
// js_bytes or css_bytes. Count is the current build's samples; the page dims
// the row below 20. Zero transfer sizes are left out.
type StatsWebPerfWhySlow struct {
	Metric      string   `json:"metric"`
	Count       int      `json:"count"`
	P75         *float64 `json:"p75"`
	PreviousP75 *float64 `json:"previous_p75"`
	ChangePct   *float64 `json:"change_pct"`
}

// StatsWebPerfAction is one INP action name with its poor-sample count.
type StatsWebPerfAction struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
}

// StatsWebPerfPoorINP covers the poor (above 200 ms) INP samples of both tabs
// over the range, all builds. The page dims everything but Count below 20.
type StatsWebPerfPoorINP struct {
	Count                int                  `json:"count"`
	InputDelayP75        *float64             `json:"input_delay_p75"`
	ProcessingP75        *float64             `json:"processing_p75"`
	PresentationDelayP75 *float64             `json:"presentation_delay_p75"`
	TopActions           []StatsWebPerfAction `json:"top_actions"`
	WorstChunk           string               `json:"worst_chunk,omitempty"`
}

// StatsWebPerfCard is one metric on one tab. Status is good,
// needs-improvement or poor, and empty with fewer than 20 samples or no
// standard cutoff. ChangePct is the percent change of p75 against the
// preceding equal window.
type StatsWebPerfCard struct {
	Metric    string   `json:"metric"`
	Count     int      `json:"count"`
	P75       *float64 `json:"p75"`
	Status    string   `json:"status,omitempty"`
	ChangePct *float64 `json:"change_pct"`
}

// StatsWebPerfPoint is one day of a trend: one point per entry of Stats.Days.
type StatsWebPerfPoint struct {
	P75   *float64 `json:"p75"`
	Count int      `json:"count"`
}

// StatsWebPerfTrend is the daily p75 series of one metric, over both tabs.
type StatsWebPerfTrend struct {
	Metric string              `json:"metric"`
	Days   []StatsWebPerfPoint `json:"days"`
}

// StatsWebPerfRoute is one route's p75 of route_ready.
type StatsWebPerfRoute struct {
	Route string  `json:"route"`
	P75   float64 `json:"p75"`
	Count int     `json:"count"`
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
