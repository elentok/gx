package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
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

// turnTokens returns one assistant turn's total tokens: input, output, cache
// writes and cache reads.
func turnTokens(u transcript.Usage) int {
	return u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// sessionStats are one Claude Code session's aggregate figures, computed
// from its own transcript: peak per-turn context occupancy (the same
// per-turn, never-summed calculation the smart-zone guardrail itself uses),
// its wall-clock span (first transcript line's timestamp to last), its
// total tokens and its total cost (both summed across every assistant turn,
// subagents included; each turn priced by its own model).
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

// SessionHomes are the directories session transcripts are read from. An
// empty one means "none": its sessions are reported as not found.
type SessionHomes struct {
	// Claude is the home whose .claude/projects holds Claude transcripts.
	Claude string
	// Codex is the Codex home (the resolved ~/.codex-equivalent directory).
	Codex string
}

// readSessionStats reads the Claude Code transcript under home for the session
// launched in cwd with the given id. ok is false (not an error) if the
// transcript can't be found yet — a mid-run report may ask about a session
// still being written, or one whose cwd/id weren't captured.
func readSessionStats(home, cwd, sessionID string) (sessionStats, error) {
	if home == "" || cwd == "" || sessionID == "" {
		return sessionStats{}, nil
	}
	return readTranscriptStats(transcript.PathIn(home, cwd, sessionID))
}

// readTranscriptStats is readSessionStats for the transcript at path.
func readTranscriptStats(path string) (sessionStats, error) {
	lines, ok, err := transcript.ReadAll(path)
	if err != nil || !ok || len(lines) == 0 {
		return sessionStats{}, err
	}

	stats := sessionStats{ok: true}
	for _, l := range lines {
		// ReadAll keeps undated compaction boundaries; they can't bound the session.
		if !l.Timestamp.IsZero() {
			if stats.start.IsZero() || l.Timestamp.Before(stats.start) {
				stats.start = l.Timestamp
			}
			if l.Timestamp.After(stats.end) {
				stats.end = l.Timestamp
			}
		}
		if l.Type != "assistant" {
			continue
		}
		if occ := l.Usage.Occupancy(); occ > stats.peakOccupancy {
			stats.peakOccupancy = occ
		}
		stats.cost += turnCost(l.Usage)
		stats.totalTokens += turnTokens(l.Usage)
	}
	subTokens, subCost, err := subagentUsage(path)
	if err != nil {
		return sessionStats{}, err
	}
	stats.totalTokens += subTokens
	stats.cost += subCost
	return stats, nil
}

// subagentUsage sums the tokens and cost of the subagent transcripts stored in
// the "<session>/subagents" directory next to the main transcript at path.
// They feed totals only: a subagent's context is its own, so it never counts
// towards the session's peak occupancy.
func subagentUsage(path string) (tokens int, cost float64, err error) {
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	if err != nil {
		return 0, 0, err
	}
	for _, f := range files {
		lines, ok, err := transcript.ReadAll(f)
		if err != nil {
			return 0, 0, err
		}
		if !ok {
			continue
		}
		for _, l := range lines {
			if l.Type == "assistant" {
				tokens += turnTokens(l.Usage)
				cost += turnCost(l.Usage)
			}
		}
	}
	return tokens, cost, nil
}

// SessionRef names one agent session: the agent that ran it, the directory it
// was launched in and its id.
type SessionRef struct {
	Agent     AgentKind
	Cwd       string
	SessionID string
}

// SessionsTotals sums the total tokens and cost of every session in refs, read
// under homes. All-or-nothing: if any session cannot be read or found, it
// returns an error and no partial sum. A Codex session adds its reported total
// and no cost.
func SessionsTotals(homes SessionHomes, refs []SessionRef) (tokens int64, cost float64, err error) {
	sum, err := sumSessions(refs, errTranscriptNotFound, func(ref SessionRef) (sessionStats, AgentKind, bool, error) {
		stats, err := readAgentSessionStats(homes, sessionKey{agent: ref.Agent, cwd: ref.Cwd, sessionID: ref.SessionID})
		return stats, ref.Agent, stats.ok, err
	})
	return sum.tokens, sum.cost, err
}

// sessionSum is the total over a set of sessions. hadCodex is true when any
// counted session was a Codex one.
type sessionSum struct {
	tokens   int64
	cost     float64
	hadCodex bool
}

var errTranscriptNotFound = errors.New("transcript not found")

// sumSessions adds up refs, all-or-nothing: resolve returns the stats of one
// ref and the agent that ran it, found=false when it has no transcript (the
// sum then fails with notFound), or an error when it cannot be read.
func sumSessions(refs []SessionRef, notFound error, resolve func(SessionRef) (stats sessionStats, agent AgentKind, found bool, err error)) (sessionSum, error) {
	var sum sessionSum
	for _, ref := range refs {
		stats, agent, found, err := resolve(ref)
		if err == nil && !found {
			err = notFound
		}
		if err != nil {
			return sessionSum{}, fmt.Errorf("session %s: %w", ref.SessionID, err)
		}
		sum.tokens += int64(stats.totalTokens)
		sum.cost += stats.cost
		sum.hadCodex = sum.hadCodex || agent == AgentCodex
	}
	return sum, nil
}

// SessionIDsTotals sums the total tokens and cost of the Claude sessions in
// ids, finding each transcript under home by id alone: the worktree a session ran in may
// be gone. All-or-nothing, like SessionsTotals; an id with no transcript, or
// with several, wraps tickets.ErrUnreadable; so does one that cannot be parsed.
// Any other error is a real I/O failure.
func SessionIDsTotals(home string, ids []string) (tokens int64, cost float64, err error) {
	refs := make([]SessionRef, len(ids))
	for i, id := range ids {
		refs[i] = SessionRef{Agent: AgentClaude, SessionID: id}
	}
	resolveByID := func(ref SessionRef) (sessionStats, AgentKind, bool, error) {
		matches, err := transcript.FindByID(home, ref.SessionID)
		if err != nil {
			return sessionStats{}, ref.Agent, false, err
		}
		if len(matches) != 1 {
			return sessionStats{}, ref.Agent, false, fmt.Errorf("%d transcripts found: %w", len(matches), tickets.ErrUnreadable)
		}
		stats, err := readTranscriptStats(matches[0])
		if err != nil {
			return sessionStats{}, ref.Agent, false, fmt.Errorf("%w: %w", tickets.ErrUnreadable, err)
		}
		return stats, ref.Agent, stats.ok, nil
	}
	sum, err := sumSessions(refs, tickets.ErrUnreadable, resolveByID)
	return sum.tokens, sum.cost, err
}

// SessionCost returns the estimated API-equivalent cost of the Claude Code
// session launched in cwd with the given id, wrapping readSessionStats for
// callers outside this package (ui/tickets' live cost aggregator). ok is
// false (not an error) if the transcript can't be found yet, matching
// readSessionStats.
func SessionCost(home, cwd, sessionID string) (cost float64, ok bool, err error) {
	stats, err := readSessionStats(home, cwd, sessionID)
	if err != nil {
		return 0, false, err
	}
	return stats.cost, stats.ok, nil
}

// readAgentSessionStats reads key's session under homes; an empty home reports
// it as not found.
func readAgentSessionStats(homes SessionHomes, key sessionKey) (sessionStats, error) {
	if key.agent != AgentCodex {
		return readSessionStats(homes.Claude, key.cwd, key.sessionID)
	}
	if homes.Codex == "" {
		return sessionStats{}, nil
	}
	stats, ok, err := codexsession.ReadStatsIn(homes.Codex, key.cwd, key.sessionID)
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

// writeMergedMetrics reads sessionID's own transcript — the same
// readAgentSessionStats call `gx ralph-loop report` uses, just invoked at
// merge-time instead of report-time — and, if its stats are available, writes
// its peak context occupancy and wall-clock duration into ticketPath's
// actual_context_window/elapsed_time frontmatter fields, also returning the
// same two values (ok=true) so mergeCherryPick can stamp them onto the merged
// commit's trailers too, matching what's in the frontmatter. A no-op
// (ok=false, not an error) when sessionID is empty or the transcript can't be
// read yet: a repair/reattached merging (see reconcile.go) has no fresh
// session of its own to read, and these metrics are a merging-time
// convenience, not a precondition for the cherry-pick itself to succeed.
//
// Tokens and cost are stamped over every session in the ticket's session_ids
// plus sessionID, independent of whether sessionID's own transcript was
// readable: all-or-nothing, so one unreadable session leaves total_tokens and
// actual_cost unstamped. A ticket with no session stamps zero.
func writeMergedMetrics(homes SessionHomes, agent AgentKind, cwd, sessionID, ticketPath string) (m mergedMetrics, ok bool, err error) {
	// A read error is treated like a transcript that is not there yet: the
	// no-op case above.
	stats, err := readAgentSessionStats(homes, sessionKey{agent: agent, cwd: cwd, sessionID: sessionID})
	if err != nil {
		stats = sessionStats{}
	}
	current, err := schema.ParseTicket(ticketPath)
	if err != nil {
		if !stats.ok {
			return mergedMetrics{}, false, nil
		}
		return mergedMetrics{}, false, err
	}
	sum, totalsErr := collectTicketTotals(homes, agent, cwd, sessionID, ticketPath, current)
	if !stats.ok && totalsErr != nil {
		return mergedMetrics{}, false, nil
	}
	m = mergedMetrics{cost: sum.cost, costStamped: totalsErr == nil}
	if stats.ok {
		m.contextWindow = stats.peakOccupancy
		m.elapsedSeconds = int(stats.end.Sub(stats.start).Seconds())
	}
	err = updateTicket(ticketPath, func(t *schema.Ticket) {
		if stats.ok {
			t.ActualContextWindow = m.contextWindow
			t.ElapsedTime = m.elapsedSeconds
		}
		if m.costStamped {
			total := sum.tokens
			t.TotalTokens = &total
			t.ActualCost = sum.cost
			t.HadCodex = sum.hadCodex
		}
	})
	if err != nil {
		return mergedMetrics{}, false, err
	}
	return m, stats.ok, nil
}

// collectTicketTotals sums every session of the ticket plus its recorded
// conversations. Any error means the sum would undercount.
func collectTicketTotals(homes SessionHomes, agent AgentKind, cwd, sessionID, ticketPath string, ticket schema.Ticket) (sessionSum, error) {
	sum, err := ticketSessionsTotals(homes, agent, cwd, sessionID, ticket.SessionIDs)
	if err != nil {
		return sessionSum{}, err
	}
	convo, err := recordedConversationTotals(ticketPath, string(ticket.ID))
	if err != nil {
		return sessionSum{}, err
	}
	sum.tokens += int64(convo.TotalTokens)
	sum.cost += convo.Cost
	return sum, nil
}

// mergedMetrics are the figures writeMergedMetrics stamped. cost is the sum over
// every session and is only meaningful when costStamped.
type mergedMetrics struct {
	contextWindow, elapsedSeconds int
	cost                          float64
	costStamped                   bool
}

// recordedConversationTotals sums the saved totals of the interactive
// conversations recorded against the ticket at path. A ticket outside the
// tracker layout has no address, so no conversations. An error means the sum
// would undercount.
func recordedConversationTotals(path, id string) (conversations.Totals, error) {
	addr, ok := tickets.AddressOfPath(path, id)
	if !ok {
		return conversations.Totals{}, nil
	}
	root, err := conversations.DefaultDir()
	if err != nil {
		return conversations.Totals{}, err
	}
	sum, complete, err := conversations.SumRecordedTotals(root, addr.String())
	if err != nil {
		return conversations.Totals{}, err
	}
	if !complete {
		return conversations.Totals{}, fmt.Errorf("conversation of %s has no saved totals", addr)
	}
	return sum, nil
}

// ticketSessionsTotals sums tokens and cost over ids plus current. A session's
// agent is only known for current; any other is tried as Claude, then Codex.
func ticketSessionsTotals(homes SessionHomes, agent AgentKind, cwd, current string, ids []string) (sessionSum, error) {
	if current != "" && !slices.Contains(ids, current) {
		ids = append(slices.Clone(ids), current)
	}
	refs := make([]SessionRef, len(ids))
	for i, id := range ids {
		refs[i] = SessionRef{Agent: AgentClaude, Cwd: cwd, SessionID: id}
	}
	resolveByKinds := func(ref SessionRef) (sessionStats, AgentKind, bool, error) {
		kinds := []AgentKind{AgentClaude, AgentCodex}
		if ref.SessionID == current {
			kinds = []AgentKind{agent}
		}
		for _, kind := range kinds {
			stats, err := readAgentSessionStats(homes, sessionKey{agent: kind, cwd: cwd, sessionID: ref.SessionID})
			if err != nil || stats.ok {
				return stats, kind, stats.ok, err
			}
		}
		return sessionStats{}, agent, false, nil
	}
	return sumSessions(refs, errTranscriptNotFound, resolveByKinds)
}
