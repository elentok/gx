package drafthistory

import (
	"errors"
	"os"

	"github.com/elentok/gx/server/wire/ops"
)

// DraftPick is what a draft page kind (Chat drafts, Design drafts) supplies to
// PickDraft: how to pick under the draft's lock, how to restore, and how the
// entry lands on the draft.
type DraftPick[D any] struct {
	List List
	// Pick is the draft store's Pick: it locks the draft and runs apply.
	Pick func(id string, revision int, apply func(d *D) (bool, error)) (*D, string, error)
	// Restore swaps the draft's images for the entry's and saves the page to the
	// history first. The draft's lock is held.
	Restore func(p Picked, d *D) (Restored, error)
	// Apply writes the entry's fields and the restored image ids onto d and
	// returns the notices it adds.
	Apply func(d *D, e Entry, images []string) []string
}

// PickOutcome is the result of PickDraft. Reason is set when the pick was refused.
type PickOutcome[D any] struct {
	Draft   *D
	Reason  string
	Notices []string
	Changed bool
}

// PickDraft restores history entry req.EntryID onto the draft req.DraftID. A
// gone entry is refused with ops.ReasonHistoryEntryGone and writes nothing.
func PickDraft[D any](h *Store, k DraftPick[D], req ops.DraftHistoryPickRequest) (PickOutcome[D], error) {
	p, err := h.Load(k.List, req.EntryID)
	if errors.Is(err, os.ErrNotExist) {
		return PickOutcome[D]{Reason: ops.ReasonHistoryEntryGone, Notices: []string{}}, nil
	}
	if err != nil {
		return PickOutcome[D]{}, err
	}
	out := PickOutcome[D]{Notices: []string{}}
	d, reason, err := k.Pick(req.DraftID, req.Revision, func(d *D) (bool, error) {
		res, err := k.Restore(p, d)
		if err != nil || !res.Changed {
			return false, err
		}
		out.Notices = append(out.Notices, k.Apply(d, p.Entry, res.Images)...)
		if res.Missing > 0 {
			out.Notices = append(out.Notices, MissingNotice(res.Missing))
		}
		out.Changed = true
		return true, nil
	})
	if err != nil {
		return PickOutcome[D]{}, err
	}
	if reason != "" {
		return PickOutcome[D]{Reason: reason, Notices: []string{}}, nil
	}
	out.Draft = d
	return out, nil
}
