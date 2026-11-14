package metrics

import "github.com/elentok/gx/transcript"

// TranscriptFileUsage accounts for one file, excluding adjacent subagent files.
// Callers can cache each file independently, including while the parent is idle.
type TranscriptFileUsage struct {
	First, Peak, Compactions              int
	Stale                                 bool
	Input, Cached, Written, Output, Total int64
	Cost                                  float64
}

// ReadTranscriptFileUsage reduces reported request usage and compaction markers.
func ReadTranscriptFileUsage(path string) (TranscriptFileUsage, error) {
	lines, _, err := transcript.ReadAll(path)
	if err != nil {
		return TranscriptFileUsage{}, err
	}
	var result TranscriptFileUsage
	for _, line := range lines {
		if line.IsCompactBoundary() {
			result.Compactions++
			result.Stale = true
		}
		if line.Type != "assistant" {
			continue
		}
		u := line.Usage
		if input := u.Occupancy(); input > 0 {
			if result.First == 0 {
				result.First = input
			}
			result.Peak = max(result.Peak, input)
			result.Stale = false
		}
		result.Input += int64(u.InputTokens)
		result.Cached += int64(u.CacheReadInputTokens)
		result.Written += int64(u.CacheCreationInputTokens)
		result.Output += int64(u.OutputTokens)
		result.Total += int64(turnTokens(u))
		result.Cost += turnCost(u)
	}
	return result, nil
}
