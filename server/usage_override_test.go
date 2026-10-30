package server

import (
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
)

func TestUsageOverride_HoldsUntilResetsAtThenGatesAgain(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	resets := now.Add(time.Hour)
	gated := func(at time.Time) bool {
		_, held := s.usageGate(ralphloop.AgentClaude, at)
		return held
	}

	r.write("a", usageRecord(now, 99, resets))
	s.scanUsage()
	if !gated(now) {
		t.Fatal("99% did not gate")
	}
	res, err := s.usageOverride(now)
	if err != nil || res.Refused {
		t.Fatalf("override = %+v, %v", res, err)
	}
	if !res.Until.Equal(resets) || len(res.Windows) != 1 || res.Windows[0] != "5-hour" {
		t.Errorf("override result = %+v, want the 5-hour window until %s", res, resets)
	}
	if gated(now) {
		t.Fatal("still gated after the override")
	}

	r.write("b", usageRecord(now.Add(time.Minute), 99, resets))
	s.scanUsage()
	if gated(now.Add(2 * time.Minute)) {
		t.Fatal("a new 99% sample in the same window brought the gate back")
	}

	// The next window: a later sample with a new reset gates again.
	next := resets.Add(5 * time.Hour)
	r.write("c", usageRecord(resets.Add(time.Minute), 98, next))
	s.scanUsage()
	if !gated(resets.Add(2 * time.Minute)) {
		t.Fatal("the next window at 98% did not gate")
	}
}

func TestUsageOverride_SurvivesRestart(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	r.write("a", usageRecord(now, 99, now.Add(time.Hour)))
	s.scanUsage()
	if res, err := s.usageOverride(now); err != nil || res.Refused {
		t.Fatalf("override = %+v, %v", res, err)
	}
	if _, _, ok := openUsageSamples(r.stateDir).limitWindow(now); ok {
		t.Fatal("override lost across restart")
	}
}

func TestUsageOverride_RefusesWhenNothingGates(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	res, err := s.usageOverride(now)
	if err != nil || !res.Refused || res.Reason != ReasonNotGated {
		t.Fatalf("no sample: %+v, %v", res, err)
	}
	r.write("a", usageRecord(now, 50, now.Add(time.Hour)))
	s.scanUsage()
	res, err = s.usageOverride(now)
	if err != nil || !res.Refused || res.Reason != ReasonNotGated {
		t.Fatalf("50%%: %+v, %v", res, err)
	}
}
