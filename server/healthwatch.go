package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/elentok/gx/syshealth"
)

const defaultHealthPoll = 5 * time.Second

// waitingMeter is a meter with no value yet.
var waitingMeter = MeterHealth{State: HealthStateUnknown, Reason: HealthReasonWaiting}

// meterTrack is one meter's history between readings.
type meterTrack struct {
	win  syshealth.Window
	high float64
	crit float64
}

// next adds v and returns the meter for the new average. prev is the meter
// before this reading.
func (m *meterTrack) next(prev MeterHealth, v float64, at time.Time) MeterHealth {
	m.win.Add(v)
	avg := m.win.Average()
	return MeterHealth{
		Level:  syshealth.NextLevel(levelSeed(prev), avg),
		State:  HealthState(syshealth.NextState(syshealth.State(prev.State), avg, m.high, m.crit)),
		ReadAt: at,
	}
}

// levelSeed is the level a new reading starts from: none after an unknown meter.
func levelSeed(prev MeterHealth) int {
	if prev.State == HealthStateUnknown {
		return -1
	}
	return prev.Level
}

// syshealthDisks converts the disk watcher's statuses for the health rules.
func syshealthDisks(statuses []DiskStatus, cleanBelow uint64) []syshealth.Disk {
	in := make([]syshealth.Disk, len(statuses))
	for i, d := range statuses {
		in[i] = syshealth.Disk{Free: d.FreeBytes, Total: d.TotalBytes, Warned: d.Warned, Held: d.Held, CleanBelow: cleanBelow}
	}
	return in
}

// healthWatch is the CPU and memory meters. Every reading goes through observe.
type healthWatch struct {
	mu     sync.Mutex
	health Health
	prev   *syshealth.Reading
	// started is set by the first reading, which sets the baseline without an event.
	started bool
	cpu     meterTrack
	mem     meterTrack
	latest  latestReading
}

// latestReading is the newest raw values the health route shows beside the meters.
type latestReading struct {
	cpuPct, cpuAvg, memPct, memAvg float64
	memUsed, memTotal              uint64
	pressure                       int
}

func (s *Server) initHealth() {
	s.healthW.health = Health{CPU: waitingMeter, Memory: waitingMeter, Disk: waitingMeter}
	s.healthW.cpu = meterTrack{high: syshealth.CPUHigh, crit: syshealth.CPUCritical}
	s.healthW.mem = meterTrack{high: syshealth.MemHigh, crit: syshealth.MemCritical}
}

// observeHealth applies one read result and publishes an event when a level or
// state changed.
func (s *Server) observeHealth(r syshealth.Reading, err error, now time.Time) {
	w := &s.healthW
	w.mu.Lock()
	old := w.health
	switch {
	case err != nil:
		reason := HealthReasonError
		if errors.Is(err, syshealth.ErrUnavailable) {
			reason = HealthReasonUnavailable
		}
		unknown := MeterHealth{State: HealthStateUnknown, Reason: reason, ReadAt: now}
		w.health.CPU, w.health.Memory = unknown, unknown
		w.prev = nil
		w.latest = latestReading{}
	default:
		busy, reason := syshealth.BusyPercent(w.prev, r)
		w.prev = &r
		w.latest.cpuPct, w.latest.cpuAvg = 0, 0
		if reason != "" {
			w.health.CPU = MeterHealth{State: HealthStateUnknown, Reason: HealthReason(reason), ReadAt: now}
		} else {
			w.health.CPU = w.cpu.next(old.CPU, busy, now)
			w.latest.cpuPct, w.latest.cpuAvg = busy, w.cpu.win.Average()
		}
		w.health.Memory = w.mem.next(old.Memory, r.MemPercent, now)
		w.latest.memPct, w.latest.memAvg = r.MemPercent, w.mem.win.Average()
		w.latest.memUsed, w.latest.memTotal, w.latest.pressure = r.MemUsed, r.MemTotal, r.Pressure
	}
	cur, first := w.health, !w.started
	w.started = true
	w.mu.Unlock()
	if !first && (meterChanged(old.CPU, cur.CPU) || meterChanged(old.Memory, cur.Memory) || meterChanged(old.Disk, cur.Disk)) {
		s.events.publish(EventHealthChanged, "")
	}
}

// observeDiskHealth recomputes the disk meter from the disk watcher's own
// status, the same list the disk status route shows, so the Tray and the banner
// agree. It publishes an event only when the disk level or state changed.
func (s *Server) observeDiskHealth(now time.Time) {
	in := syshealthDisks(s.diskStatuses(false), uint64(s.settings.Load().DiskSpace.CleanBelow))
	worst := syshealth.WorstDisk(in)
	w := &s.healthW
	w.mu.Lock()
	old := w.health.Disk
	cur := waitingMeter
	if worst >= 0 {
		cur = MeterHealth{
			Level:  syshealth.NextLevel(levelSeed(old), in[worst].PercentUsed()),
			State:  HealthState(in[worst].State()),
			ReadAt: now,
		}
	}
	w.health.Disk = cur
	started := w.started
	w.mu.Unlock()
	if started && meterChanged(old, cur) {
		s.events.publish(EventHealthChanged, "")
	}
}

// meterChanged compares what a client shows; the reading time alone is no change.
func meterChanged(a, b MeterHealth) bool {
	return a.Level != b.Level || a.State != b.State || a.Reason != b.Reason
}

func (s *Server) currentHealth() Health {
	s.healthW.mu.Lock()
	defer s.healthW.mu.Unlock()
	return s.healthW.health
}

// keepHealthWatched reads CPU and memory on a timer until ctx ends, taking the
// first reading at once.
func (s *Server) keepHealthWatched(ctx context.Context) {
	read := s.cfg.SystemHealth
	if read == nil {
		read = syshealth.Default()
	}
	interval := s.cfg.HealthPollInterval
	if interval <= 0 {
		interval = defaultHealthPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		r, err := read()
		s.observeHealth(r, err, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
