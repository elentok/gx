package notify

import (
	"path/filepath"
	"testing"
	"time"
)

var capT0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func capPath(t *testing.T) string { return filepath.Join(t.TempDir(), "notifications-state.json") }

// flood makes n attempts one second apart from start and returns how many went
// out and how many tripped.
func flood(path, channel string, start time.Time, n int) (sent, trips int) {
	for i := range n {
		send, trip := ChannelCap(path, channel, start.Add(time.Duration(i)*time.Second), 0)
		if send {
			sent++
		}
		if trip {
			trips++
		}
	}
	return sent, trips
}

func TestChannelBreaker_TripsOnTheTwentyFirstAttemptAndSendsOneNotice(t *testing.T) {
	path := capPath(t)
	sent, trips := flood(path, "desktop", capT0, 20)
	if sent != 20 || trips != 0 {
		t.Fatalf("first 20: sent=%d trips=%d", sent, trips)
	}
	send, trip := ChannelCap(path, "desktop", capT0.Add(20*time.Second), 0)
	if send || !trip {
		t.Fatalf("21st: send=%v trip=%v, want dropped with the edge", send, trip)
	}
	sent, trips = flood(path, "desktop", capT0.Add(21*time.Second), 5)
	if sent != 0 || trips != 0 {
		t.Fatalf("after trip: sent=%d trips=%d, want none", sent, trips)
	}
}

func TestChannelBreaker_ClearsAfterFiveQuietMinutes(t *testing.T) {
	path := capPath(t)
	flood(path, "push", capT0, 21)
	last := capT0.Add(20 * time.Second)
	if send, _ := ChannelCap(path, "push", last.Add(5*time.Minute-time.Second), 0); send {
		t.Fatal("4m59s after the last attempt the breaker must stay tripped")
	}
	// That drop was an attempt, so the quiet time restarts from it.
	if send, _ := ChannelCap(path, "push", last.Add(10*time.Minute-time.Second), 0); !send {
		t.Fatal("5m after the last attempt the breaker must clear and send")
	}
}

func TestChannelBreaker_DroppedAttemptsKeepItTripped(t *testing.T) {
	path := capPath(t)
	flood(path, "web-toast", capT0, 21)
	at := capT0.Add(20 * time.Second)
	for range 3 {
		at = at.Add(4 * time.Minute)
		if send, _ := ChannelCap(path, "web-toast", at, 0); send {
			t.Fatalf("attempt at %v went out; each dropped attempt must push the clear time back", at)
		}
	}
	st, err := LoadNotificationState(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := st.Transports["web-toast"]
	if !ts.ChannelTripped(at, 0) || !ts.TrippedAt.Equal(capT0.Add(20*time.Second)) {
		t.Fatalf("state = %+v", ts)
	}
}

func TestChannelBreaker_ChannelsAreIndependent(t *testing.T) {
	path := capPath(t)
	flood(path, "desktop", capT0, 21)
	if sent, _ := flood(path, "push", capT0, 20); sent != 20 {
		t.Fatalf("push sent %d, want 20 while desktop is tripped", sent)
	}
	if send, _ := ChannelCap(path, "slack", capT0, 0); !send {
		t.Fatal("slack must be unaffected")
	}
	st, _ := LoadNotificationState(path)
	if st.Transports["push"].Muted || len(st.Transports["desktop"].Trips) != 1 {
		t.Fatalf("state = %+v", st.Transports)
	}
}
