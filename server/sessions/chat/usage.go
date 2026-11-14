package chat

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// ConversationUsage returns request input and cumulative usage. Each transcript,
// including each Claude subagent, is cached independently by size and mtime.
func (c *Chat) ConversationUsage(m Meta) *wire.SessionUsage {
	path, _, ok := c.Transcript(m.ID, m)
	if !ok {
		return nil
	}
	if m.Harness == string(harness.Codex) {
		usage, err := c.codexTotals.Read(path, codexsession.ReadContextUsage)
		if err != nil || usage.Total.Total == 0 {
			return nil
		}
		current := codexBreakdown(usage.Latest)
		result := &wire.SessionUsage{
			TotalTokens: usage.Total.Total, Totals: codexBreakdown(usage.Total),
			FirstContext: usage.First, PeakContext: usage.Peak,
			Compactions: usage.Compactions, ContextStale: usage.Stale,
			ModelContextWindow: usage.Capacity, Scope: "thread",
		}
		if current.InputTokens != nil {
			result.InputTokens = int(*current.InputTokens)
			if current.CacheReadTokens != nil {
				result.CacheReadTokens = int(*current.CacheReadTokens)
			}
			if current.CacheCreationTokens != nil {
				result.CacheCreationTokens = int(*current.CacheCreationTokens)
			}
		}
		return result
	}
	if m.Harness != string(harness.Claude) {
		return nil
	}
	current, err := c.reportedUsage.Read(path, func(p string) (transcript.Usage, error) {
		usage, _, err := transcript.LastAssistantReading(p)
		return usage, err
	})
	if err != nil {
		return nil
	}
	main, err := c.claudeTotals.Read(path, metrics.ReadTranscriptFileUsage)
	if err != nil {
		return nil
	}
	total := main
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	if err != nil {
		return nil
	}
	for _, file := range files {
		sub, err := c.claudeTotals.Read(file, metrics.ReadTranscriptFileUsage)
		if err != nil {
			return nil
		}
		total.Input += sub.Input
		total.Cached += sub.Cached
		total.Written += sub.Written
		total.Output += sub.Output
		total.Total += sub.Total
		total.Cost += sub.Cost
	}
	subTokens := total.Total - main.Total
	return &wire.SessionUsage{
		InputTokens: current.InputTokens, CacheReadTokens: current.CacheReadInputTokens,
		CacheCreationTokens: current.CacheCreationInputTokens,
		TotalTokens:         total.Total, EstimatedCost: &total.Cost,
		FirstContext: main.First, PeakContext: main.Peak,
		Compactions: main.Compactions, ContextStale: main.Stale, Scope: "conversation",
		MainTokens: &main.Total, SubagentTokens: &subTokens,
		Totals: &wire.SessionTokenTotals{
			InputTokens: &total.Input, CacheReadTokens: &total.Cached,
			CacheCreationTokens: &total.Written, OutputTokens: &total.Output,
		},
	}
}

func codexBreakdown(u codexsession.TokenBreakdown) *wire.SessionTokenTotals {
	result := &wire.SessionTokenTotals{
		CacheReadTokens: u.Cached, CacheCreationTokens: u.Written,
		OutputTokens: u.Output, ReasoningTokens: u.Reasoning,
	}
	// Preserve unknown categories in older rollouts instead of manufacturing zeros.
	if u.Input != nil && u.Cached != nil {
		input := *u.Input - *u.Cached
		if u.Written != nil {
			input -= *u.Written
		}
		input = max(0, input)
		result.InputTokens = &input
	}
	return result
}
