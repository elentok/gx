package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
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
