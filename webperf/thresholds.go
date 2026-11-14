package webperf

const (
	// PoorINP and PoorLCP are the values above which a sample is poor and
	// keeps its attribution.
	PoorINP = 200.0
	PoorLCP = 2500.0

	// MinSamples is the count below which a value has no status and no change.
	MinSamples = 20
)
