package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/cpuwatch"
)

// CPUSpikeDir is where the server and every TUI write CPU spike evidence.
func CPUSpikeDir(stateDir string) string { return filepath.Join(stateDir, "cpu-spikes") }

// CPUWatchConfig maps cpu-watchdog to a watcher's trigger; off is the zero
// Config.
func CPUWatchConfig(c config.CPUWatchdogConfig) cpuwatch.Config {
	if !c.Enabled {
		return cpuwatch.Config{}
	}
	return cpuwatch.Config{Threshold: c.ThresholdPercent, Sustain: c.Sustain}
}

// CPUSpikeResult is what became of a reported spike.
type CPUSpikeResult struct {
	// Address is the investigate ticket filed for it; empty when none was.
	Address string `json:"address,omitempty"`
	// Message says why no ticket was filed.
	Message string `json:"message,omitempty"`
}

// cpuSpikes holds when the last CPU ticket was filed, for the cooldown. It
// lives in memory: a restart lets one more through.
type cpuSpikes struct {
	mu        sync.Mutex
	lastFiled time.Time
}

// keepCPUWatched watches the server's own CPU use until ctx ends.
func (s *Server) keepCPUWatched(ctx context.Context) {
	w := &cpuwatch.Watcher{
		Source:   "server",
		Build:    s.cfg.Build,
		Dir:      CPUSpikeDir(s.cfg.StateDir),
		Settings: func() cpuwatch.Config { return CPUWatchConfig(s.settings.Load().CPUWatchdog) },
		Report:   func(sp cpuwatch.Spike) { s.cpuSpike(sp, time.Now()) },
	}
	w.Run(ctx)
}

// cpuSpike logs a spike and, in evolve maintainer mode, files a ticket in the
// gx project for an agent to find its cause. The cooldown and the one-off's
// dedupe key keep a busy gx from filing more than one at a time.
func (s *Server) cpuSpike(sp cpuwatch.Spike, now time.Time) CPUSpikeResult {
	s.log.Warn("cpu spike", "source", sp.Source, "pid", sp.PID, "avg_percent", avgCPU(sp.Samples), "evidence", sp.Dir)
	set := s.settings.Load()
	if !set.CPUWatchdog.Enabled {
		return CPUSpikeResult{Message: "cpu-watchdog is off"}
	}
	if set.Evolve != config.EvolveMaintainer {
		return CPUSpikeResult{Message: "filing a ticket needs evolve maintainer mode"}
	}
	if _, err := s.projectDir(OutboxProject); err != nil {
		return CPUSpikeResult{Message: "filing a ticket needs a gx project: " + err.Error()}
	}
	s.cpuSpikes.mu.Lock()
	defer s.cpuSpikes.mu.Unlock()
	if last := s.cpuSpikes.lastFiled; !last.IsZero() && now.Sub(last) < set.CPUWatchdog.Cooldown {
		return CPUSpikeResult{Message: fmt.Sprintf("cooling down: last ticket filed %s ago", now.Sub(last).Round(time.Second))}
	}
	res, err := s.oneOff(OneOffRequest{
		Prompt:  cpuSpikePrompt(sp, set.CPUWatchdog),
		Project: OutboxProject,
		Name:    "cpu-spike-" + sp.Source + "-" + now.Format("20060102-1504"),
		Unique:  "cpu-spike:" + sp.Source,
		Front:   true,
		Notify:  true,
	})
	switch {
	case err != nil:
		s.log.Warn("cpu spike: file ticket", "err", err)
		return CPUSpikeResult{Message: err.Error()}
	case res.Refused:
		s.log.Info("cpu spike: ticket refused", "reason", res.Reason, "message", res.Message)
		return CPUSpikeResult{Message: res.Message}
	}
	s.cpuSpikes.lastFiled = now
	s.log.Info("cpu spike: ticket filed", "address", res.Address)
	return CPUSpikeResult{Address: res.Address}
}

func cpuSpikePrompt(sp cpuwatch.Spike, c config.CPUWatchdogConfig) string {
	var b strings.Builder
	self, children := 0.0, 0.0
	for _, x := range sp.Samples {
		self += x.Self
		children += x.Children
	}
	if n := float64(len(sp.Samples)); n > 0 {
		self, children = self/n, children/n
	}
	fmt.Fprintf(&b, "Investigate a CPU spike in gx %s (pid %d", sp.Source, sp.PID)
	if sp.Build != "" {
		fmt.Fprintf(&b, ", build %s", sp.Build)
	}
	fmt.Fprintf(&b, "). It used %.0f%% of a core on average (%.0f%% itself, %.0f%% in subprocesses) for %s, above the cpu-watchdog threshold of %.0f%%.\n\n",
		self+children, self, children, spikeLength(sp.Samples), c.ThresholdPercent)
	if sp.Dir == "" {
		b.WriteString("No evidence was captured; reproduce the load and profile it.\n\n")
	} else {
		fmt.Fprintf(&b, "Evidence in %s:\n\n", sp.Dir)
		fmt.Fprintf(&b, "- summary.md: the samples\n- cpu.pprof: a CPU profile (`go tool pprof -top %s`)\n- goroutines.txt: every goroutine's stack during the spike\n\n", filepath.Join(sp.Dir, "cpu.pprof"))
	}
	b.WriteString("Subprocess CPU (herdr, git, ...) does not show in the profile: look for loops that start processes.\n\n")
	b.WriteString("Find the cause and report it with file:line references. This ticket is commitless: put the fix in a follow-up ticket (`gx tickets add --parent`) instead of committing here.")
	return b.String()
}

// spikeLength is the time the samples cover, counting the first interval.
func spikeLength(samples []cpuwatch.Sample) time.Duration {
	if len(samples) < 2 {
		return cpuwatch.DefaultInterval
	}
	step := samples[1].At.Sub(samples[0].At)
	return (samples[len(samples)-1].At.Sub(samples[0].At) + step).Round(time.Second)
}

func avgCPU(samples []cpuwatch.Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range samples {
		sum += x.Total()
	}
	return sum / float64(len(samples))
}

// cpuSpikeWrite takes a TUI's spike report.
func (s *Server) cpuSpikeWrite(w http.ResponseWriter, r *http.Request) {
	var sp cpuwatch.Spike
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sp.Source == "" {
		http.Error(w, "a spike needs a source", http.StatusBadRequest)
		return
	}
	writeJSON(w, s.cpuSpike(sp, time.Now()))
}
