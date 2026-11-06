package queue

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
)

// sweepOneOffs moves every done or cancelled ticket of a one-offs epic into
// one-offs-archived, whatever wrote its status. It runs after each rescan, so a
// crash between a move's two writes is finished by the first rescan of the next
// server. A ticket whose run is still tracked stays: the run's post-merging
// steps (retro stamp, result, follow-up) still read it at its old address.
func (o *OneOffs) Sweep() { o.SweepAt(time.Now()) }

const (
	sweepBackoffBase = time.Minute
	sweepBackoffCap  = time.Hour
)

// sweepBackoff is one stuck ticket's retry state.
type sweepBackoff struct {
	failures int
	retryAt  time.Time
}

// SweepAt is Sweep with the clock supplied, so a test can walk the back-off.
// A ticket whose archive failed is retried after 1 minute, doubling up to a
// 1 hour cap, so one stuck ticket does not cost its failure on every rescan.
// Only the first failure logs at warn; the retries stay quiet. The state is in
// memory and ends when the ticket archives or leaves the store.
func (o *OneOffs) SweepAt(now time.Time) {
	if o.TicketStore == "" {
		return
	}
	o.sweepMu.Lock()
	defer o.sweepMu.Unlock()
	if o.backoff == nil {
		o.backoff = map[string]*sweepBackoff{}
	}
	seen := map[string]bool{}
	listed := true
	defer func() {
		if !listed {
			return // a project that failed to load may still hold its stuck tickets
		}
		for key := range o.backoff {
			if !seen[key] {
				delete(o.backoff, key)
			}
		}
	}()

	dirs, err := tickets.ProjectDirs(o.TicketStore)
	if err != nil {
		listed = false
		o.Log.Warn("sweep one-offs", "err", err)
		return
	}
	moved := false
	for _, dir := range dirs {
		live, archived, err := tickets.LoadOneOffs(dir)
		if err != nil {
			listed = false
			o.Log.Warn("sweep one-offs", "dir", dir, "err", err)
			continue
		}
		for _, t := range live.Tickets {
			if !tickets.IsTerminalStatus(t.Status) {
				continue
			}
			addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: OneOffsEpic, ID: t.Identifier}
			if _, tracked := o.Registry.Tracked(addr.String()); tracked {
				continue
			}
			key := addr.String()
			seen[key] = true
			b := o.backoff[key]
			if b != nil && now.Before(b.retryAt) {
				continue
			}
			if err := o.archiveOneOff(dir, archived, addr, t, now); err != nil {
				if b == nil {
					b = &sweepBackoff{}
					o.backoff[key] = b
					o.Log.Warn("archive one-off", "ticket", key, "err", err)
				} else {
					o.Log.Debug("archive one-off retry failed", "ticket", key, "err", err)
				}
				b.failures++
				b.retryAt = now.Add(min(sweepBackoffBase<<(b.failures-1), sweepBackoffCap))
				continue
			}
			delete(o.backoff, key)
			moved = true
		}
	}
	if moved {
		if err := o.Index.Refresh(); err != nil {
			o.Log.Warn("refresh after one-off sweep", "err", err)
		}
	}
	for _, dir := range dirs {
		o.dequeueIdleOneOffs(dir)
	}
}

// dequeueIdleOneOffs takes the project's one-offs root off the queue once no
// live one-off is left; the next submit queues it again. A submit writes its
// ticket before it looks at the queue, so the disk check under the queue lock
// either sees that ticket (root stays) or the submit finds the root gone and
// queues it afresh.
func (o *OneOffs) dequeueIdleOneOffs(dir string) {
	root := RootRef{Project: tickets.ProjectName(dir), Epic: OneOffsEpic}
	removed, err := o.Queue.removeRootIf(root, func() bool {
		return !o.oneOffsLive(dir, root)
	})
	if err != nil {
		o.Log.Warn("dequeue idle one-offs", "root", root, "err", err)
	} else if removed {
		o.Events.Publish(wire.EventQueueChanged, root.String())
	}
}

// oneOffsLive reports whether a one-off is still pending, or its run is tracked.
// Failed, paused and needs-* tickets are not terminal, so they count.
func (o *OneOffs) oneOffsLive(dir string, root RootRef) bool {
	live, _, err := tickets.LoadOneOffs(dir)
	if err != nil {
		return true
	}
	for _, t := range live.Tickets {
		addr := tickets.Address{Project: root.Project, Epic: OneOffsEpic, ID: t.Identifier}
		if _, tracked := o.Registry.Tracked(addr.String()); tracked {
			return true
		}
		if !tickets.IsTerminalStatus(t.Status) {
			return true
		}
	}
	return false
}

// archiveOneOff moves one terminal one-off into the archive epic, creating the
// epic on the first move. The archive copy is written before the original is
// removed; a copy already there (its Resolution names addr) means an earlier
// move was cut short, so only its leftovers are finished.
func (o *OneOffs) archiveOneOff(projectDir string, archived tickets.Epic, addr tickets.Address, t tickets.Ticket, now time.Time) error {
	archivePath := filepath.Join(projectDir, OneOffsArchivedEpic)
	if err := o.Host.EnsureDraftEpic(archivePath); err != nil {
		return err
	}
	unlock, err := ticketmove.LockEpics(filepath.Join(projectDir, OneOffsEpic), archivePath)
	if err != nil {
		return err
	}
	defer unlock()

	// The index may be a scan behind: only a ticket still terminal on disk moves.
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tickets.IsTerminalStatus(string(tk.Status)) {
		return nil
	}
	for _, c := range archived.Tickets {
		if tickets.IsOriginal(c, addr) {
			to := tickets.Address{Project: addr.Project, Epic: OneOffsArchivedEpic, ID: c.Identifier}
			return ticketmove.ResumeMove(projectDir, addr, to, t.Path, c.Path)
		}
	}
	completed := tk.IterationFinishedAt
	if completed.IsZero() {
		completed = now.UTC()
	}
	p, err := ticketmove.PlanMove(projectDir, addr, t, OneOffsArchivedEpic, "", ticketmove.MoveOpts{Archive: true, AppendOriginal: true, CompletedAt: completed})
	if err != nil {
		return err
	}
	return ticketmove.ApplyMove(p)
}
