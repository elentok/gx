package server

import "testing"

func TestEventThrottle_ZeroIntervalIsNotDefaulted(t *testing.T) {
	th := newEventThrottle(0, func() {})
	if th.interval != 0 {
		t.Fatalf("interval = %v, want 0 (no feature default in the generic throttle)", th.interval)
	}
}

func TestLimitsEventInterval_ZeroYieldsDefault(t *testing.T) {
	if got := limitsEventInterval(Config{}); got != DefaultLimitsEventInterval {
		t.Fatalf("interval = %v, want %v", got, DefaultLimitsEventInterval)
	}
	cfg := Config{LimitsEventInterval: 5}
	if got := limitsEventInterval(cfg); got != 5 {
		t.Fatalf("interval = %v, want 5", got)
	}
}
