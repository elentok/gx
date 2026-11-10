package merge

import (
	"slices"

	"github.com/elentok/gx/config"
	rlmerge "github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// EpicMarkMerged records a merge done outside gx. It refuses an epic with a
// running or queued ticket and one that is not complete, so it cannot hide
// unfinished work; an epic already stamped is a no-op success. In a `pr`
// project Trunk is brought up to origin first and merged_at is stamped only
// once that succeeded; a failure leaves the stored block and comes back as the
// refusal's reason.
func (s *Merger) EpicMarkMerged(req wire.QueueRequest) (wire.QueueResult, error) {
	m, err := s.loadMergeEpic(req)
	if err != nil {
		return mergeVerbError(err)
	}
	if !tickets.LoadEpicTicketMD(m.epic.Path).MergedAt.IsZero() {
		return wire.QueueResult{}, nil
	}
	if r := s.markMergedRefusal(m); r != nil {
		return *r, nil
	}
	// A pending fix or Merge ticket is the PR flow's own work; the merge ends it.
	for _, t := range m.epic.Tickets {
		if isFlowTicket(t) && !m.epic.RenderedStatus(t).Terminal() {
			if err := ticketops.CancelWithResolution(t.Path, "marked merged by hand"); err != nil {
				return wire.QueueResult{}, err
			}
		}
	}
	if m.epic.PRNumber != 0 && !hasEntry(m.epic.Path, prflow.KindMarkedMerged) {
		if err := prflow.Append(m.epic.Path, prflow.Entry{Kind: prflow.KindMarkedMerged, Detail: m.epic.PRURL}); err != nil {
			return wire.QueueResult{}, err
		}
	}
	if pf, _ := config.ReadProjectFile(m.dir); pf.DeliveryIsPR() {
		if s.SyncTrunkAfterMerge(m.root) {
			_ = prflow.Append(m.epic.Path, prflow.Entry{Kind: prflow.KindTrunkUpdated})
			return wire.QueueResult{}, nil
		}
		fresh := tickets.LoadEpicAt(m.epic.Path)
		return queue.Refusal(fresh.MergeBlockKind, fresh.MergeBlockReason), nil
	}
	return wire.QueueResult{}, s.stampMarkedMerged(m)
}

// stampMarkedMerged stamps a merge-delivery epic under the Trunk lock and
// dequeues its root. No notice goes out: the person did the merge.
func (s *Merger) stampMarkedMerged(m mergeEpicRef) error {
	_, _, target, err := s.EpicRefs(m.root.Project, m.epics, m.root.Epic, m.repo)
	if err != nil {
		return err
	}
	unlock := s.guards.Trunks.Lock(m.repo, target)
	if tickets.LoadEpicTicketMD(m.epic.Path).MergedAt.IsZero() {
		added, deleted, cerr := rlmerge.EpicTrailerLines(m.repo, "refs/heads/"+m.epic.Name, m.epic.Name)
		s.stampMergedEpic(m.root, m.epic, added, deleted, cerr)
		s.StampEpic(m.root, m.epic.Path, "merged_at")
	}
	unlock()
	s.clearMergeBlock(m.root, m.epic.Path)
	removed, err := s.queue.RemoveRoot(m.root)
	if err != nil {
		s.log.Warn("dequeue marked-merged root", "root", m.root, "err", err)
	}
	if removed {
		s.events.Publish(wire.EventQueueChanged, m.root.String())
		s.events.Publish(wire.EventRootCompleted, m.root.String())
	}
	return nil
}

// markMergedRefusal is why e cannot be marked merged, nil when it can. Live
// work is checked before completeness: a running ticket also makes the epic
// incomplete, and the live reason is the one to act on.
func (s *Merger) markMergedRefusal(m mergeEpicRef) *wire.QueueResult {
	queued := map[string]bool{}
	for _, item := range s.queue.List() {
		if a, err := tickets.ParseQueuedAddress(item.Address); err == nil && queue.RootOf(a) == m.root {
			queued[a.ID] = true
		}
	}
	complete := len(m.epic.Tickets) > 0
	for _, t := range m.epic.Tickets {
		st := m.epic.RenderedStatus(t)
		if st.Terminal() {
			continue
		}
		if st == tickets.StatusClaimed || queued[t.Identifier] {
			r := queue.Refusal(wire.ReasonTicketLive, m.root.String()+" has a running or queued ticket: "+t.Identifier)
			return &r
		}
		if !isFlowTicket(t) {
			complete = false
		}
	}
	if !complete {
		r := queue.Refusal(wire.ReasonNotComplete, m.root.String()+" is not complete")
		return &r
	}
	return nil
}

func isFlowTicket(t tickets.Ticket) bool {
	return t.Type == string(schema.TypeCIFix) || t.Type == string(schema.TypeMerge)
}

func hasEntry(dir string, k prflow.Kind) bool {
	entries, err := prflow.Read(dir)
	return err == nil && slices.ContainsFunc(entries, func(e prflow.Entry) bool { return e.Kind == k })
}
