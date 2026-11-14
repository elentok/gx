package webperf

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var zone = time.FixedZone("test", 2*3600)

func at(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, zone) }

func fnow(t time.Time) func() time.Time { return func() time.Time { return t } }

func open(t *testing.T, dir string, now time.Time) *Store {
	t.Helper()
	s, err := Open(dir, fnow(now))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func smp(session, id string, v float64) Sample {
	return Sample{ID: id, Session: session, Metric: "lcp", Value: v, Route: "stats"}
}

func dayFile(dir, day string) string { return filepath.Join(dir, DirName, day+fileExt) }

func TestAppend_WritesOneFilePerReceiveDayInPrivateMode(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 23)
	s := open(t, dir, now)
	// Sample timestamps are ignored; the receive time picks the file.
	sm := smp("s", "1", 1)
	sm.Timestamp = at(2026, 1, 1, 0).UnixMilli()
	stored, dropped, err := s.Append(now, []Sample{sm, smp("s", "2", 2)})
	if err != nil || stored != 2 || dropped != 0 {
		t.Fatalf("got %d %d %v", stored, dropped, err)
	}
	if _, _, err := s.Append(at(2026, 11, 15, 1), []Sample{smp("s", "3", 3)}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dayFile(dir, "2026-11-14"))
	if err != nil {
		t.Fatal(err)
	}
	if n := countLines(b); n != 2 || b[len(b)-1] != '\n' {
		t.Fatalf("want 2 newline-ended lines, got %q", b)
	}
	fi, _ := os.Stat(dayFile(dir, "2026-11-15"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Join(dir, DirName))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", di.Mode().Perm())
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestTornLastLine_SkippedOnReadAndNextWriteStartsNewLine(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	s := open(t, dir, now)
	if _, _, err := s.Append(now, []Sample{smp("s", "1", 10)}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(dayFile(dir, "2026-11-14"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"id":"torn","sess`)
	f.Close()

	d, err := s.Day("2026-11-14")
	if err != nil || d.N != 1 {
		t.Fatalf("torn line should be skipped: n=%d err=%v", d.N, err)
	}
	if _, _, err := s.Append(now, []Sample{smp("s", "2", 20)}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Day("2026-11-14")
	if d.N != 2 || d.Value[1] != 20 {
		t.Fatalf("next write should land on its own line: %+v", d.Value)
	}
}

func TestDedupe_AcrossCallsAndRestart(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	s := open(t, dir, now)
	stored, dropped, _ := s.Append(now, []Sample{smp("s", "1", 1), smp("s", "1", 1), smp("other", "1", 1)})
	if stored != 2 || dropped != 1 {
		t.Fatalf("in-batch: %d %d", stored, dropped)
	}
	stored, dropped, _ = s.Append(now, []Sample{smp("s", "1", 1)})
	if stored != 0 || dropped != 1 {
		t.Fatalf("across calls: %d %d", stored, dropped)
	}
	s.Close()
	s2 := open(t, dir, now.Add(time.Hour))
	stored, dropped, _ = s2.Append(now, []Sample{smp("s", "1", 1), smp("s", "2", 1)})
	if stored != 1 || dropped != 1 {
		t.Fatalf("across restart: %d %d", stored, dropped)
	}
}

func TestCaps_PerSessionAndPerDay(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	s := open(t, dir, now)
	var batch []Sample
	for i := range MaxPerSessionPerDay + 5 {
		batch = append(batch, smp("big", fmt.Sprint(i), 1))
	}
	stored, dropped, err := s.Append(now, batch)
	if err != nil || stored != MaxPerSessionPerDay || dropped != 5 {
		t.Fatalf("session cap: %d %d %v", stored, dropped, err)
	}
	// The cap survives a restart.
	s.Close()
	s = open(t, dir, now)
	stored, dropped, _ = s.Append(now, []Sample{smp("big", "new", 1)})
	if stored != 0 || dropped != 1 {
		t.Fatalf("session cap after restart: %d %d", stored, dropped)
	}

	// Fill the day with many sessions until the day cap bites.
	total := MaxPerSessionPerDay
	for sess := 0; total < MaxPerDay; sess++ {
		var b []Sample
		for i := range MaxPerSessionPerDay {
			b = append(b, smp(fmt.Sprint("s", sess), fmt.Sprint(i), 1))
		}
		st, _, err := s.Append(now, b)
		if err != nil {
			t.Fatal(err)
		}
		total += st
	}
	if total != MaxPerDay {
		t.Fatalf("day total %d", total)
	}
	stored, dropped, err = s.Append(now, []Sample{smp("fresh", "1", 1)})
	if err != nil || stored != 0 || dropped != 1 {
		t.Fatalf("day cap: %d %d %v", stored, dropped, err)
	}
}

func TestSweep_DeletesFilesOlderThanRetention(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o700); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -RetentionDays-1)
	edge := now.AddDate(0, 0, -RetentionDays)
	for _, d := range []time.Time{old, edge} {
		if err := os.WriteFile(dayFile(dir, dayOf(d)), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := open(t, dir, now)
	days, _ := s.Days()
	if len(days) != 1 || days[0] != dayOf(edge) {
		t.Fatalf("open should delete the 91-day file: %v", days)
	}
	oldest, ok := s.OldestDay()
	if !ok || oldest != dayOf(edge) {
		t.Fatalf("oldest %q %v", oldest, ok)
	}
	if err := s.Sweep(now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.OldestDay(); ok {
		t.Fatal("daily sweep should delete the file that aged out")
	}
}

func TestDay_ColumnsAndNaNForAbsent(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	s := open(t, dir, now)
	ttfb := 120.0
	a := smp("s", "1", 5)
	a.TTFB = &ttfb
	a.BuildID, a.Env, a.Tab = "b1", "prod", "hard"
	if _, _, err := s.Append(now, []Sample{a, smp("s", "2", 7)}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.Day("2026-11-14")
	if d.Metric.At(0) != "lcp" || d.BuildID.At(0) != "b1" || d.Tab.At(0) != "hard" || d.Session.At(1) != "s" {
		t.Fatal("string columns wrong")
	}
	if d.TTFB[0] != 120 || !math.IsNaN(d.TTFB[1]) {
		t.Fatalf("ttfb %v", d.TTFB)
	}
	empty, _ := s.Day("2026-11-13")
	if empty.N != 0 {
		t.Fatal("missing day should be empty")
	}
}

func TestDay_PastDaysAreNotReRead(t *testing.T) {
	dir := t.TempDir()
	now := at(2026, 11, 14, 10)
	s := open(t, dir, now)
	yesterday := at(2026, 11, 13, 10)
	if _, _, err := s.Append(yesterday, []Sample{smp("s", "1", 1)}); err != nil {
		t.Fatal(err)
	}
	d1, _ := s.Day("2026-11-13")
	// Change the file behind the store's back: a cached past day must not notice.
	if err := os.WriteFile(dayFile(dir, "2026-11-13"), []byte(`{"id":"x","session":"s","metric":"lcp","value":9}`+"\n"+`{"id":"y","session":"s","metric":"lcp","value":9}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d2, _ := s.Day("2026-11-13")
	if d1 != d2 || d2.N != 1 {
		t.Fatalf("past day re-read: n=%d", d2.N)
	}
	// Today is re-read after an append.
	today1, _ := s.Day("2026-11-14")
	if _, _, err := s.Append(now, []Sample{smp("s", "1", 1)}); err != nil {
		t.Fatal(err)
	}
	today2, _ := s.Day("2026-11-14")
	if today1.N != 0 || today2.N != 1 {
		t.Fatalf("today: %d then %d", today1.N, today2.N)
	}
}

func TestPercentile_NearestRank(t *testing.T) {
	vals := []float64{15, 20, 35, 40, 50, math.NaN()}
	for p, want := range map[float64]float64{0: 15, 30: 20, 40: 20, 50: 35, 75: 40, 100: 50} {
		if got, ok := Percentile(vals, p); !ok || got != want {
			t.Errorf("p%v = %v (%v), want %v", p, got, ok, want)
		}
	}
	if _, ok := Percentile(nil, 75); ok {
		t.Error("empty input should report false")
	}
}
