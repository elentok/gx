package ralphloop

import (
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// pricing is per-million-token USD pricing for one Claude model tier.
type pricing struct {
	input, output, cacheRead, cacheWrite float64
}

// modelPricingTable maps a model-name substring (matched case-insensitively
// against transcript.Usage.Model) to its per-million-token pricing tier.
// Anthropic prices by tier (Opus/Sonnet/Haiku) rather than by exact dated
// model string, so matching on the tier name resolves any dated variant
// (e.g. "claude-haiku-4-5-20251001") without needing to enumerate every
// model release.
var modelPricingTable = []struct {
	match   string
	pricing pricing
}{
	{"opus", pricing{input: 15, output: 75, cacheRead: 1.5, cacheWrite: 18.75}},
	{"sonnet", pricing{input: 3, output: 15, cacheRead: 0.3, cacheWrite: 3.75}},
	{"haiku", pricing{input: 0.8, output: 4, cacheRead: 0.08, cacheWrite: 1}},
}

// pricingForModel returns model's pricing tier, falling back to Sonnet-tier
// pricing for an unrecognized model name rather than silently reporting
// zero cost.
func pricingForModel(model string) pricing {
	lower := strings.ToLower(model)
	for _, entry := range modelPricingTable {
		if strings.Contains(lower, entry.match) {
			return entry.pricing
		}
	}
	return modelPricingTable[1].pricing
}

// turnCost returns one assistant turn's USD cost, priced by its own model.
func turnCost(u transcript.Usage) float64 {
	const perMTok = 1_000_000.0
	p := pricingForModel(u.Model)
	return float64(u.InputTokens)/perMTok*p.input +
		float64(u.OutputTokens)/perMTok*p.output +
		float64(u.CacheReadInputTokens)/perMTok*p.cacheRead +
		float64(u.CacheCreationInputTokens)/perMTok*p.cacheWrite
}

// sessionStats are one Claude Code session's aggregate figures, computed
// from its own transcript: peak per-turn context occupancy (the same
// per-turn, never-summed calculation the smart-zone guardrail itself uses),
// its wall-clock span (first transcript line's timestamp to last), and its
// total cost (summed across every assistant turn, each turn priced by its
// own model).
type sessionStats struct {
	peakOccupancy int
	totalTokens   int
	cost          float64
	start, end    time.Time
	ok            bool // false if the transcript couldn't be read/found
}

// sessionKey identifies one agent session an event referenced.
type sessionKey struct {
	agent     AgentKind
	cwd       string
	sessionID string
}

// readSessionStats reads the Claude Code transcript for the session
// launched in cwd with the given id. ok is false (not an error) if the
// transcript can't be found yet — a mid-run report may ask about a session
// still being written, or one whose cwd/id weren't captured.
func readSessionStats(cwd, sessionID string) (sessionStats, error) {
	if cwd == "" || sessionID == "" {
		return sessionStats{}, nil
	}
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		return sessionStats{}, err
	}
	lines, ok, err := transcript.ReadAll(path)
	if err != nil || !ok || len(lines) == 0 {
		return sessionStats{}, err
	}

	stats := sessionStats{start: lines[0].Timestamp, end: lines[0].Timestamp, ok: true}
	for _, l := range lines {
		if l.Timestamp.Before(stats.start) {
			stats.start = l.Timestamp
		}
		if l.Timestamp.After(stats.end) {
			stats.end = l.Timestamp
		}
		if l.Type != "assistant" {
			continue
		}
		if occ := l.Usage.Occupancy(); occ > stats.peakOccupancy {
			stats.peakOccupancy = occ
		}
		stats.cost += turnCost(l.Usage)
	}
	return stats, nil
}

// SessionCost returns the estimated API-equivalent cost of the Claude Code
// session launched in cwd with the given id, wrapping readSessionStats for
// callers outside this package (ui/tickets' live cost aggregator). ok is
// false (not an error) if the transcript can't be found yet, matching
// readSessionStats.
func SessionCost(cwd, sessionID string) (cost float64, ok bool, err error) {
	stats, err := readSessionStats(cwd, sessionID)
	if err != nil {
		return 0, false, err
	}
	return stats.cost, stats.ok, nil
}

func readAgentSessionStats(key sessionKey) (sessionStats, error) {
	if key.agent != AgentCodex {
		return readSessionStats(key.cwd, key.sessionID)
	}
	stats, ok, err := codexsession.ReadStats(key.cwd, key.sessionID)
	if err != nil || !ok {
		return sessionStats{}, err
	}
	return sessionStats{
		start:         stats.Start,
		end:           stats.End,
		peakOccupancy: stats.PeakContext,
		totalTokens:   stats.TotalTokens,
		ok:            true,
	}, nil
}

// writeLandedMetrics reads sessionID's own transcript — the same
// readAgentSessionStats call `gx ralph-loop report` uses, just invoked at
// land-time instead of report-time — and, if its stats are available, writes
// its peak context occupancy and wall-clock duration into ticketPath's
// actual_context_window/elapsed_time frontmatter fields, also returning the
// same two values (ok=true) so landCherryPick can stamp them onto the landed
// commit's trailers too, matching what's in the frontmatter. A no-op
// (ok=false, not an error) when sessionID is empty or the transcript can't be
// read yet: a repair/reattached landing (see reconcile.go) has no fresh
// session of its own to read, and these metrics are a landing-time
// convenience, not a precondition for the cherry-pick itself to succeed.
func writeLandedMetrics(agent AgentKind, cwd, sessionID, ticketPath string) (contextWindow, elapsedSeconds int, cost float64, ok bool, err error) {
	stats, err := readAgentSessionStats(sessionKey{agent: agent, cwd: cwd, sessionID: sessionID})
	if err != nil || !stats.ok {
		return 0, 0, 0, false, nil
	}
	elapsedSeconds = int(stats.end.Sub(stats.start).Seconds())
	if err := writeTicketMetrics(ticketPath, stats.peakOccupancy, elapsedSeconds, stats.cost); err != nil {
		return 0, 0, 0, false, err
	}
	return stats.peakOccupancy, elapsedSeconds, stats.cost, true, nil
}

// writeTicketMetrics rewrites ticketPath's frontmatter with contextWindow/
// elapsedSeconds/cost in actual_context_window/elapsed_time/actual_cost,
// round-tripping through schema's typed parse/marshal so every other field
// and the markdown body are carried through unchanged.
func writeTicketMetrics(ticketPath string, contextWindow, elapsedSeconds int, cost float64) error {
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		return err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		return err
	}
	t.ActualContextWindow = contextWindow
	t.ElapsedTime = elapsedSeconds
	t.ActualCost = cost

	out, err := schema.MarshalTicket(t, schema.ParseBody(string(raw)))
	if err != nil {
		return err
	}
	return writeFileAtomic(ticketPath, out)
}
