package epics

import (
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// prInfo is e's PR for the wire, nil without one. live is the watcher's view.
func (s *Epics) prInfo(root queue.RootRef, e tickets.Epic) (info *wire.PRInfo, live *prstate.Live) {
	if e.PRNumber == 0 {
		return nil, nil
	}
	if s.PRView != nil {
		if l, ok := s.PRView(root); ok {
			live = &l
		}
	}
	i := prstate.Info(e.PRNumber, e.PRURL, !e.MergedAt.IsZero(), live)
	return &i, live
}

// prActions are the PR buttons of an unmerged loop epic in state. Create PR is
// the main action of the pr delivery's Awaiting PR; in a merge project it sits
// beside Merge on Awaiting merge (a merge block is the merge's own to fix). A one-off, an epic with a PR or a finished one never has it.
func prActions(root queue.RootRef, e tickets.Epic, state wire.EpicListState, delivery string, info *wire.PRInfo) []wire.CardAction {
	if !root.Mergeable() || !e.MergedAt.IsZero() {
		return nil
	}
	target := root.String()
	var out []wire.CardAction
	switch {
	case info == nil && (state == wire.EpicListAwaitingPR || (delivery != prstate.DeliveryPR && state == wire.EpicListAwaitingMerge)):
		out = append(out, wire.CardAction{Kind: wire.CardActionCreatePR, Label: "Create PR", Target: target})
	case info != nil:
		if state == wire.EpicListCIFailing || state == wire.EpicListPRTextFailed {
			out = append(out, wire.CardAction{Kind: wire.CardActionTryAgain, Label: "Try again", Target: target})
		}
		out = append(out, wire.CardAction{Kind: wire.CardActionMarkMerged, Label: "Mark as merged", Target: target})
		if info.URL != "" {
			out = append(out, wire.CardAction{Kind: wire.CardActionOpenPR, Label: "Open on GitHub", Target: target, Link: info.URL})
		}
	}
	return out
}

// ciFixWords are e's status words for state, counting ci-fix attempts from the
// newest Try again against the configured cap.
func (s *Epics) ciFixWords(state wire.EpicListState, info *wire.PRInfo, live *prstate.Live, e tickets.Epic) string {
	since, perCheck := time.Time{}, config.DefaultPRConfig().CIFixAttemptsPerCheck
	if s.PRCIFix != nil {
		since, perCheck = s.PRCIFix(e)
	}
	return prstate.Words(state, info, live, prstate.CIFixAttempt(e, since), perCheck)
}

// decoratePR adds the PR fields to a loop epic row, after its needs-you part is set.
func (s *Epics) decoratePR(row *wire.EpicListRow, root queue.RootRef, e tickets.Epic) {
	var live *prstate.Live
	row.PR, live = s.prInfo(root, e)
	if s.PRReason != nil {
		row.PRReason = s.PRReason(root)
	}
	if w := s.ciFixWords(row.State, row.PR, live, e); w != "" {
		row.StatusWords = w
	}
	more := prActions(root, e, row.State, s.Delivery(root.Project), row.PR)
	if len(more) == 0 {
		return
	}
	// Try again is the main action of a PR pause; Open epic follows it.
	if more[0].Kind == wire.CardActionTryAgain {
		row.Actions = append(more[:1:1], append(row.Actions, more[1:]...)...)
		return
	}
	row.Actions = append(row.Actions, more...)
}

// decoratePRDetail adds the PR fields to a loop epic's detail.
func (s *Epics) decoratePRDetail(d *wire.EpicDetail, root queue.RootRef, e tickets.Epic) {
	state := wire.EpicListState(d.State)
	if d.State == wire.EpicStatePaused && prstate.Blocked(e) {
		state = wire.EpicListState(e.MergeBlockKind)
	}
	var live *prstate.Live
	d.PR, live = s.prInfo(root, e)
	if s.PRReason != nil {
		d.PRReason = s.PRReason(root)
	}
	d.StatusWords = s.ciFixWords(state, d.PR, live, e)
	d.PRActions = prActions(root, e, state, s.Delivery(root.Project), d.PR)
	if prstate.Blocked(e) {
		d.PRNotice = e.MergeBlockReason
	}
	if s.PRActivity != nil {
		d.PRActivity = s.PRActivity(root, e)
	}
}
