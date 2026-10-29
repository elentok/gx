package server

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/syshealth"
)

const (
	defaultProcessSecondReadDelay = time.Second
	defaultProcessShareWindow     = time.Second
	defaultProcessCutoff          = 15 * time.Second
	defaultProcessWaitCap         = 5 * time.Second
)

// procWatch is the process table's cache. The table is read only while a
// /v1/health call needs it, never on a timer.
type procWatch struct {
	mu sync.Mutex
	// prev and cur are the last two reads; prev is nil when cur is the first
	// read after a quiet spell, or when the last read failed.
	prev, cur *syshealth.ProcessTable
	curAt     time.Time
	curErr    error
	// inflight is closed when the read in progress ends.
	inflight chan struct{}
	// second is closed when the scheduled second read ends.
	second chan struct{}
}

func durationOr(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

// fetchProcesses takes one read now, sharing it with a caller already reading.
func (s *Server) fetchProcesses() {
	p := &s.procs
	p.mu.Lock()
	if ch := p.inflight; ch != nil {
		p.mu.Unlock()
		<-ch
		return
	}
	done := make(chan struct{})
	p.inflight = done
	p.mu.Unlock()

	t, err := s.cfg.ProcessSource.Read()
	now := time.Now()

	p.mu.Lock()
	quiet := p.cur == nil || now.Sub(p.curAt) > durationOr(s.cfg.ProcessCutoff, defaultProcessCutoff)
	switch {
	case err != nil:
		p.prev, p.cur = nil, nil
	case quiet:
		p.prev, p.cur = nil, &t
	default:
		p.prev, p.cur = p.cur, &t
	}
	p.curErr, p.curAt = err, now
	p.inflight = nil
	p.mu.Unlock()
	close(done)
}

// ensureFresh reads the table unless a read younger than the share window holds.
// It returns early when ctx ends.
func (s *Server) ensureFresh(ctx context.Context) {
	p := &s.procs
	window := durationOr(s.cfg.ProcessShareWindow, defaultProcessShareWindow)
	for {
		p.mu.Lock()
		fresh := !p.curAt.IsZero() && time.Since(p.curAt) < window
		ch := p.inflight
		p.mu.Unlock()
		if fresh {
			return
		}
		if ch == nil {
			s.fetchProcesses()
			return
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return
		}
	}
}

// scheduleSecondRead starts the second read once; the returned channel closes
// when it ends. The caller holds p.mu.
func (s *Server) scheduleSecondRead() chan struct{} {
	p := &s.procs
	if p.second != nil {
		return p.second
	}
	done := make(chan struct{})
	p.second = done
	delay := durationOr(s.cfg.ProcessSecondReadDelay, defaultProcessSecondReadDelay)
	go func() {
		time.Sleep(delay)
		s.fetchProcesses()
		p.mu.Lock()
		p.second = nil
		p.mu.Unlock()
		close(done)
	}()
	return done
}

// healthCauses answers the route's two cause lists. With wait, a call that would
// show the reading marker blocks until the second read ends, ctx ends or the
// cap passes.
func (s *Server) healthCauses(ctx context.Context, wait bool) HealthCauses {
	s.ensureFresh(ctx)
	p := &s.procs
	p.mu.Lock()
	if p.cur != nil && p.prev == nil && wait {
		second := s.scheduleSecondRead()
		p.mu.Unlock()
		t := time.NewTimer(durationOr(s.cfg.ProcessWaitCap, defaultProcessWaitCap))
		defer t.Stop()
		select {
		case <-second:
		case <-ctx.Done():
		case <-t.C:
		}
		p.mu.Lock()
	}
	prev, cur, err := p.prev, p.cur, p.curErr
	if cur != nil && prev == nil {
		s.scheduleSecondRead()
	}
	p.mu.Unlock()

	if cur == nil {
		marker := CauseMarkerUnavailable
		if err == nil {
			// Never read: the caller's context ended before the first read.
			marker = CauseMarkerReading
		}
		return HealthCauses{CPU: CauseList{Marker: marker}, Memory: CauseList{Marker: marker}}
	}
	first := prev
	if first == nil {
		first = cur
	}
	lists := syshealth.TopCauses(*first, *cur, s.healthOwners(*cur), s.cfg.ProcessSource.Argv)
	out := HealthCauses{CPU: causeList(lists.CPU), Memory: causeList(lists.Memory)}
	if prev == nil {
		out.CPU = CauseList{Marker: CauseMarkerReading}
	}
	return out
}

// serverOwners is the owner map with only the gx server's own pid.
func serverOwners(t syshealth.ProcessTable) map[syshealth.OwnerKey]syshealth.Owner {
	pid := os.Getpid()
	for _, p := range t.Processes {
		if p.PID == pid && !p.Unreadable {
			return map[syshealth.OwnerKey]syshealth.Owner{
				{PID: pid, StartUnix: p.StartUnix}: {Kind: syshealth.OwnerServer},
			}
		}
	}
	return nil
}

func causeList(rows []syshealth.Cause) CauseList {
	out := CauseList{Rows: make([]CauseRow, 0, len(rows))}
	for _, c := range rows {
		out.Rows = append(out.Rows, CauseRow{
			Kind: string(c.Kind), Label: c.Label, Busiest: c.Busiest, Ticket: c.Ticket,
			CPU: c.CPU, MemBytes: c.MemBytes,
		})
	}
	return out
}
