package server

import (
	"cmp"
	"fmt"
	"github.com/elentok/gx/server/wire"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
)

// cardSectionOrder is the order the page lists the sections in.
var cardSectionOrder = []wire.CardSection{
	wire.SectionNeedsYou, wire.SectionImplementing, wire.SectionReadyToQueue, wire.SectionDesigning, wire.SectionReadyToDesign, wire.SectionFinished,
}

// loopCardSection is the section of a loop epic row.
func loopCardSection(state wire.EpicListState) wire.CardSection {
	switch state {
	case wire.EpicListPaused, wire.EpicListMergeBlocked:
		return wire.SectionNeedsYou
	case wire.EpicListRunning, wire.EpicListQueued, wire.EpicListMerging, wire.EpicListAwaitingMerge:
		return wire.SectionImplementing
	case wire.EpicListReady:
		return wire.SectionReadyToQueue
	case wire.EpicListFinished:
		return wire.SectionFinished
	}
	return wire.SectionReadyToDesign
}

// loopStatusWords is the status line of a loop epic row; position is the
// 1-based queue slot of a queued epic.
func loopStatusWords(state wire.EpicListState, position int) string {
	switch state {
	case wire.EpicListRunning, wire.EpicListPaused:
		return "Implementing..."
	case wire.EpicListMerging:
		return "Merging..."
	case wire.EpicListAwaitingMerge, wire.EpicListMergeBlocked:
		return "Implemented"
	case wire.EpicListQueued:
		return fmt.Sprintf("Queued #%d", position)
	case wire.EpicListFinished:
		return "Merged"
	case wire.EpicListDraft:
		return "Draft"
	case wire.EpicListNoOpenTickets:
		return "No open tickets"
	}
	return "Not started"
}

// decorateLoopCard fills the card fields of a loop epic row.
func decorateLoopCard(row *wire.EpicListRow, position int) {
	row.Addresses = []string{row.Address}
	row.CardKind = wire.CardKindImplementation
	row.Section = loopCardSection(row.State)
	row.StatusWords = loopStatusWords(row.State, position)
	if row.Total > 0 {
		row.RingProgress = row.Done * 100 / row.Total
	}
	row.Actions = []wire.CardAction{}
	if row.State == wire.EpicListReady {
		row.Actions = append(row.Actions, wire.CardAction{Kind: wire.CardActionQueue, Label: "Queue"})
	}
}

// queueKey is the queue slot key of an item address: its epic root, or the
// ticket itself for a one-off.
func queueKey(a tickets.Address) string {
	if isOneOffsEpicName(a.Epic) {
		return a.String()
	}
	return rootOf(a).String()
}

// queueIndex maps queue keys (see queueKey) to their slot and earliest enqueue
// time. It is built once per /v1/epics request, not per card.
type queueIndex struct {
	slots    map[string]int
	enqueued map[string]time.Time
}

// newQueueIndex indexes the queue. A slot is 1-based, in queue order; an epic's
// tickets share one slot and each one-off ticket has its own. A key whose items
// have no enqueue time has no entry in enqueued.
func (s *Server) newQueueIndex() queueIndex {
	idx := queueIndex{slots: map[string]int{}, enqueued: map[string]time.Time{}}
	for _, it := range s.queued.list() {
		a, err := parseQueuedAddress(it.Address)
		if err != nil {
			continue
		}
		k := queueKey(a)
		if _, ok := idx.slots[k]; !ok {
			idx.slots[k] = len(idx.slots) + 1
		}
		if at := idx.enqueued[k]; !it.EnqueuedAt.IsZero() && (at.IsZero() || it.EnqueuedAt.Before(at)) {
			idx.enqueued[k] = it.EnqueuedAt
		}
	}
	return idx
}

// slot is the 1-based slot of key, or 0 when the queue holds none of its items.
func (q queueIndex) slot(key string) int { return q.slots[key] }

// enqueuedAt is the earliest enqueue time among key's items; zero when none has one.
func (q queueIndex) enqueuedAt(key string) time.Time { return q.enqueued[key] }

