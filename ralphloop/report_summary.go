package ralphloop

import (
	"fmt"
	"sort"
	"time"
)

// ticketSummary is one ticket's report figures, joined from its run-log
// events (order/window) and its session(s)' own transcripts
// (duration/peak-context/cost).
type ticketSummary struct {
	identifier      string
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
	// depsCommand is the install command run in this ticket's iteration
	// worktree, from its deps-installed event's Reason ("" if no command
	// matched or no such event was logged, e.g. logs predating this field).
	depsCommand string
}

// ticketWindow is the [start, end] span of every event logged against one
// ticket, used to detect which tickets ran concurrently.
type ticketWindow struct {
	start, end time.Time
}

// ticketOrderAndWindows returns every ticket identifier that appears in
// events, in chronological order of its first event, plus each ticket's
// event window.
func ticketOrderAndWindows(events []Event) (order []string, windows map[string]ticketWindow) {
	windows = map[string]ticketWindow{}
	var firstSeen []string
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
func ticketSessions(events []Event) map[string][]sessionKey {
	out := map[string][]sessionKey{}
	seen := map[string]map[sessionKey]bool{}
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

// ticketDepsCommands returns, per ticket, its deps-installed event's Reason
// (the install command run, "" if none matched).
func ticketDepsCommands(events []Event) map[string]string {
	out := map[string]string{}
	for _, ev := range events {
		if ev.Type == eventDepsInstalled {
			out[ev.Ticket] = ev.Reason
		}
	}
	return out
}

// mergeOverlappingWindows groups order's tickets into concurrency groups: a
// maximal run of tickets whose windows form a connected overlap chain once
// sorted by start time.
func mergeOverlappingWindows(order []string, windows map[string]ticketWindow) [][]string {
	sorted := make([]string, len(order))
	copy(sorted, order)
	sort.SliceStable(sorted, func(i, j int) bool {
		return windows[sorted[i]].start.Before(windows[sorted[j]].start)
	})

	var groups [][]string
	var current []string
	var currentEnd time.Time
	for _, id := range sorted {
		w := windows[id]
		if len(current) == 0 || w.start.Before(currentEnd) {
			current = append(current, id)
			if w.end.After(currentEnd) {
				currentEnd = w.end
			}
			continue
		}
		groups = append(groups, current)
		current = []string{id}
		currentEnd = w.end
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func ticketLabel(t *ticketSummary) string {
	if t.title == "" {
		return t.identifier
	}
	return fmt.Sprintf("%s %s", t.identifier, t.title)
}
