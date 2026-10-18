package cpuwatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// feed observes one sample per 10s at the given totals and returns the index of
// each sample that fired.
func feed(w *Watcher, totals ...float64) []int {
	var fired []int
	for i, total := range totals {
		if _, ok := w.observe(Sample{At: t0.Add(time.Duration(i) * 10 * time.Second), Self: total}); ok {
			fired = append(fired, i)
		}
	}
	return fired
}

func TestObserve_SustainedHighFiresOnce(t *testing.T) {
	w := &Watcher{Config: Config{Threshold: 15, Sustain: time.Minute}}
	got := feed(w, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20)
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("fired at %v, want once at sample 6 (60s after the first high one)", got)
	}
}

func TestObserve_ShortBurstDoesNotFire(t *testing.T) {
	w := &Watcher{Config: Config{Threshold: 15, Sustain: time.Minute}}
	if got := feed(w, 90, 90, 90, 5, 90, 90, 90, 5); len(got) != 0 {
		t.Fatalf("fired at %v, want never", got)
	}
}

func TestObserve_FiresAgainAfterDrop(t *testing.T) {
	w := &Watcher{Config: Config{Threshold: 15, Sustain: 20 * time.Second}}
	got := feed(w, 20, 20, 20, 20, 5, 20, 20, 20)
	if len(got) != 2 || got[0] != 2 || got[1] != 7 {
		t.Fatalf("fired at %v, want at 2 and 7", got)
	}
}

func TestObserve_CountsChildren(t *testing.T) {
	w := &Watcher{Config: Config{Threshold: 15, Sustain: 10 * time.Second}}
	for i := range 2 {
		samples, ok := w.observe(Sample{At: t0.Add(time.Duration(i) * 10 * time.Second), Self: 5, Children: 12})
		if i == 1 && (!ok || len(samples) != 2) {
			t.Fatalf("observe = %v, %v; want a 2-sample spike from children CPU", samples, ok)
		}
	}
}

func TestObserve_SettingsWinAndZeroIsOff(t *testing.T) {
	w := &Watcher{Config: Config{Threshold: 15, Sustain: 0}, Settings: func() Config { return Config{} }}
	if got := feed(w, 99, 99, 99); len(got) != 0 {
		t.Fatalf("fired at %v with the watch off", got)
	}
}

func TestFire_WritesEvidenceAndReports(t *testing.T) {
	var got []Spike
	w := &Watcher{Source: "server", Build: "v1", Dir: t.TempDir(), ProfileFor: time.Millisecond, Report: func(s Spike) { got = append(got, s) }}
	w.fire(context.Background(), []Sample{{At: t0, Self: 20, Children: 3}})
	if len(got) != 1 || got[0].Dir == "" {
		t.Fatalf("reports = %+v, want one with an evidence dir", got)
	}
	for _, name := range []string{"cpu.pprof", "goroutines.txt", "summary.md"} {
		if _, err := os.Stat(filepath.Join(got[0].Dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	sum, _ := os.ReadFile(filepath.Join(got[0].Dir, "summary.md"))
	if !strings.Contains(string(sum), "| 20.0 | 3.0 |") {
		t.Errorf("summary.md lacks the sample row:\n%s", sum)
	}
}
