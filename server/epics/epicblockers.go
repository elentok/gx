package epics

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// listStateOf is the Epics page state of e: the one computation behind its own
// card and behind every blocker entry that names it.
func (s *Epics) listStateOf(project string, e tickets.Epic) wire.EpicListState {
	root := queue.RootRef{Project: project, Epic: e.Name}
	queued := s.Queue.RootQueued(root)
	return epicListState(e, queued, queue.EpicQueueable(e, queued), EpicStateFor(root, e, s.Delivery(project), s.Runs.StartedAtOf))
}

// notMovingStates are the list states of an epic that will not progress by
// itself. Every other state is moving.
var notMovingStates = map[wire.EpicListState]bool{
	wire.EpicListReady: true, wire.EpicListDraft: true, wire.EpicListNoOpenTickets: true,
	wire.EpicListPaused: true, wire.EpicListMergeBlocked: true,
	wire.EpicListCIFailing: true, wire.EpicListPRTextFailed: true,
}

// blockerWalk is the project view a blocker's state and walk read from.
type blockerWalk struct {
	s       *Epics
	project string
	byName  map[string]tickets.Epic
}

func (w blockerWalk) hintOf(e tickets.Epic) wire.BlockerHint {
	return wire.BlockerHint{
		Address:    queue.RootRef{Project: w.project, Epic: e.Name}.String(),
		Name:       e.Name,
		State:      w.s.listStateOf(w.project, e),
		Deadlocked: StateOf(e) == StateCycle,
	}
}

// notMoving reports whether e is stuck, and the deepest stuck epic behind it.
// A Waiting epic is stuck only when one of its own blockers is, so the walk
// follows UnreleasedBlockers; visited ends it on a hand-edited epic cycle.
func (w blockerWalk) notMoving(e tickets.Epic, visited map[string]bool) (wire.BlockerHint, bool) {
	visited[e.Name] = true
	if StateOf(e) != StateWaiting {
		h := w.hintOf(e)
		return h, notMovingStates[h.State]
	}
	for _, name := range e.UnreleasedBlockers {
		next, ok := w.byName[name]
		if !ok || visited[name] {
			continue
		}
		if h, stuck := w.notMoving(next, visited); stuck {
			return h, true
		}
	}
	return wire.BlockerHint{}, false
}

// blockersOf is the list of epics e waits on: invalid stored refs first, then
// the not-moving blockers, then by name. A map epic has none.
func (s *Epics) blockersOf(project string, e tickets.Epic, byName map[string]tickets.Epic) []wire.EpicBlocker {
	if e.IsMap {
		return nil
	}
	walk := blockerWalk{s, project, byName}
	var invalid, stuck, moving []wire.EpicBlocker
	for _, ref := range e.InvalidBlockers {
		invalid = append(invalid, wire.EpicBlocker{Invalid: true, Ref: ref})
	}
	for _, name := range e.UnreleasedBlockers {
		b, ok := byName[name]
		if !ok {
			continue
		}
		entry := wire.EpicBlocker{Address: queue.RootRef{Project: project, Epic: name}.String(), Name: name}
		h, notMoving := walk.notMoving(b, map[string]bool{e.Name: true})
		own := walk.hintOf(b)
		entry.State, entry.Deadlocked = own.State, own.Deadlocked
		if !notMoving {
			moving = append(moving, entry)
			continue
		}
		if h.Name != name {
			entry.Hint = &h
		}
		stuck = append(stuck, entry)
	}
	byEntryName := func(a, b wire.EpicBlocker) int { return cmp.Compare(a.Name, b.Name) }
	slices.SortStableFunc(stuck, byEntryName)
	slices.SortStableFunc(moving, byEntryName)
	return slices.Concat(invalid, stuck, moving)
}

// applyBlockers sets the blocker list on row, and the Waiting words and flag.
func applyBlockers(row *wire.EpicListRow, e tickets.Epic, blockers []wire.EpicBlocker) {
	row.Blockers = blockers
	if e.IsMap || StateOf(e) != StateWaiting {
		return
	}
	row.WaitingOnEpic = true
	row.StatusWords = waitingWords(blockers)
}

// waitingWords is "Waiting on A", "... and 1 more", "... and 2 more", where A is
// entry 0's bare name.
func waitingWords(blockers []wire.EpicBlocker) string {
	name := ""
	if len(blockers) > 0 {
		name = blockers[0].Name
		if blockers[0].Invalid {
			name = blockers[0].Ref
		}
	}
	if more := len(blockers) - 1; more > 0 {
		return fmt.Sprintf("Waiting on %s and %d more", name, more)
	}
	return "Waiting on " + name
}

func epicsByName(epics []tickets.Epic) map[string]tickets.Epic {
	byName := make(map[string]tickets.Epic, len(epics))
	for _, e := range epics {
		byName[e.Name] = e
	}
	return byName
}
