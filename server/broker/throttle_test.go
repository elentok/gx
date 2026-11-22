package broker_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/broker/brokertest"
)

func TestThrottle_BurstFiresLeadingThenOneTrailingWhenWindowEnds(t *testing.T) {
	clock := brokertest.NewFakeClock()
	fires := 0
	th := broker.NewThrottleClock(300*time.Millisecond, clock.Clock(), func() { fires++ })

	for range 5 {
		th.Trigger()
	}
	if fires != 1 {
		t.Fatalf("fires after burst = %d, want 1 (the leading fire)", fires)
	}
	clock.Advance(299 * time.Millisecond)
	if fires != 1 {
		t.Fatalf("fires before the window ends = %d, want 1", fires)
	}
	clock.Advance(time.Millisecond)
	if fires != 2 {
		t.Fatalf("fires after the window ends = %d, want 2 (leading plus one trailing)", fires)
	}
	clock.Advance(time.Hour)
	if fires != 2 {
		t.Fatalf("fires after idle = %d, want 2", fires)
	}
}

func TestThrottle_StopCancelsTrailingFire(t *testing.T) {
	clock := brokertest.NewFakeClock()
	fires := 0
	th := broker.NewThrottleClock(time.Second, clock.Clock(), func() { fires++ })
	th.Trigger()
	th.Trigger()
	th.Stop()
	clock.Advance(time.Hour)
	if fires != 1 {
		t.Fatalf("fires = %d, want 1", fires)
	}
}
