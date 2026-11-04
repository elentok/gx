package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
)

// historyRig runs a report through the real merge, then the history.
type historyRig struct {
	t *testing.T
	l *limitsStore
	h *limitHistory
}

func newHistoryRig(t *testing.T, dir string) *historyRig {
	return &historyRig{t: t, l: openLimitsStore(dir), h: openLimitHistory(dir)}
}

func (r *historyRig) report(rep wire.LimitsReport, now int64) {
	r.t.Helper()
	res, err := r.l.apply(rep, now)
	if err != nil {
		r.t.Fatal(err)
	}
	if err := r.h.recordClaude(rep, res, now); err != nil {
		r.t.Fatal(err)
	}
}

func five(pct float64, reset int64) wire.LimitsReport {
	return wire.LimitsReport{FiveHour: &wire.LimitWindow{UsedPercentage: pct, ResetsAt: reset}}
}

func TestLimitHistory_FileSitsBesideLimitsAndStaleCheckReadsLimitsOnly(t *testing.T) {
	dir := t.TempDir()
	r := newHistoryRig(t, dir)
	if filepath.Dir(r.h.path) != filepath.Dir(r.l.path) {
		t.Errorf("history %s and limits %s are in different directories", r.h.path, r.l.path)
	}

	// The limits file holds an old reading; the history holds a fresh one.
	old := int64(1000)
	if _, err := r.l.apply(five(10, 5000), old); err != nil {
		t.Fatal(err)
	}
	fresh := old + 10*wire.LimitsStaleAfterSeconds
	if err := r.h.recordClaude(five(10, 5000), wire.LimitsApplyResult{FiveHour: &wire.LimitResult{Outcome: wire.LimitChanged}}, fresh); err != nil {
		t.Fatal(err)
	}
	res, err := r.l.apply(five(10, 5000), fresh+1)
	if err != nil {
		t.Fatal(err)
	}
	if res.FiveHour.PrevReceivedAt != old || !limitsVisiblyChanged(res.FiveHour, fresh+1) {
		t.Errorf("prev received = %d, want %d from the limits file, and a stale confirm", res.FiveHour.PrevReceivedAt, old)
	}
}

func TestLimitHistory_ChangeSavesSampleWithAllFields(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(five(12.5, 5000), 1000)
	got := r.h.Samples(wire.LimitIDClaude5h)
	want := wire.LimitSample{LimitID: wire.LimitIDClaude5h, UsedPercentage: 12.5, ResetsAt: 5000, ReceivedAt: 1000}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("samples = %+v, want [%+v]", got, want)
	}
	r.report(five(30, 5000), 1100)
	r.report(five(30, 9000), 1200)
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 3 {
		t.Errorf("samples = %d, want 3 (percentage change, then reset change)", n)
	}
}

func TestLimitHistory_SameValueWithinHourSavesNothing(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(five(10, 5000), 1000)
	r.report(five(10, 5000), 1000+limitConfirmEverySeconds-1)
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 1 {
		t.Errorf("samples = %d, want 1", n)
	}
	l, _ := r.h.Latest(wire.LimitIDClaude5h)
	if l.ReceivedAt != 1000+limitConfirmEverySeconds-1 {
		t.Errorf("latest received = %d, want the unchanged report's time", l.ReceivedAt)
	}
}

func TestLimitHistory_SameValueAfterHourSavesOneConfirmingSample(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(five(10, 5000), 1000)
	r.report(five(10, 5000), 1000+limitConfirmEverySeconds)
	r.report(five(10, 5000), 1000+limitConfirmEverySeconds+5)
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 2 {
		t.Errorf("samples = %d, want 2", n)
	}
}

func TestLimitHistory_FractionalRiseSavesSample(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(five(10.2, 5000), 1000)
	r.report(five(10.4, 5000), 1010) // merge says confirmed: same rounded value
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 2 {
		t.Errorf("samples = %d, want 2", n)
	}
}

func TestLimitHistory_DroppedWindowSavesNothing(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(five(30, 5000), 1000)
	r.report(five(10, 5000), 1010) // lower, same reset: dropped
	r.report(five(90, 4000), 1020) // earlier reset, stored one in the future: dropped
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 1 {
		t.Errorf("samples = %d, want 1", n)
	}
}

func TestLimitHistory_WeeklyHasItsOwnID(t *testing.T) {
	r := newHistoryRig(t, t.TempDir())
	r.report(wire.LimitsReport{SevenDay: &wire.LimitWindow{UsedPercentage: 5, ResetsAt: 7000}}, 1000)
	if n := len(r.h.Samples(wire.LimitIDClaudeWeekly)); n != 1 {
		t.Errorf("weekly samples = %d, want 1", n)
	}
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 0 {
		t.Errorf("5h samples = %d, want 0", n)
	}
}

func TestLimitHistory_ReopenKeepsEverySample(t *testing.T) {
	dir := t.TempDir()
	r := newHistoryRig(t, dir)
	r.report(five(10, 5000), 1000)
	r.report(five(20, 5000), 1100)
	h := openLimitHistory(dir)
	if n := len(h.Samples(wire.LimitIDClaude5h)); n != 2 {
		t.Errorf("samples after reopen = %d, want 2", n)
	}
	if first, ok := h.FirstSampleAt(wire.LimitIDClaude5h); !ok || first != 1000 {
		t.Errorf("first = %d, %v, want 1000", first, ok)
	}
	if l, ok := h.Latest(wire.LimitIDClaude5h); !ok || l.UsedPercentage != 20 {
		t.Errorf("latest = %+v, %v, want 20%%", l, ok)
	}
}

func TestLimitHistory_CorruptFileLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, limitHistoryFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newHistoryRig(t, dir)
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 0 {
		t.Fatalf("samples = %d, want 0", n)
	}
	r.report(five(10, 5000), 1000)
	if n := len(r.h.Samples(wire.LimitIDClaude5h)); n != 1 {
		t.Errorf("samples after save = %d, want 1", n)
	}
}
