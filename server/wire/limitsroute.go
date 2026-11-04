package wire

// LimitsReportResult is the outcome of a limits report: the windows stored
// after the merge, or a refusal. A report with no windows changes nothing and
// reads the stored windows back.
type LimitsReportResult struct {
	FiveHour *LimitWindow `json:"five_hour,omitempty"`
	SevenDay *LimitWindow `json:"seven_day,omitempty"`
	Refused  bool         `json:"refused,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Message  string       `json:"message,omitempty"`
}
