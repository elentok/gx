package server

import (
	"strings"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
)

func newStatsGrilling(n int) wire.StatsGrilling {
	return wire.StatsGrilling{Words: make([]wire.StatsAmount, n), Questions: make([]wire.StatsAmount, n)}
}

// buildGrilling sums the counted items of the Design records. isDesign is asked
// for records that lack the Design flag. An item's message ID counts once
// across all records. The hatching boundary is the earliest counted item of
// any project, so it does not move with the project filter.
func buildGrilling(recs []conversations.Record, isDesign func(ticket string) bool, win limits.StatsWindow, project string) wire.StatsGrilling {
	out := newStatsGrilling(len(win.Days))
	seen := map[string]bool{}
	var earliest time.Time
	for _, r := range recs {
		if r.Grilling == nil || !(r.Design || (r.Ticket != "" && isDesign(r.Ticket))) {
			continue
		}
		inProject := project == "" || ticketProject(r.Ticket) == project
		for _, it := range r.Grilling {
			if seen[it.MessageID] {
				continue
			}
			seen[it.MessageID] = true
			if earliest.IsZero() || it.At.Before(earliest) {
				earliest = it.At
			}
			if i := win.Bucket(it.At); inProject && i >= 0 {
				out.Words[i].Value += float64(it.Words)
				out.Questions[i].Value += float64(it.Questions)
			}
		}
	}
	for i, day := range win.Days {
		if earliest.IsZero() || day.Before(localMidnight(earliest)) {
			out.Words[i].NoData, out.Questions[i].NoData = true, true
		}
	}
	return out
}

func localMidnight(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func ticketProject(address string) string {
	project, _, _ := strings.Cut(address, ":")
	return project
}
