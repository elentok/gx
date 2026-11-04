package broker

import "testing"

func TestThrottle_ZeroIntervalIsNotDefaulted(t *testing.T) {
	th := NewThrottle(0, func() {})
	if th.interval != 0 {
		t.Fatalf("interval = %v, want 0 (no feature default in the generic throttle)", th.interval)
	}
}

func TestLimitsInterval_ZeroYieldsDefault(t *testing.T) {
	if got := LimitsInterval(0); got != DefaultLimitsEventInterval {
		t.Fatalf("interval = %v, want %v", got, DefaultLimitsEventInterval)
	}
	if got := LimitsInterval(5); got != 5 {
		t.Fatalf("interval = %v, want 5", got)
	}
}
