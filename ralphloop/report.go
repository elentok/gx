package ralphloop

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ReportOptions configures a single `gx ralph-loop report {epic-name}`
// invocation.
type ReportOptions struct {
	EpicName   string
	ScratchDir string // defaults to ".scratch"
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

	titles := map[string]string{}
	if epic, epicErr := loadNamedEpic(scratchDir, opts.EpicName); epicErr == nil && epic != nil {
		for _, t := range epic.Tickets {
			titles[t.Identifier] = t.Title
		}
	}

	order, windows := ticketOrderAndWindows(events)
	sessionsByTicket := ticketSessions(events)
	depsByTicket := ticketDepsCommands(events)

	summaries := make(map[string]*ticketSummary, len(order))
	for _, id := range order {
		w := windows[id]
		s := &ticketSummary{identifier: id, title: titles[id], windowStart: w.start, windowEnd: w.end, depsCommand: depsByTicket[id]}
		// Summed, not spanned: a ticket's sessions (its main iteration agent,
		// plus a conflict-resolution agent if one ran) never run concurrently
		// with each other by construction — exactly one is active for a given
		// ticket at a time — so summing each session's own span gives the same
		// answer as a true elapsed-time span would, without needing to assume
		// the sessions are contiguous.
		for _, key := range sessionsByTicket[id] {
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
		summaries[id] = s
	}

	printReport(out, opts.EpicName, order, summaries, mergeOverlappingWindows(order, windows), events)
	return nil
}

func printReport(out io.Writer, epicName string, order []string, summaries map[string]*ticketSummary, groups [][]string, events []Event) {
	fmt.Fprintf(out, "gx ralph-loop report: %s\n\n", epicName)

	fmt.Fprintln(out, "Task order:")
	for _, id := range order {
		fmt.Fprintf(out, "  %s\n", ticketLabel(summaries[id]))
	}

	fmt.Fprintln(out, "\nConcurrency:")
	for _, group := range groups {
		if len(group) == 1 {
			fmt.Fprintf(out, "  %s (solo)\n", ticketLabel(summaries[group[0]]))
			continue
		}
		var labels []string
		for _, id := range group {
			labels = append(labels, id)
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
	for _, id := range order {
		s := summaries[id]
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
		if s.depsCommand != "" {
			fmt.Fprintf(out, "      deps: %s\n", s.depsCommand)
		}
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
