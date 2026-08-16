package tickets

import "testing"

func TestBudgetLimitLatch_TripsOnThresholdCross(t *testing.T) {
	var l budgetLimitLatch

	if l.checkAndTrip(9.0, 10.0, nil) {
		t.Fatal("expected no trip while under the limit")
	}
	if !l.checkAndTrip(10.0, 10.0, nil) {
		t.Fatal("expected a trip on crossing the limit")
	}
	if l.checkAndTrip(12.0, 10.0, nil) {
		t.Fatal("expected the latch to stay tripped (no re-fire) on a later over-limit call")
	}
	if l.checkAndTrip(0.0, 10.0, nil) {
		t.Fatal("expected the latch to stay tripped even once total drops back under the limit")
	}
}

func TestBudgetLimitLatch_OverrideSuppressesRetrip(t *testing.T) {
	var l budgetLimitLatch
	if !l.checkAndTrip(12.0, 10.0, nil) {
		t.Fatal("expected the initial trip")
	}

	l.override(12.0) // re-arm point == 12 + 10%*10 == 13
	if l.checkAndTrip(12.5, 10.0, nil) {
		t.Fatal("expected no re-trip while still under the re-arm point")
	}
}

func TestBudgetLimitLatch_RearmClearsOverrideOnceBackAboveRearmPoint(t *testing.T) {
	var l budgetLimitLatch
	if !l.checkAndTrip(12.0, 10.0, nil) {
		t.Fatal("expected the initial trip")
	}

	l.override(12.0) // re-arm point == 13
	if l.checkAndTrip(12.5, 10.0, nil) {
		t.Fatal("expected no re-trip while still under the re-arm point")
	}
	if !l.checkAndTrip(13.5, 10.0, nil) {
		t.Fatal("expected a fresh trip once total climbs past the re-arm point")
	}
	if l.checkAndTrip(20.0, 10.0, nil) {
		t.Fatal("expected the latch to stay tripped (no re-fire) after the fresh trip")
	}
}

func TestBudgetLimitLatch_Reset(t *testing.T) {
	var l budgetLimitLatch
	l.checkAndTrip(12.0, 10.0, nil)
	l.override(12.0)

	l.reset()
	if l.tripped || l.overridden || l.overridePoint != 0 {
		t.Fatalf("reset() left latch = %+v, want zero value", l)
	}
	if !l.checkAndTrip(10.0, 10.0, nil) {
		t.Fatal("expected a reset latch to trip fresh")
	}
}

func TestBudgetLimitRearmPoint(t *testing.T) {
	// Default-config case: the only threshold equals the limit, i.e. no
	// threshold sits strictly above the override point, so the +10%
	// fallback applies.
	if got, want := budgetLimitRearmPoint(20.0, 20.0, []float64{5, 10, 15, 20}), 22.0; got != want {
		t.Fatalf("budgetLimitRearmPoint(default config) = %v, want %v", got, want)
	}
	// Empty thresholds: same +10% fallback.
	if got, want := budgetLimitRearmPoint(20.0, 20.0, nil), 22.0; got != want {
		t.Fatalf("budgetLimitRearmPoint(empty thresholds) = %v, want %v", got, want)
	}
	// A threshold strictly above the override point, but still under the
	// +10% arm, wins (min of the two).
	if got, want := budgetLimitRearmPoint(10.0, 20.0, []float64{5, 11, 15, 20}), 11.0; got != want {
		t.Fatalf("budgetLimitRearmPoint(threshold above, under +10%% arm) = %v, want %v", got, want)
	}
}
