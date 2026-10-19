// Package cpuwatch notices when a gx process keeps burning CPU, captures a
// profile of it and reports the spike, so an agent can find the cause.
package cpuwatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultInterval is how often the process's CPU time is read.
	DefaultInterval = 10 * time.Second
	// DefaultProfileFor is how long a spike's CPU profile records.
	DefaultProfileFor = 20 * time.Second
)

// Config is when a spike fires. A zero Threshold turns the watch off.
type Config struct {
	// Threshold is the CPU use, in percent of one core, that counts as high.
	Threshold float64
	// Sustain is how long the use must stay high before a spike fires, and
	// low before a fired spike re-arms.
	Sustain time.Duration
	// Cooldown is the least time between two spikes, so a process that hovers
	// around the threshold does not capture evidence over and over.
	Cooldown time.Duration
}

// Sample is one interval's CPU use, in percent of one core. Children is the
// subprocesses that exited in the interval (herdr, git): a profile never shows
// them, so they are kept apart.
type Sample struct {
	At       time.Time `json:"at"`
	Self     float64   `json:"self"`
	Children float64   `json:"children"`
}

// Total is the sample's whole CPU use.
func (s Sample) Total() float64 { return s.Self + s.Children }

// Spike is a fired spike: the high samples and where its evidence was written.
type Spike struct {
	// Source names the process: "server" or "tui".
	Source  string   `json:"source"`
	PID     int      `json:"pid"`
	Build   string   `json:"build,omitempty"`
	Samples []Sample `json:"samples"`
	// Dir holds cpu.pprof, goroutines.txt and summary.md.
	Dir string `json:"dir"`
}

// Watcher samples its own process and reports spikes.
type Watcher struct {
	Config
	Source string
	Build  string
	// Dir is the evidence root; each spike gets a timestamped dir in it.
	Dir    string
	Report func(Spike)

	// Interval and ProfileFor default to DefaultInterval and DefaultProfileFor.
	Interval   time.Duration
	ProfileFor time.Duration
	// Usage reads the CPU time used so far; nil means getrusage.
	Usage func() (self, children time.Duration)
	// Settings, when set, is read before each sample so a config reload
	// applies without a restart; it wins over Config.
	Settings func() Config

	streak []Sample
	// fired holds a spike back until the use stays below the threshold for
	// Sustain, from calmSince, so one long spike reports once.
	fired     bool
	calmSince time.Time
	lastFired time.Time
}

// Run samples until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	usage := w.Usage
	if usage == nil {
		usage = rusage
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastAt := time.Now()
	lastSelf, lastChildren := usage()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			self, children := usage()
			wall := now.Sub(lastAt)
			s := Sample{At: now, Self: percent(self-lastSelf, wall), Children: percent(children-lastChildren, wall)}
			lastAt, lastSelf, lastChildren = now, self, children
			if samples, ok := w.observe(s); ok {
				w.fire(ctx, samples)
			}
		}
	}
}

// observe adds s and returns the high samples when they make a spike.
func (w *Watcher) observe(s Sample) ([]Sample, bool) {
	cfg := w.Config
	if w.Settings != nil {
		cfg = w.Settings()
	}
	if cfg.Threshold <= 0 {
		w.streak, w.fired, w.calmSince = nil, false, time.Time{}
		return nil, false
	}
	if s.Total() < cfg.Threshold {
		w.streak = nil
		if w.fired {
			if w.calmSince.IsZero() {
				w.calmSince = s.At
			}
			if s.At.Sub(w.calmSince) >= cfg.Sustain {
				w.fired, w.calmSince = false, time.Time{}
			}
		}
		return nil, false
	}
	w.calmSince = time.Time{}
	w.streak = append(w.streak, s)
	// Keep only the last Sustain of samples: a spike held back by the
	// cooldown reports its recent use, and a long one does not grow forever.
	for len(w.streak) > 1 && s.At.Sub(w.streak[1].At) >= cfg.Sustain {
		w.streak = w.streak[1:]
	}
	if w.fired || s.At.Sub(w.streak[0].At) < cfg.Sustain {
		return nil, false
	}
	if !w.lastFired.IsZero() && s.At.Sub(w.lastFired) < cfg.Cooldown {
		return nil, false
	}
	w.fired, w.lastFired = true, s.At
	samples := w.streak
	w.streak = nil
	return samples, true
}

// fire writes the evidence and reports the spike. A failed capture still
// reports: the samples alone say the process was busy.
func (w *Watcher) fire(ctx context.Context, samples []Sample) {
	spike := Spike{Source: w.Source, PID: os.Getpid(), Build: w.Build, Samples: samples}
	dir := filepath.Join(w.Dir, fmt.Sprintf("%s-%s-%d", time.Now().Format("20060102-150405"), w.Source, spike.PID))
	if err := os.MkdirAll(dir, 0o755); err == nil {
		spike.Dir = dir
		profileFor := w.ProfileFor
		if profileFor <= 0 {
			profileFor = DefaultProfileFor
		}
		errs := capture(ctx, dir, profileFor)
		_ = os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summary(spike, errs)), 0o644)
	}
	if w.Report != nil {
		w.Report(spike)
	}
}

// capture writes a CPU profile and a goroutine dump into dir and returns what
// failed. Another profile already running makes the CPU one fail.
func capture(ctx context.Context, dir string, profileFor time.Duration) []error {
	var errs []error
	if g, err := os.Create(filepath.Join(dir, "goroutines.txt")); err != nil {
		errs = append(errs, err)
	} else {
		if err := pprof.Lookup("goroutine").WriteTo(g, 2); err != nil {
			errs = append(errs, fmt.Errorf("goroutine dump: %w", err))
		}
		g.Close()
	}
	f, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		return append(errs, err)
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		return append(errs, fmt.Errorf("cpu profile: %w", err))
	}
	select {
	case <-ctx.Done():
	case <-time.After(profileFor):
	}
	pprof.StopCPUProfile()
	return errs
}

func summary(s Spike, errs []error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# CPU spike: gx %s (pid %d)\n\n", s.Source, s.PID)
	if s.Build != "" {
		fmt.Fprintf(&b, "Build: %s\n\n", s.Build)
	}
	b.WriteString("Children is CPU used by subprocesses that exited (herdr, git, ...). `cpu.pprof` does not show it: look in `goroutines.txt` for loops that start processes.\n\n")
	b.WriteString("| time | self % | children % |\n|---|---|---|\n")
	for _, x := range s.Samples {
		fmt.Fprintf(&b, "| %s | %.1f | %.1f |\n", x.At.Format(time.TimeOnly), x.Self, x.Children)
	}
	for _, err := range errs {
		fmt.Fprintf(&b, "\nCapture failed: %v\n", err)
	}
	return b.String()
}

func percent(cpu, wall time.Duration) float64 {
	if wall <= 0 {
		return 0
	}
	return 100 * float64(cpu) / float64(wall)
}

// rusage is the CPU time this process and its waited-for children used.
func rusage() (self, children time.Duration) {
	return rusageOf(syscall.RUSAGE_SELF), rusageOf(syscall.RUSAGE_CHILDREN)
}

func rusageOf(who int) time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(who, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