// cardTimeSources are the times a card's time pair draws on beyond the row itself.
type cardTimeSources struct {
	// startedAt is when the card's work began: the epic's started_at, a Design's
	// design start or a one-off's claim.
	startedAt time.Time
	// enqueuedAt is the earliest enqueue time of the card's queue items.
	enqueuedAt time.Time
}

// cardTimeFor maps the row's section to the card's time pair; a section without
// one, or a zero start, gives none. Call it once, after the row's section is final.
func cardTimeFor(row wire.EpicListRow, src cardTimeSources) (wire.CardTimeKind, time.Time) {
	var kind wire.CardTimeKind
	var start time.Time
	switch row.Section {
	case wire.SectionNeedsYou:
		kind, start = wire.CardTimeWaiting, row.WaitingSince
	case wire.SectionImplementing, wire.SectionDesigning:
		kind, start = wire.CardTimeElapsed, src.startedAt
		if row.State == wire.EpicListQueued {
			start = cmp.Or(src.enqueuedAt, row.QueuedAt)
		}
	case wire.SectionReadyToQueue, wire.SectionReadyToDesign:
		kind, start = wire.CardTimeCreated, row.CreatedAt
	case wire.SectionFinished:
		kind, start = wire.CardTimeFinished, finishedAt(row)
	}
	if start.IsZero() {
		return "", time.Time{}
	}
	return kind, start
}

// setCardTime sets the row's time pair from cardTimeFor.
func setCardTime(r *wire.EpicListRow, src cardTimeSources) {
	r.TimeKind, r.TimeStart = cardTimeFor(*r, src)
}

// implementingRank orders Implementing: running, then queued, then the rest.
func implementingRank(r wire.EpicListRow) int {
	switch r.State {
	case wire.EpicListRunning:
		return 0
	case wire.EpicListQueued:
		return 1
	}
	return 2
}

// designingRank orders Designing: live Designs first, then stopped ones.
func designingRank(r wire.EpicListRow) int {
	if slices.ContainsFunc(r.Actions, isContinue) {
		return 1
	}
	return 0
}

func isContinue(a wire.CardAction) bool { return a.Kind == wire.CardActionContinue }

func followUpsRank(r wire.EpicListRow) int {
	if r.CardKind == wire.CardKindFollowUps {
		return 0
	}
	return 1
}

// finishedAt is when a Finished card landed: its merge, else its completion.
func finishedAt(r wire.EpicListRow) time.Time { return cmp.Or(r.MergedAt, r.CompletedAt) }

// sortCards orders rows by section, then by the section's rule. Rows without a
// section keep their place after the sectioned ones.
func sortCards(rows []wire.EpicListRow, queuePosition func(wire.EpicListRow) int) {
	rank := func(r wire.EpicListRow) int {
		i := slices.Index(cardSectionOrder, r.Section)
		if i < 0 {
			return len(cardSectionOrder)
		}
		return i
	}
	slices.SortStableFunc(rows, func(a, b wire.EpicListRow) int {
		if c := cmp.Compare(rank(a), rank(b)); c != 0 {
			return c
		}
		switch a.Section {
		case wire.SectionNeedsYou:
			return a.WaitingSince.Compare(b.WaitingSince)
		case wire.SectionImplementing:
			if c := cmp.Compare(implementingRank(a), implementingRank(b)); c != 0 {
				return c
			}
			if a.State == wire.EpicListQueued {
				return cmp.Compare(queuePosition(a), queuePosition(b))
			}
			if implementingRank(a) >= 2 {
				return b.LastActivityAt.Compare(a.LastActivityAt)
			}
		case wire.SectionDesigning:
			if c := cmp.Compare(designingRank(a), designingRank(b)); c != 0 {
				return c
			}
			return b.LastActivityAt.Compare(a.LastActivityAt)
		case wire.SectionReadyToQueue:
			return b.CreatedAt.Compare(a.CreatedAt)
		case wire.SectionReadyToDesign:
			// Follow-ups cards come first, then the rest by name.
			if c := cmp.Compare(followUpsRank(a), followUpsRank(b)); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		case wire.SectionFinished:
			return finishedAt(b).Compare(finishedAt(a))
		}
		return 0
	})
}
