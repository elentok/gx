package server

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
)

// CardSection is the Epics page section a card belongs to. The server decides it;
// the web client does not derive sections.
type CardSection string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	SectionNeedsYou      CardSection = "needs-you"
	SectionInProgress    CardSection = "in-progress"
	SectionReadyToQueue  CardSection = "ready-to-queue"
	SectionReadyToDesign CardSection = "ready-to-design"
	SectionFinished      CardSection = "finished"
)

// cardSectionOrder is the order the page lists the sections in.
var cardSectionOrder = []CardSection{
	SectionNeedsYou, SectionInProgress, SectionReadyToQueue, SectionReadyToDesign, SectionFinished,
}

// CardKind is the kind of an Epics page card.
type CardKind string

const (
	CardKindImplementation CardKind = "implementation"
	CardKindDesign         CardKind = "design"
	CardKindOneOff         CardKind = "one-off"
	CardKindFollowUps      CardKind = "follow-ups"
)

// CardActionKind names what a card button does.
type CardActionKind string

const (
	CardActionQueue      CardActionKind = "queue"
	CardActionAnswer     CardActionKind = "answer"
	CardActionOpenTicket CardActionKind = "open-ticket"
	CardActionOpenEpic   CardActionKind = "open-epic"
	CardActionMerge      CardActionKind = "merge"
	// StashMerge targets the epic like Merge; its route is a later ticket's.
	CardActionStashMerge CardActionKind = "stash-merge"
	// The Design actions target the Design's epic address. Design and Continue
	// start a session first (POST open-design); Answer and Open only navigate.
	CardActionDesign   CardActionKind = "design"
	CardActionContinue CardActionKind = "continue"
	CardActionOpen     CardActionKind = "open"
	// Triage targets the project whose follow-ups the card counts.
	CardActionTriage CardActionKind = "triage"
)

// CardAction is one button of a card. Target is the address the button opens (a
// ticket for Answer and Open ticket, an epic for Open epic) or acts on (Merge).
type CardAction struct {
	Kind   CardActionKind `json:"kind"`
	Label  string         `json:"label"`
	Target string         `json:"target,omitempty"`
	// Link is the hash route the button opens, when the server decides it. Answer
	// sets it for a Design question; the client falls back to the ticket page.
	Link string `json:"link,omitempty"`
}

// loopCardSection is the section of a loop epic row.
func loopCardSection(state EpicListState) CardSection {
	switch state {
	case EpicListPaused, EpicListMergeBlocked:
		return SectionNeedsYou
	case EpicListRunning, EpicListQueued, EpicListMerging, EpicListAwaitingMerge:
		return SectionInProgress
	case EpicListReady:
		return SectionReadyToQueue
	case EpicListFinished:
		return SectionFinished
	}
	return SectionReadyToDesign
}

// loopStatusWords is the status line of a loop epic row; position is the
// 1-based queue slot of a queued epic.
func loopStatusWords(state EpicListState, position int) string {
	switch state {
	case EpicListRunning, EpicListPaused:
		return "Implementing..."
	case EpicListMerging:
		return "Merging..."
	case EpicListAwaitingMerge, EpicListMergeBlocked:
		return "Implemented"
	case EpicListQueued:
		return fmt.Sprintf("Queued #%d", position)
	case EpicListFinished:
		return "Merged"
	case EpicListDraft:
		return "Draft"
	case EpicListNoOpenTickets:
		return "No open tickets"
	}
	return "Not started"
}

// decorateLoopCard fills the card fields of a loop epic row.
func decorateLoopCard(row *EpicListRow, position int) {
	row.Addresses = []string{row.Address}
	row.CardKind = CardKindImplementation
	row.Section = loopCardSection(row.State)
	row.StatusWords = loopStatusWords(row.State, position)
	if row.Total > 0 {
		row.RingProgress = row.Done * 100 / row.Total
	}
	row.Actions = []CardAction{}
	if row.State == EpicListReady {
		row.Actions = append(row.Actions, CardAction{Kind: CardActionQueue, Label: "Queue"})
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

// CardTimeKind says what the card's start time measures; the client words it.
type CardTimeKind string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	CardTimeElapsed  CardTimeKind = "elapsed"
	CardTimeWaiting  CardTimeKind = "waiting"
	CardTimeCreated  CardTimeKind = "created"
	CardTimeFinished CardTimeKind = "finished"
)

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
func cardTimeFor(row EpicListRow, src cardTimeSources) (CardTimeKind, time.Time) {
	var kind CardTimeKind
	var start time.Time
	switch row.Section {
	case SectionNeedsYou:
		kind, start = CardTimeWaiting, row.WaitingSince
	case SectionInProgress:
		kind, start = CardTimeElapsed, src.startedAt
		if row.State == EpicListQueued {
			start = cmp.Or(src.enqueuedAt, row.QueuedAt)
		}
	case SectionReadyToQueue, SectionReadyToDesign:
		kind, start = CardTimeCreated, row.CreatedAt
	case SectionFinished:
		kind, start = CardTimeFinished, finishedAt(row)
	}
	if start.IsZero() {
		return "", time.Time{}
	}
	return kind, start
}

// setCardTime sets the row's time pair from cardTimeFor.
func (r *EpicListRow) setCardTime(src cardTimeSources) {
	r.TimeKind, r.TimeStart = cardTimeFor(*r, src)
}

// inProgressRank orders In progress: running, then queued, then the rest, then
// stopped Designs.
func inProgressRank(r EpicListRow) int {
	switch {
	case r.State == EpicListRunning:
		return 0
	case r.State == EpicListQueued:
		return 1
	case r.CardKind == CardKindDesign && slices.ContainsFunc(r.Actions, isContinue):
		return 3
	}
	return 2
}

func isContinue(a CardAction) bool { return a.Kind == CardActionContinue }

func followUpsRank(r EpicListRow) int {
	if r.CardKind == CardKindFollowUps {
		return 0
	}
	return 1
}

// finishedAt is when a Finished card landed: its merge, else its completion.
func finishedAt(r EpicListRow) time.Time { return cmp.Or(r.MergedAt, r.CompletedAt) }

// sortCards orders rows by section, then by the section's rule. Rows without a
// section keep their place after the sectioned ones.
func sortCards(rows []EpicListRow, queuePosition func(EpicListRow) int) {
	rank := func(r EpicListRow) int {
		i := slices.Index(cardSectionOrder, r.Section)
		if i < 0 {
			return len(cardSectionOrder)
		}
		return i
	}
	slices.SortStableFunc(rows, func(a, b EpicListRow) int {
		if c := cmp.Compare(rank(a), rank(b)); c != 0 {
			return c
		}
		switch a.Section {
		case SectionNeedsYou:
			return a.WaitingSince.Compare(b.WaitingSince)
		case SectionInProgress:
			if c := cmp.Compare(inProgressRank(a), inProgressRank(b)); c != 0 {
				return c
			}
			if a.State == EpicListQueued {
				return cmp.Compare(queuePosition(a), queuePosition(b))
			}
			if inProgressRank(a) >= 2 {
				return b.LastActivityAt.Compare(a.LastActivityAt)
			}
		case SectionReadyToQueue:
			return b.CreatedAt.Compare(a.CreatedAt)
		case SectionReadyToDesign:
			// Follow-ups cards come first, then the rest by name.
			if c := cmp.Compare(followUpsRank(a), followUpsRank(b)); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		case SectionFinished:
			return finishedAt(b).Compare(finishedAt(a))
		}
		return 0
	})
}
