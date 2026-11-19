package chatdrafts

import (
	"errors"
	"net/http"
	"os"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// pickHistory serves POST /v1/agents/drafts/pick-history: it restores a Chat
// drafts entry's text, images and name onto the page. The page's project,
// folder, harness, model and effort stay.
func (c *ChatDrafts) pickHistory(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.DraftHistoryPickRequest](w, r)
	if !ok {
		return
	}
	p, err := c.history.Load(drafthistory.ChatDrafts, req.EntryID)
	if errors.Is(err, os.ErrNotExist) {
		wire.WriteJSON(w, ops.ChatDraftPickResult{Refused: true, Reason: ops.ReasonHistoryEntryGone, Notices: []string{}})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	notices := []string{}
	changedAny := false
	d, reason, err := c.store.Pick(req.DraftID, req.Revision, func(d *ops.ChatDraft) (bool, error) {
		page := drafthistory.Entry{Text: d.Brief, Name: d.Name}
		res, err := c.history.Restore(drafthistory.ChatDrafts, p, page, c.store.ImagesDir(d.ID), d.Images,
			func() error { return c.saveHistory(d) })
		if err != nil || !res.Changed {
			return false, err
		}
		d.Brief, d.Name, d.Images = p.Entry.Text, p.Entry.Name, res.Images
		if res.Missing > 0 {
			notices = append(notices, drafthistory.MissingNotice(res.Missing))
		}
		changedAny = true
		return true, nil
	})
	if err != nil {
		serverError(w, err)
		return
	}
	if reason != "" {
		wire.WriteJSON(w, ops.ChatDraftPickResult{Refused: true, Reason: reason, Notices: []string{}})
		return
	}
	if changedAny {
		c.publish()
	}
	wire.WriteJSON(w, ops.ChatDraftPickResult{Draft: d, Notices: notices})
}

// saveHistory adds d to the Chat drafts list with copies of its images. The
// caller holds the draft's lock, so the text cannot change before the save.
// The resolved Dir is not kept: a restore never changes the page's folder.
// A failed save is the caller's call: an explicit delete refuses and keeps the
// draft, Start logs it and goes on.
func (c *ChatDrafts) saveHistory(d *ops.ChatDraft) error {
	var paths []string
	for _, id := range d.Images {
		if src, ok := imagestore.Find(c.store.ImagesDir(d.ID), id); ok {
			paths = append(paths, src)
		}
	}
	_, err := c.history.Save(drafthistory.ChatDrafts, drafthistory.Entry{
		Text: d.Brief, Project: d.Project, Name: d.Name, Harness: d.Harness,
		Model: d.Model, Effort: d.Effort, Folder: d.Folder,
	}, paths)
	return err
}
