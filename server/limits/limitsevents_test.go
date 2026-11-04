package limits_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const limitsEventInterval = 50 * time.Millisecond

func limitsEventHarness(t *testing.T) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(),
		func(c *server.Config) { c.LimitsEventInterval = limitsEventInterval })
}

// limitsEvents counts the limits-changed events on ch until it is quiet.
func limitsEvents(ch <-chan wire.Event) int {
	n := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == wire.EventLimitsChanged {
				n++
			}
		case <-time.After(10 * limitsEventInterval):
			return n
		}
	}
}

func wantLimitsEvents(t *testing.T, ch <-chan wire.Event, want int) {
	t.Helper()
	if got := limitsEvents(ch); got != want {
		t.Fatalf("limits-changed events = %d, want %d", got, want)
	}
}

func TestLimitsEvent_FirstReportPublishesOne(t *testing.T) {
	h := limitsEventHarness(t)
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	wantLimitsEvents(t, ch, 1)
}

func TestLimitsEvent_ChangedPercentagePublishes(t *testing.T) {
	h := limitsEventHarness(t)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(40, 4_000_000_000)})
	wantLimitsEvents(t, ch, 1)
}

func TestLimitsEvent_FractionOnlyChangePublishesNothing(t *testing.T) {
	h := limitsEventHarness(t)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34.1, 4_000_000_000)})
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34.3, 4_000_000_000)})
	wantLimitsEvents(t, ch, 0)
}

func TestLimitsEvent_ConfirmOfStaleWindowPublishes(t *testing.T) {
	h := limitsEventHarness(t)
	old := time.Now().Unix() - 2*wire.LimitsStaleAfterSeconds
	seed := wire.LimitsReport{FiveHour: &wire.LimitWindow{UsedPercentage: 34, ResetsAt: 4_000_000_000, ReceivedAt: old}}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "claude-limits.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	wantLimitsEvents(t, ch, 1)
}

func TestLimitsEvent_ConfirmOfFreshWindowPublishesNothing(t *testing.T) {
	h := limitsEventHarness(t)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	wantLimitsEvents(t, ch, 0)
}

func TestLimitsEvent_DroppedReportPublishesNothing(t *testing.T) {
	h := limitsEventHarness(t)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(50, 4_000_000_000)})
	ch := servertest.EventStream(t, h)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(40, 4_000_000_000)})
	wantLimitsEvents(t, ch, 0)
}

func TestLimitsEvent_BurstCoalescesAndTrailingPublishShowsLastValue(t *testing.T) {
	h := limitsEventHarness(t)
	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(1, 4_000_000_000)})
	ch := servertest.EventStream(t, h)
	for pct := 2; pct <= 21; pct++ {
		sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(float64(pct), 4_000_000_000)})
	}
	got := limitsEvents(ch)
	if got < 1 || got >= 20 {
		t.Fatalf("limits-changed events = %d, want at least a trailing one and far fewer than 20", got)
	}
	res := sendLimits(t, h, wire.LimitsReport{})
	if res.FiveHour == nil || res.FiveHour.UsedPercentage != 21 {
		t.Fatalf("stored = %+v, want 21", res.FiveHour)
	}
}
