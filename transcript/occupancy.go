package transcript

import "slices"

// OccupancyReading is what the transcript's tail says about a session's
// current context occupancy: the newest assistant turn's usage, whether one
// was found at all, and whether a compaction boundary has landed since.
type OccupancyReading struct {
	Usage Usage
	Found bool

	// Stale reports that a compaction boundary appears after the assistant
	// turn Usage came from — the context has already shrunk, but no turn has
	// been written yet to say by how much, so Usage still describes the
	// pre-compaction session. The number is reported anyway: a caller that
	// displays occupancy keeps something to show, while a caller deciding
	// whether the session has breached a ceiling can skip the decision until
	// the next turn lands.
	Stale bool
}

// ReadOccupancy reports the newest assistant turn's usage in the transcript
// at path together with whether a compaction boundary is newer than it, in a
// single tail scan.
//
// Recency is decided by position in the file, never by timestamps: the
// transcript is append-only, and a boundary and the turn around it can carry
// the same timestamp down to the nanosecond, which no timestamp comparison
// can order. Scanning backwards, a boundary met before any assistant turn
// therefore means the occupancy is stale — including when there is no
// assistant turn in the file at all, which is the same "compaction has
// happened, nothing has reported since" state.
//
// A missing or empty file reads as the zero OccupancyReading.
func ReadOccupancy(path string) (OccupancyReading, error) {
	var reading OccupancyReading
	err := tailScan(path, func(lines []string) bool {
		reading = OccupancyReading{}
		for _, raw := range slices.Backward(lines) {
			entry, ok := parseLine(raw)
			if !ok {
				continue
			}
			if entry.Type == "system" && entry.Subtype == compactBoundarySubtype {
				reading.Stale = true
				continue
			}
			if entry.Type != "assistant" {
				continue
			}
			reading.Usage, reading.Found = usageFromLine(entry), true
			return true
		}
		return false
	})
	if err != nil {
		return OccupancyReading{}, err
	}
	return reading, nil
}

// ReadSessionOccupancy is a convenience wrapper combining Path and
// ReadOccupancy: it reads the transcript of the session launched in cwd.
func ReadSessionOccupancy(cwd, sessionID string) (OccupancyReading, error) {
	path, err := Path(cwd, sessionID)
	if err != nil {
		return OccupancyReading{}, err
	}
	return ReadOccupancy(path)
}
