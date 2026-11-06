package notify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestNotificationState_ReadWriteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	trippedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	eventTime := trippedAt.Add(-time.Minute)
	sendTime := trippedAt.Add(-30 * time.Second)

	err := updateNotificationStateAt(path, func(s *NotificationState) {
		s.Transports["telegram"] = TransportState{
			Muted:     true,
			TrippedAt: trippedAt,
			Reason:    "auto-trip",
			Events: []NotificationEvent{
				{EventType: "ticket-stalled", Source: "ticket-01", Time: eventTime},
			},
			Sends: []NotificationSend{
				{Time: sendTime},
			},
			Trips: []TransportTrip{
				{TrippedAt: trippedAt, Reason: "auto-trip", Sources: []SourceCount{
					{Source: "ticket-01", Count: 5},
				}},
			},
		}
	})
	if err != nil {
		t.Fatalf("updateNotificationStateAt: %v", err)
	}

	got, err := loadNotificationStateAt(path)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}

	telegram, ok := got.Transports["telegram"]
	if !ok {
		t.Fatalf("expected telegram transport state, got %+v", got.Transports)
	}
	if !telegram.Muted || telegram.Reason != "auto-trip" || !telegram.TrippedAt.Equal(trippedAt) {
		t.Fatalf("mute fields mismatch: %+v", telegram)
	}
	if len(telegram.Events) != 1 || telegram.Events[0].EventType != "ticket-stalled" ||
		telegram.Events[0].Source != "ticket-01" || !telegram.Events[0].Time.Equal(eventTime) {
		t.Fatalf("event series mismatch: %+v", telegram.Events)
	}
	if len(telegram.Sends) != 1 || !telegram.Sends[0].Time.Equal(sendTime) {
		t.Fatalf("send series mismatch: %+v", telegram.Sends)
	}
	if len(telegram.Trips) != 1 || telegram.Trips[0].Reason != "auto-trip" ||
		len(telegram.Trips[0].Sources) != 1 || telegram.Trips[0].Sources[0].Count != 5 {
		t.Fatalf("trips mismatch: %+v", telegram.Trips)
	}
}

func TestNotificationState_PublicFormsUseTheGivenPath(t *testing.T) {
	t.Parallel()
	path := NotificationStatePath(t.TempDir())
	if err := UpdateNotificationState(path, func(s *NotificationState) {
		s.Transports["slack"] = TransportState{Muted: true, Reason: "manual-disable"}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not written at the given path: %v", err)
	}
	state, err := LoadNotificationState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Transports["slack"].Muted {
		t.Errorf("loaded state = %+v, want slack muted", state)
	}
}

func TestNotificationGate_NoPathFails(t *testing.T) {
	t.Parallel()
	if _, err := notificationGateAt("", "telegram", "ev", "src", time.Now(), true, nil); err == nil {
		t.Error("notificationGateAt with no path = nil error, want one")
	}
}

func TestNotificationState_LoadMissingFileReturnsEmptyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")

	got, err := loadNotificationStateAt(path)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}
	if len(got.Transports) != 0 {
		t.Fatalf("expected empty transports, got %+v", got.Transports)
	}
}

func TestNotificationState_AtomicWrite_NeverExposesTornFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications-state.json")

	// Seed an initial complete state.
	if err := updateNotificationStateAt(path, func(s *NotificationState) {
		s.Transports["slack"] = TransportState{Muted: false}
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// A crash mid-write leaves only a stray temp file behind — never a
	// partially-written notifications-state.json — because the real file is
	// only ever produced by a rename from a fully-written temp file.
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := tmp.Write([]byte(`{"transports":{"slack":{`)); err != nil {
		t.Fatalf("write partial temp: %v", err)
	}
	tmp.Close()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after interrupted write: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("concurrent reader observed a torn write: got %q, want %q", after, before)
	}

	if _, err := loadNotificationStateAt(path); err != nil {
		t.Fatalf("state file is not valid JSON after interrupted write: %v", err)
	}
}

func TestNotificationState_LockTimeout_FailsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		t.Fatalf("hold lock: %v", err)
	}

	start := time.Now()
	err = updateNotificationStateAt(path, func(s *NotificationState) {
		t.Fatal("mutate should not run when the lock can't be acquired")
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrNotificationStateLocked) {
		t.Fatalf("expected ErrNotificationStateLocked, got %v", err)
	}
	if elapsed < lockTimeout {
		t.Fatalf("returned before lockTimeout elapsed: %v", elapsed)
	}
	if elapsed > lockTimeout+2*time.Second {
		t.Fatalf("blocked far longer than lockTimeout: %v", elapsed)
	}
}

