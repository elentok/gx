package stats

import (
	"time"

	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/rest"
	"github.com/elentok/gx/server/wire"
)

// RestRecords is the Rest records store as the Mental health section reads it.
type RestRecords interface {
	Range(from, to time.Time) ([]rest.Record, error)
}

// buildMentalHealth counts the records per local day of the window. A dropped
// Break cancels its start record, so it is subtracted on the day of that
// start, not the day of the drop.
func buildMentalHealth(src RestRecords, win limits.StatsWindow) (wire.StatsMentalHealth, error) {
	out := wire.NewStatsMentalHealth(len(win.Days))
	if src == nil || len(win.Days) == 0 {
		return out, nil
	}
	// Day files are named by each record's own zone, so read a day more on
	// each side and let Bucket drop what is outside the window.
	recs, err := src.Range(win.Days[0].AddDate(0, 0, -1), win.Days[len(win.Days)-1].AddDate(0, 0, 1))
	if err != nil {
		return out, err
	}
	for _, r := range recs {
		switch r.Kind {
		case rest.RecordBreakStart:
			if i := win.Bucket(r.Time); i >= 0 {
				out.BreakStarts[i]++
			}
		case rest.RecordBreakDropped:
			if i := win.Bucket(r.Started); i >= 0 {
				out.BreakStarts[i]--
			}
		case rest.RecordUnlock:
			i := win.Bucket(r.Time)
			if i < 0 {
				continue
			}
			switch r.Rest {
			case rest.KindBreak:
				out.BreakUnlocks[i]++
			case rest.KindBedtime:
				out.BedtimeUnlocks[i]++
			}
		}
	}
	return out, nil
}
