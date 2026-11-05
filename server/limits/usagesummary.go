package limits

import (
	"net/http"
	"time"

	"github.com/elentok/gx/server/wire"
)

// usageLimitWindow builds a row from a window and, for a weekly limit, the
// history of limit id.
func (l *Limits) usageLimitWindow(w wire.LimitWindow, id string, weekly bool, now time.Time) *wire.UsageLimitWindow {
	row := &wire.UsageLimitWindow{LimitWindow: w}
	if !weekly {
		return row
	}
	samples := l.history.Samples(id)
	row.Chip = limitBurnChip(samples, now)
	f := limitForecast(samples, l.latestOr(id, w), now)
	row.Forecast = &f
	return row
}

// latestOr is the saved latest sample of limit id, or the live window w when
// none is saved.
func (l *Limits) latestOr(id string, w wire.LimitWindow) wire.LimitSample {
	if latest, ok := l.history.Latest(id); ok {
		return latest
	}
	return wire.LimitSample{UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt, ReceivedAt: w.ReceivedAt}
}

func (l *Limits) codexSummary(now time.Time) *wire.UsageLimits {
	var out wire.UsageLimits
	for _, c := range []struct {
		id     string
		weekly bool
		dst    **wire.UsageLimitWindow
	}{
		{wire.LimitIDCodex5h, false, &out.FiveHour},
		{wire.LimitIDCodexWeekly, true, &out.SevenDay},
	} {
		latest, ok := l.history.Latest(c.id)
		if !ok {
			continue
		}
		w := wire.LimitWindow{UsedPercentage: latest.UsedPercentage, ResetsAt: latest.ResetsAt, ReceivedAt: latest.ReceivedAt}
		*c.dst = l.usageLimitWindow(w, c.id, c.weekly, now)
	}
	if out.FiveHour == nil && out.SevenDay == nil {
		return nil
	}
	return &out
}

func (l *Limits) summaryRead(w http.ResponseWriter, _ *http.Request) {
	stored := l.store.read()
	mode := wire.UsageModeSpending
	if stored.FiveHour != nil || stored.SevenDay != nil {
		mode = wire.UsageModeLimits
	}
	now := time.Now()
	var claude wire.UsageLimits
	if stored.FiveHour != nil {
		claude.FiveHour = l.usageLimitWindow(*stored.FiveHour, wire.LimitIDClaude5h, false, now)
	}
	if stored.SevenDay != nil {
		claude.SevenDay = l.usageLimitWindow(*stored.SevenDay, wire.LimitIDClaudeWeekly, true, now)
	}
	sw := computeSpendWindows(now, l.spendDays())
	wire.WriteJSON(w, wire.UsageSummary{
		Mode:   mode,
		Limits: claude,
		Codex:  l.codexSummary(now),
		Spend: wire.UsageSpend{
			Today:            sw.Today.Total,
			SinceSunday:      sw.Week.Total,
			SinceSundayShort: sw.Week.Short,
			SinceFirst:       sw.Month.Total,
			SinceFirstShort:  sw.Month.Short,
			FirstDay:         sw.First,
		},
	})
}
