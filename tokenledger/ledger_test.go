package tokenledger

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func openAt(t *testing.T, dir, today string) *Ledger {
	t.Helper()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := time.ParseInLocation(DayLayout, today, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	l.now = func() time.Time { return d }
	return l
}

func key(day, project, group, harness string) Key {
	return Key{Day: day, Project: project, Group: group, Harness: harness}
}

func mustDay(t *testing.T, l *Ledger, day string) []Row {
	t.Helper()
	rows, err := l.Day(day)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestTokenLedgerRangeMatchesDays(t *testing.T) {
	l := openAt(t, t.TempDir(), "2026-05-12")
	for _, day := range []string{"2026-05-09", "2026-05-10", "2026-05-11", "2026-05-12"} {
		if err := l.Add(Delta{Key: key(day, "gx", GroupQueue, "claude"), Tokens: 1}); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, firstLive, err := l.Range("2026-05-10", "2026-05-11")
	if err != nil {
		t.Fatal(err)
	}
	want := append(mustDay(t, l, "2026-05-10"), mustDay(t, l, "2026-05-11")...)
	if !reflect.DeepEqual(rows, want) || len(rows) != 2 {
		t.Fatalf("Range rows = %v, want %v", rows, want)
	}
	if firstLive != "2026-05-12" {
		t.Fatalf("firstLive = %q", firstLive)
	}
}

func TestTokenLedgerBucketKeySplits(t *testing.T) {
	l := openAt(t, t.TempDir(), "2026-05-10")
	base := key("2026-05-10", "gx", GroupQueue, "claude")
	deltas := []Key{
		base,
		key("2026-05-11", "gx", GroupQueue, "claude"),
		key("2026-05-10", "other", GroupQueue, "claude"),
		key("2026-05-10", "gx", GroupDesign, "claude"),
		key("2026-05-10", "gx", GroupQueue, "codex"),
		key("2026-05-10", "", GroupQueue, "claude"),
	}
	for _, k := range deltas {
		if err := l.Add(Delta{Key: k, Tokens: 10, Dollars: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Add(Delta{Key: base, Tokens: 5}); err != nil {
		t.Fatal(err)
	}
	if got := len(mustDay(t, l, "2026-05-10")); got != 5 {
		t.Fatalf("2026-05-10 buckets = %d, want 5", got)
	}
	for _, r := range mustDay(t, l, "2026-05-10") {
		if r.Key == base && r.Tokens != 15 {
			t.Fatalf("base tokens = %d, want 15", r.Tokens)
		}
	}
	if got := len(mustDay(t, l, "2026-05-11")); got != 1 {
		t.Fatalf("2026-05-11 buckets = %d, want 1", got)
	}
}

func TestTokenLedgerZeroDeltaWritesNothing(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-10", "gx", GroupQueue, "claude")
	if err := l.Add(Delta{Key: k, Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	if err := l.SetSeen("s1", Seen{Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	before, _ := os.ReadFile(path)
	fi1, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if err := l.Add(Delta{Key: k}); err != nil {
		t.Fatal(err)
	}
	if err := l.SetSeen("s1", Seen{Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	fi2, _ := os.Stat(path)
	if string(before) != string(after) || !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("file changed on a zero delta / unchanged seen")
	}
}

func TestTokenLedgerRecordAppliesDeltaAndSeenInOneWrite(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-10", "gx", GroupQueue, "claude")
	if err := l.Record(Delta{Key: k, Tokens: 4, Dollars: 0.25}, "s1", Seen{Tokens: 9, Dollars: 0.5}); err != nil {
		t.Fatal(err)
	}
	// A fresh handle reads only what the single save wrote.
	l2 := openAt(t, dir, "2026-05-10")
	rows := mustDay(t, l2, "2026-05-10")
	if len(rows) != 1 || rows[0].Tokens != 4 || rows[0].Dollars != 0.25 {
		t.Fatalf("delta not stored: %+v", rows)
	}
	if s, ok, err := l2.Seen("s1"); err != nil || !ok || s != (Seen{Tokens: 9, Dollars: 0.5}) {
		t.Fatalf("seen = %+v %v %v", s, ok, err)
	}
}

func TestTokenLedgerRecordZeroDeltaStillSetsSeen(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-10", "gx", GroupQueue, "claude")
	if err := l.Record(Delta{Key: k}, "s1", Seen{Tokens: 3}); err != nil {
		t.Fatal(err)
	}
	if rows := mustDay(t, l, "2026-05-10"); len(rows) != 0 {
		t.Fatalf("zero delta made a bucket: %+v", rows)
	}
	if s, ok, _ := openAt(t, dir, "2026-05-10").Seen("s1"); !ok || s.Tokens != 3 {
		t.Fatalf("seen = %+v %v", s, ok)
	}
}

func TestTokenLedgerRecordUnchangedWritesNothing(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-10", "gx", GroupQueue, "claude")
	if err := l.Record(Delta{Key: k, Tokens: 1}, "s1", Seen{Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	before, _ := os.ReadFile(path)
	fi1, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if err := l.Record(Delta{Key: k}, "s1", Seen{Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	fi2, _ := os.Stat(path)
	if string(before) != string(after) || !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("file changed on a zero delta / unchanged seen")
	}
}

func TestTokenLedgerFirstSeenWrites(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	if err := l.SetSeen("s1", Seen{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("first record of a session did not write: %v", err)
	}
	if d, _ := l.FirstLiveDay(); d != "2026-05-10" {
		t.Fatalf("first live day = %q", d)
	}
}

func TestTokenLedgerRestartKeepsState(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-01-01", "gx", GroupQueue, "claude")
	if err := l.Add(Delta{Key: k, Tokens: 7, Dollars: 0.5}); err != nil {
		t.Fatal(err)
	}
	if err := l.SetSeen("s1", Seen{Tokens: 7, Dollars: 0.5}); err != nil {
		t.Fatal(err)
	}
	l2 := openAt(t, dir, "2026-05-20")
	s, ok, err := l2.Seen("s1")
	if err != nil || !ok || s.Tokens != 7 {
		t.Fatalf("seen = %+v %v %v", s, ok, err)
	}
	rows := mustDay(t, l2, "2026-01-01")
	if len(rows) != 1 || rows[0].Tokens != 7 {
		t.Fatalf("old day lost: %+v", rows)
	}
}

func TestTokenLedgerFirstLiveDayStampedOnce(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-10", "gx", GroupQueue, "claude")
	if err := l.Add(Delta{Key: k, Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	l.now = func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local) }
	if err := l.Add(Delta{Key: k, Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	if d, _ := l.FirstLiveDay(); d != "2026-05-10" {
		t.Fatalf("first live day = %q, want 2026-05-10", d)
	}
}

func TestTokenLedgerApplyRebuiltSkipsLiveDayAndToday(t *testing.T) {
	// No live row yet: today and later are skipped.
	l := openAt(t, t.TempDir(), "2026-05-10")
	rows := []Row{
		{Key: key("2026-05-09", "gx", GroupQueue, "claude"), Tokens: 5},
		{Key: key("2026-05-10", "gx", GroupQueue, "claude"), Tokens: 5},
		{Key: key("2026-05-10", "gx", GroupDesign, "claude"), Tokens: 5},
	}
	w, s, err := l.ApplyRebuilt(rows)
	if err != nil || w != 1 || s != 1 {
		t.Fatalf("written=%d skipped=%d err=%v", w, s, err)
	}
	if d, _ := l.FirstRebuiltDay(); d != "2026-05-09" {
		t.Fatalf("first rebuilt day = %q", d)
	}
	if r := mustDay(t, l, "2026-05-09"); len(r) != 1 || !r[0].Rebuilt {
		t.Fatalf("rows = %+v", r)
	}

	// With a live row on 05-08 (stamped from the clock), days on/after it are skipped.
	l2 := openAt(t, t.TempDir(), "2026-05-08")
	if err := l2.Add(Delta{Key: key("2026-05-08", "gx", GroupQueue, "claude"), Tokens: 1}); err != nil {
		t.Fatal(err)
	}
	l2.now = func() time.Time { return time.Date(2026, 5, 20, 0, 0, 0, 0, time.Local) }
	w, s, _ = l2.ApplyRebuilt([]Row{
		{Key: key("2026-05-07", "gx", GroupQueue, "claude"), Tokens: 2},
		{Key: key("2026-05-08", "gx", GroupQueue, "claude"), Tokens: 2},
		{Key: key("2026-05-12", "gx", GroupQueue, "claude"), Tokens: 2},
	})
	if w != 1 || s != 2 {
		t.Fatalf("written=%d skipped=%d", w, s)
	}
	if r := mustDay(t, l2, "2026-05-08"); len(r) != 1 || r[0].Tokens != 1 || r[0].Rebuilt {
		t.Fatalf("live day mixed with rebuilt: %+v", r)
	}
}

func TestTokenLedgerPlanRebuiltMatchesApplyWithoutWriting(t *testing.T) {
	l := openAt(t, t.TempDir(), "2026-05-10")
	rows := []Row{
		{Key: key("2026-05-08", "gx", GroupQueue, "claude"), Tokens: 5, Dollars: 1},
		{Key: key("2026-05-08", "gx", GroupDesign, "claude"), Tokens: 3, Dollars: 2},
		{Key: key("2026-05-10", "gx", GroupQueue, "claude"), Tokens: 9},
	}
	days, skipped, tokens, dollars, err := l.PlanRebuilt(rows)
	if err != nil || days != 1 || skipped != 1 || tokens != 8 || dollars != 3 {
		t.Fatalf("days=%d skipped=%d tokens=%d dollars=%v err=%v", days, skipped, tokens, dollars, err)
	}
	if r := mustDay(t, l, "2026-05-08"); len(r) != 0 {
		t.Fatalf("plan wrote rows: %+v", r)
	}
	if _, s, _ := l.ApplyRebuilt(rows); s != skipped {
		t.Fatalf("apply skipped %d, plan %d", s, skipped)
	}
}

func TestTokenLedgerApplyRebuiltNeverLowersAndRepeatWritesNothing(t *testing.T) {
	dir := t.TempDir()
	l := openAt(t, dir, "2026-05-10")
	k := key("2026-05-01", "gx", GroupQueue, "claude")
	if _, _, err := l.ApplyRebuilt([]Row{{Key: k, Tokens: 100, Dollars: 2}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	before, _ := os.ReadFile(path)
	fi1, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)

	w, _, err := l.ApplyRebuilt([]Row{{Key: k, Tokens: 100, Dollars: 2}})
	if err != nil || w != 0 {
		t.Fatalf("repeat written=%d err=%v", w, err)
	}
	w, _, _ = l.ApplyRebuilt([]Row{{Key: k, Tokens: 40, Dollars: 1}})
	if w != 0 {
		t.Fatalf("lowered input wrote %d rows", w)
	}
	after, _ := os.ReadFile(path)
	fi2, _ := os.Stat(path)
	if string(before) != string(after) || !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("file rewritten")
	}
	if r := mustDay(t, l, "2026-05-01"); r[0].Tokens != 100 {
		t.Fatalf("bucket lowered: %+v", r)
	}

	// A higher value replaces it.
	w, _, _ = l.ApplyRebuilt([]Row{{Key: k, Tokens: 150, Dollars: 3}})
	if w != 1 {
		t.Fatalf("raise wrote %d", w)
	}
	if r := mustDay(t, l, "2026-05-01"); r[0].Tokens != 150 {
		t.Fatalf("bucket = %+v", r)
	}
}

func TestTokenLedgerTwoWritersLoseNoRow(t *testing.T) {
	dir := t.TempDir()
	a := openAt(t, dir, "2026-05-10")
	b := openAt(t, dir, "2026-05-10")
	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := a.Add(Delta{Key: key("2026-05-10", "a", GroupQueue, "claude"), Tokens: 1}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := b.Add(Delta{Key: key("2026-05-10", "b", GroupQueue, "claude"), Tokens: 1}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	c := openAt(t, dir, "2026-05-10")
	got := map[string]int64{}
	for _, r := range mustDay(t, c, "2026-05-10") {
		got[r.Project] = r.Tokens
	}
	if got["a"] != n || got["b"] != n {
		t.Fatalf("lost rows: %+v", got)
	}
}

func TestTokenLedgerReadSeesOtherWriter(t *testing.T) {
	dir := t.TempDir()
	a := openAt(t, dir, "2026-05-10")
	b := openAt(t, dir, "2026-05-10")
	if got := mustDay(t, a, "2026-05-01"); len(got) != 0 {
		t.Fatalf("rows = %+v", got)
	}
	if _, _, err := b.ApplyRebuilt([]Row{{Key: key("2026-05-01", "gx", GroupQueue, "claude"), Tokens: 3}}); err != nil {
		t.Fatal(err)
	}
	if got := mustDay(t, a, "2026-05-01"); len(got) != 1 {
		t.Fatalf("reader missed the other writer's row: %+v", got)
	}
}
