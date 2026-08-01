package ralphloop

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/transcript"
)

// ReportOptions configures a single `gx ralph-loop report {epic-name}`
// invocation.
type ReportOptions struct {
	EpicName   string
	ScratchDir string // defaults to ".scratch"
}

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

// ticketSummary is one ticket's report figures, joined from its run-log
// events (order/window) and its session(s)' own transcripts
// (duration/peak-context/cost).
type ticketSummary struct {
	number          int
	title           string
	windowStart     time.Time
	windowEnd       time.Time
	duration        time.Duration
	peakOccupancy   int
	totalTokens     int
	cost            float64
	hasCodex        bool
	hasClaude       bool
	haveSessionData bool
	metricsMissing  bool
}

// sessionKey identifies one agent session an event referenced.
type sessionKey struct {
	agent     AgentKind
	cwd       string
	sessionID string
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

// Report reads opts.EpicName's run-log.jsonl under opts.ScratchDir and
// prints a summary to out: chronological task order, which tickets ran
// concurrently, per-ticket duration/peak-context/cost, and totals. It's
// safe to call at any time, including mid-run against a partial log — a
// ticket with no events yet, or a session whose transcript can't be read,
// is simply reported with zero/unknown figures rather than erroring.
func Report(opts ReportOptions, out io.Writer) error {
	scratchDir := opts.ScratchDir
	if scratchDir == "" {
		scratchDir = defaultScratchDir
	}

	events, ok, err := readEvents(scratchDir, opts.EpicName)
	if err != nil {
		return fmt.Errorf("reading run-log for epic %q: %w", opts.EpicName, err)
	}
	if !ok || len(events) == 0 {
		fmt.Fprintf(out, "no run-log events recorded yet for epic %q\n", opts.EpicName)
		return nil
	}

	titles := map[int]string{}
	if epic, epicErr := loadNamedEpic(scratchDir, opts.EpicName); epicErr == nil && epic != nil {
		for _, t := range epic.Tickets {
			titles[t.Number] = t.Title
		}
	}

	order, windows := ticketOrderAndWindows(events)
	sessionsByTicket := ticketSessions(events)

	summaries := make(map[int]*ticketSummary, len(order))
	for _, n := range order {
		w := windows[n]
		s := &ticketSummary{number: n, title: titles[n], windowStart: w.start, windowEnd: w.end}
		// Summed, not spanned: a ticket's sessions (its main iteration agent,
		// plus a conflict-resolution agent if one ran) never run concurrently
		// with each other by construction — exactly one is active for a given
		// ticket at a time — so summing each session's own span gives the same
		// answer as a true elapsed-time span would, without needing to assume
		// the sessions are contiguous.
		for _, key := range sessionsByTicket[n] {
			s.hasCodex = s.hasCodex || key.agent == AgentCodex
			s.hasClaude = s.hasClaude || key.agent == AgentClaude
			stats, statsErr := readAgentSessionStats(key)
			if statsErr != nil || !stats.ok {
				s.metricsMissing = true
				continue
			}
			s.haveSessionData = true
			s.duration += stats.end.Sub(stats.start)
			s.cost += stats.cost
			s.totalTokens += stats.totalTokens
			if stats.peakOccupancy > s.peakOccupancy {
				s.peakOccupancy = stats.peakOccupancy
			}
		}
		summaries[n] = s
	}

	printReport(out, opts.EpicName, order, summaries, mergeOverlappingWindows(order, windows), events)
	return nil
}

// ticketWindow is the [start, end] span of every event logged against one
// ticket, used to detect which tickets ran concurrently.
type ticketWindow struct {
	start, end time.Time
}

// ticketOrderAndWindows returns every ticket number that appears in events,
// in chronological order of its first event, plus each ticket's event
// window.
func ticketOrderAndWindows(events []Event) (order []int, windows map[int]ticketWindow) {
	windows = map[int]ticketWindow{}
	var firstSeen []int
	for _, ev := range events {
		w, exists := windows[ev.Ticket]
		if !exists {
			w = ticketWindow{start: ev.Time, end: ev.Time}
			firstSeen = append(firstSeen, ev.Ticket)
		}
		if ev.Time.Before(w.start) {
			w.start = ev.Time
		}
		if ev.Time.After(w.end) {
			w.end = ev.Time
		}
		windows[ev.Ticket] = w
	}
	sort.SliceStable(firstSeen, func(i, j int) bool {
		return windows[firstSeen[i]].start.Before(windows[firstSeen[j]].start)
	})
	return firstSeen, windows
}

// ticketSessions returns, per ticket, every distinct (cwd, agent_session)
// pair its events reference, in first-seen order.
func ticketSessions(events []Event) map[int][]sessionKey {
	out := map[int][]sessionKey{}
	seen := map[int]map[sessionKey]bool{}
	for _, ev := range events {
		if ev.AgentSession == "" {
			continue
		}
		agent := ev.Agent
		if agent == "" {
			agent = AgentClaude
		}
		key := sessionKey{agent: agent, cwd: ev.Cwd, sessionID: ev.AgentSession}
		if seen[ev.Ticket] == nil {
			seen[ev.Ticket] = map[sessionKey]bool{}
		}
		if seen[ev.Ticket][key] {
			continue
		}
		seen[ev.Ticket][key] = true
		out[ev.Ticket] = append(out[ev.Ticket], key)
	}
	return out
}

// mergeOverlappingWindows groups order's tickets into concurrency groups: a
// maximal run of tickets whose windows form a connected overlap chain once
// sorted by start time.
func mergeOverlappingWindows(order []int, windows map[int]ticketWindow) [][]int {
	sorted := make([]int, len(order))
	copy(sorted, order)
	sort.SliceStable(sorted, func(i, j int) bool {
		return windows[sorted[i]].start.Before(windows[sorted[j]].start)
	})

	var groups [][]int
	var current []int
	var currentEnd time.Time
	for _, n := range sorted {
		w := windows[n]
		if len(current) == 0 || w.start.Before(currentEnd) {
			current = append(current, n)
			if w.end.After(currentEnd) {
				currentEnd = w.end
			}
			continue
		}
		groups = append(groups, current)
		current = []int{n}
		currentEnd = w.end
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func ticketLabel(t *ticketSummary) string {
	if t.title == "" {
		return fmt.Sprintf("%02d", t.number)
	}
	return fmt.Sprintf("%02d %s", t.number, t.title)
}

func printReport(out io.Writer, epicName string, order []int, summaries map[int]*ticketSummary, groups [][]int, events []Event) {
	fmt.Fprintf(out, "gx ralph-loop report: %s\n\n", epicName)

	fmt.Fprintln(out, "Task order:")
	for _, n := range order {
		fmt.Fprintf(out, "  %s\n", ticketLabel(summaries[n]))
	}

	fmt.Fprintln(out, "\nConcurrency:")
	for _, group := range groups {
		if len(group) == 1 {
			fmt.Fprintf(out, "  %s (solo)\n", ticketLabel(summaries[group[0]]))
			continue
		}
		var labels []string
		for _, n := range group {
			labels = append(labels, fmt.Sprintf("%02d", n))
		}
		fmt.Fprintf(out, "  %s\n", strings.Join(labels, " + "))
	}

	fmt.Fprintln(out, "\nPer-ticket:")
	var totalCost float64
	var totalTokens int
	var hasCodex bool
	var hasClaude bool
	var totalPeak int
	totalMetricsKnown := true
	for _, n := range order {
		s := summaries[n]
		totalCost += s.cost
		totalTokens += s.totalTokens
		hasCodex = hasCodex || s.hasCodex
		hasClaude = hasClaude || s.hasClaude
		metricsKnown := s.haveSessionData && !s.metricsMissing
		totalMetricsKnown = totalMetricsKnown && metricsKnown
		if s.peakOccupancy > totalPeak {
			totalPeak = s.peakOccupancy
		}
		duration := "unknown"
		peakContext := "unknown"
		if metricsKnown {
			duration = s.duration.Round(time.Second).String()
			peakContext = fmt.Sprint(s.peakOccupancy)
		}
		tokens := "n/a"
		if s.hasCodex {
			tokens = "unknown"
			if metricsKnown {
				tokens = fmt.Sprint(s.totalTokens)
			}
		}
		fmt.Fprintf(out, "  %-40s duration=%-10s peak-context=%-8s tokens=%-8s cost=%s\n",
			ticketLabel(s), duration, peakContext, tokens, reportCost(s.cost, s.hasClaude, s.hasCodex))
	}

	epicStart, epicEnd := events[0].Time, events[0].Time
	for _, ev := range events {
		if ev.Time.Before(epicStart) {
			epicStart = ev.Time
		}
		if ev.Time.After(epicEnd) {
			epicEnd = ev.Time
		}
	}
	totalPeakText := "unknown"
	if totalMetricsKnown {
		totalPeakText = fmt.Sprint(totalPeak)
	}
	totalTokensText := "n/a"
	if hasCodex {
		totalTokensText = "unknown"
		if totalMetricsKnown {
			totalTokensText = fmt.Sprint(totalTokens)
		}
	}
	fmt.Fprintf(out, "\nTotal: duration=%s peak-context=%s tokens=%s cost=%s\n",
		epicEnd.Sub(epicStart).Round(time.Second), totalPeakText, totalTokensText, reportCost(totalCost, hasClaude, hasCodex))
}

func reportCost(cost float64, hasClaude, hasCodex bool) string {
	if !hasClaude && hasCodex {
		return "n/a"
	}
	value := fmt.Sprintf("$%.4f", cost)
	if hasCodex {
		return value + " + n/a"
	}
	return value
}
