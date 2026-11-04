package wire

// LimitBurnDay is one local day of a limit's burn. NoData marks a day before the
// limit's first sample; ResetDay marks a day a new window started.
type LimitBurnDay struct {
	Date     string  `json:"date"`
	Burn     float64 `json:"burn"`
	NoData   bool    `json:"no_data"`
	ResetDay bool    `json:"reset_day"`
}

// LimitChipDirection is how today so far compares with yesterday.
type LimitChipDirection string

const (
	LimitChipUp    LimitChipDirection = "up"
	LimitChipDown  LimitChipDirection = "down"
	LimitChipEqual LimitChipDirection = "equal"
)

// LimitBurnChip compares today's burn so far with yesterday's full day, in whole
// points.
type LimitBurnChip struct {
	Today     int                `json:"today"`
	Yesterday int                `json:"yesterday"`
	Direction LimitChipDirection `json:"direction"`
}

// LimitForecastStatus says which forecast line a row shows.
type LimitForecastStatus string

const (
	// LimitForecastNone shows no forecast line.
	LimitForecastNone LimitForecastStatus = "none"
	// LimitForecastNeedsData shows "Forecast needs 3 days of data".
	LimitForecastNeedsData LimitForecastStatus = "needs_data"
	// LimitForecastProjected shows the percentage at reset, and when the limit
	// runs out if RunsOutAt is set.
	LimitForecastProjected LimitForecastStatus = "projected"
)

// LimitForecast is the forecast line of a weekly limit. Pace is points per day.
// AtReset is rounded to whole points; RunsOutAt is epoch seconds and set only
// when the forecast passes 100.
type LimitForecast struct {
	Status    LimitForecastStatus `json:"status"`
	Pace      float64             `json:"pace"`
	AtReset   int                 `json:"at_reset"`
	RunsOutAt int64               `json:"runs_out_at,omitempty"`
}
