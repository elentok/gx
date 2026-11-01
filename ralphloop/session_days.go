package ralphloop

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

// SessionDay is the tokens and dollars one Claude session used on one local day.
type SessionDay struct {
	SessionID string
	// Day is the local calendar day, formatted "2006-01-02".
	Day    string
	Tokens int
	Cost   float64
}

// ClaudeSessionDays reads every transcript under home/.claude/projects
// (subagent transcripts included) and returns each session's per-day deltas,
// with day boundaries taken in loc. Each turn is priced by turnCost, so the
// deltas sum to the totals readTranscriptStats reports.
func ClaudeSessionDays(home string, loc *time.Location) ([]SessionDay, error) {
	paths, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []SessionDay
	for _, path := range paths {
		files := []string{path}
		subs, err := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
		if err != nil {
			return nil, err
		}
		files = append(files, subs...)

		byDay := map[string]*SessionDay{}
		for _, file := range files {
			lines, ok, err := transcript.ReadAll(file)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			for _, l := range lines {
				if l.Type != "assistant" || l.Timestamp.IsZero() {
					continue
				}
				day := l.Timestamp.In(loc).Format(time.DateOnly)
				d := byDay[day]
				if d == nil {
					d = &SessionDay{SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"), Day: day}
					byDay[day] = d
				}
				d.Tokens += turnTokens(l.Usage)
				d.Cost += turnCost(l.Usage)
			}
		}
		for _, d := range byDay {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}
