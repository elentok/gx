package syshealth

import "testing"

func TestBusyPercent(t *testing.T) {
	cases := []struct {
		name    string
		prev    *Reading
		cur     Reading
		want    float64
		wantWhy Reason
	}{
		{"no previous", nil, Reading{CPUIdle: 10, CPUTotal: 20}, 0, ReasonWaiting},
		{"half busy", &Reading{CPUIdle: 100, CPUTotal: 200}, Reading{CPUIdle: 150, CPUTotal: 300}, 50, ""},
		{"no ticks", &Reading{CPUIdle: 100, CPUTotal: 200}, Reading{CPUIdle: 100, CPUTotal: 200}, 0, ReasonWaiting},
		{"32-bit wrap", &Reading{CPUIdle: 0xFFFFFFF0, CPUTotal: 0xFFFFFFE0, Wrap32: true}, Reading{CPUIdle: 0x10, CPUTotal: 0x20, Wrap32: true}, 100 * (1 - 32.0/64), ""},
		{"64-bit drop", &Reading{CPUIdle: 100, CPUTotal: 200}, Reading{CPUIdle: 50, CPUTotal: 150}, 0, ReasonError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := BusyPercent(c.prev, c.cur)
			if got != c.want || why != c.wantWhy {
				t.Fatalf("got (%v, %q), want (%v, %q)", got, why, c.want, c.wantWhy)
			}
		})
	}
}

func TestThresholdConstants(t *testing.T) {
	if CPUHigh != 70 || CPUCritical != 90 || MemHigh != 80 || MemCritical != 90 {
		t.Fatalf("thresholds changed: cpu %d/%d mem %d/%d", CPUHigh, CPUCritical, MemHigh, MemCritical)
	}
	if AverageWindow != 12 {
		t.Fatalf("AverageWindow = %d, want 12", AverageWindow)
	}
}

func TestNextState(t *testing.T) {
	cases := []struct {
		name       string
		prev       State
		avg        float64
		high, crit float64
		want       State
	}{
		{"cpu 69 normal", StateNormal, 69, CPUHigh, CPUCritical, StateNormal},
		{"cpu 70 high", StateNormal, 70, CPUHigh, CPUCritical, StateHigh},
		{"cpu 90 critical", StateNormal, 90, CPUHigh, CPUCritical, StateCritical},
		{"mem 79 normal", StateNormal, 79, MemHigh, MemCritical, StateNormal},
		{"mem 80 high", StateNormal, 80, MemHigh, MemCritical, StateHigh},
		{"mem 90 critical", StateNormal, 90, MemHigh, MemCritical, StateCritical},
		{"66 keeps high", StateHigh, 66, CPUHigh, CPUCritical, StateHigh},
		{"65 keeps high", StateHigh, 65, CPUHigh, CPUCritical, StateHigh},
		{"64 clears high", StateHigh, 64, CPUHigh, CPUCritical, StateNormal},
		{"86 keeps critical", StateCritical, 86, CPUHigh, CPUCritical, StateCritical},
		{"84 critical to high", StateCritical, 84, CPUHigh, CPUCritical, StateHigh},
		{"critical straight to normal", StateCritical, 50, CPUHigh, CPUCritical, StateNormal},
		{"unknown prev", StateUnknown, 75, CPUHigh, CPUCritical, StateHigh},
		{"empty prev", "", 10, CPUHigh, CPUCritical, StateNormal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextState(c.prev, c.avg, c.high, c.crit); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWindowAverage(t *testing.T) {
	var w Window
	if w.Average() != 0 {
		t.Fatalf("empty average = %v", w.Average())
	}
	for range AverageWindow - 1 {
		w.Add(10)
	}
	w.Add(100)
	if got := NextState(StateNormal, w.Average(), CPUHigh, CPUCritical); got != StateNormal {
		t.Fatalf("one high among 11 low gave %q (avg %v)", got, w.Average())
	}
	// The spike falls out after 12 more readings.
	for range AverageWindow {
		w.Add(20)
	}
	if w.Average() != 20 {
		t.Fatalf("average after window rolled = %v, want 20", w.Average())
	}
}

func TestNextLevel(t *testing.T) {
	cases := []struct {
		name string
		prev int
		v    float64
		want int
	}{
		{"no history 0", -1, 0, 0},
		{"no history 19", -1, 19, 0},
		{"no history 20", -1, 20, 1},
		{"no history 100", -1, 100, 4},
		{"within 5 above edge keeps", 0, 24, 0},
		{"past 5 above edge rises", 0, 25, 1},
		{"within 5 below edge keeps", 1, 15, 1},
		{"past 5 below edge drops", 1, 14, 0},
		{"jump two steps up", 0, 65, 3},
		{"jump two steps down", 3, 14, 0},
		{"top level holds", 4, 100, 4},
		{"top level drops past edge", 4, 74, 3},
		{"top level holds near edge", 4, 75, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextLevel(c.prev, c.v); got != c.want {
				t.Fatalf("NextLevel(%d, %v) = %d, want %d", c.prev, c.v, got, c.want)
			}
		})
	}
}

func TestDiskState(t *testing.T) {
	cases := []struct {
		name string
		disk Disk
		want State
	}{
		{"above warn", Disk{Free: 500, Total: 1000, CleanBelow: 50}, StateNormal},
		{"warned", Disk{Free: 80, Total: 1000, Warned: true, CleanBelow: 50}, StateHigh},
		{"held", Disk{Free: 80, Total: 1000, Warned: true, Held: true, CleanBelow: 50}, StateCritical},
		{"under clean", Disk{Free: 40, Total: 1000, CleanBelow: 50}, StateCritical},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.disk.State(); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWorstDisk(t *testing.T) {
	ok := Disk{Free: 500, Total: 1000, CleanBelow: 50}
	warned := Disk{Free: 80, Total: 1000, Warned: true, CleanBelow: 50}
	fuller := Disk{Free: 60, Total: 1000, Warned: true, CleanBelow: 50}
	held := Disk{Free: 80, Total: 1000, Held: true, CleanBelow: 50}

	cases := []struct {
		name  string
		disks []Disk
		want  int
	}{
		{"none", nil, -1},
		{"worst state", []Disk{ok, warned}, 1},
		{"worst state first", []Disk{held, warned}, 0},
		{"tie goes to percent used", []Disk{warned, fuller}, 1},
		{"tie other order", []Disk{fuller, warned}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WorstDisk(c.disks); got != c.want {
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestReasonCodesAreLiteral(t *testing.T) {
	if ReasonWaiting != "waiting" || ReasonUnavailable != "unavailable" || ReasonError != "error" {
		t.Fatal("reason codes changed; they are on the wire")
	}
}