func TestNotificationState_TripHistoryCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	base := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)

	for i := range 21 {
		err := updateNotificationStateAt(path, func(s *NotificationState) {
			ts := s.Transports["telegram"]
			ts.Trips = append(ts.Trips, TransportTrip{
				TrippedAt: base.Add(time.Duration(i) * time.Minute),
				Reason:    "auto-trip",
			})
			s.Transports["telegram"] = ts
		})
		if err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}

	got, err := loadNotificationStateAt(path)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}
	trips := got.Transports["telegram"].Trips
	if len(trips) != maxTripsPerTransport {
		t.Fatalf("expected %d trips, got %d", maxTripsPerTransport, len(trips))
	}
	// The oldest trip (index 0) should have been trimmed, keeping trips 1..20.
	if !trips[0].TrippedAt.Equal(base.Add(1 * time.Minute)) {
		t.Fatalf("expected oldest surviving trip to be #1, got %v", trips[0].TrippedAt)
	}
	last := trips[len(trips)-1]
	if !last.TrippedAt.Equal(base.Add(20 * time.Minute)) {
		t.Fatalf("expected newest trip to be #20, got %v", last.TrippedAt)
	}
}

func mustMark(t *testing.T, path, key string) bool {
	t.Helper()
	isNew, err := MarkOnce(path, key)
	if err != nil {
		t.Fatalf("MarkOnce(%q): %v", key, err)
	}
	return isNew
}

func TestNotificationState_MarkOnce_FirstTrueThenFalseAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")

	if !mustMark(t, path, "ready:gx/epic-impl") {
		t.Fatal("first mark should be new")
	}
	if mustMark(t, path, "ready:gx/epic-impl") {
		t.Fatal("second mark should not be new")
	}
	if !mustMark(t, path, "question:s1:1") {
		t.Fatal("a different key should be new")
	}

	got, err := LoadNotificationState(path)
	if err != nil {
		t.Fatalf("LoadNotificationState: %v", err)
	}
	if len(got.Marks) != 2 {
		t.Fatalf("marks after reload = %v, want 2 keys", got.Marks)
	}
	if mustMark(t, path, "question:s1:1") {
		t.Fatal("mark should survive a reload from disk")
	}
}

func TestNotificationState_MarkOnce_OldFormatFileLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	if err := os.WriteFile(path, []byte(`{"transports":{"telegram":{"muted":true,"events":null,"sends":null,"trips":null}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if !mustMark(t, path, "k") {
		t.Fatal("mark on an old-format file should be new")
	}
	got, err := LoadNotificationState(path)
	if err != nil {
		t.Fatalf("LoadNotificationState: %v", err)
	}
	if !got.Transports["telegram"].Muted {
		t.Fatal("marking must keep existing transport state")
	}
}

func TestNotificationState_MarksOmittedWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	if err := UpdateNotificationState(path, func(*NotificationState) {}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "marks") {
		t.Fatalf("empty marks should be omitted, got %s", b)
	}
}

func TestNotificationState_MarkOnce_RaceGivesExactlyOneTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")

	const callers = 2
	var wg sync.WaitGroup
	var news atomic.Int32
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			isNew, err := MarkOnce(path, "question:s1:1")
			if err != nil {
				t.Errorf("MarkOnce: %v", err)
				return
			}
			if isNew {
				news.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := news.Load(); got != 1 {
		t.Fatalf("exactly one caller should win, got %d", got)
	}
}

func TestNotificationState_MarkOnce_CapEvictsOldest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	err := UpdateNotificationState(path, func(s *NotificationState) {
		for i := range maxMarks {
			s.Marks = append(s.Marks, fmt.Sprintf("k%d", i))
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	if !mustMark(t, path, "newest") {
		t.Fatal("newest should be new")
	}

	got, err := LoadNotificationState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Marks) != maxMarks {
		t.Fatalf("marks = %d, want cap %d", len(got.Marks), maxMarks)
	}
	if got.Marks[0] != "k1" || got.Marks[len(got.Marks)-1] != "newest" {
		t.Fatalf("oldest not evicted: first=%q last=%q", got.Marks[0], got.Marks[len(got.Marks)-1])
	}
	if !mustMark(t, path, "k0") {
		t.Fatal("evicted key reads as new again")
	}
}

func TestNotificationState_MarkOnce_LockedReturnsErrorAndRecordsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-state.json")
	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer lockFile.Close()
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	isNew, err := MarkOnce(path, "k")
	if !errors.Is(err, ErrNotificationStateLocked) || isNew {
		t.Fatalf("got (%v, %v), want (false, ErrNotificationStateLocked)", isNew, err)
	}
}
