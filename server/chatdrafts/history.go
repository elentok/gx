package chatdrafts

import (
	"net/http"

	"github.com/elentok/gx/server/drafthistory"
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
	out, err := drafthistory.PickDraft(c.history, drafthistory.DraftPick[ops.ChatDraft]{
		List: drafthistory.ChatDrafts,
		Pick: c.store.Pick,
		Restore: func(p drafthistory.Picked, d *ops.ChatDraft) (drafthistory.Restored, error) {
			page := drafthistory.Entry{Text: d.Brief, Name: d.Name}
			return c.history.Restore(drafthistory.ChatDrafts, p, page, c.store.ImagesDir(d.ID), d.Images,
				func() error { return c.saveHistory(d) })
		},
		Apply: func(d *ops.ChatDraft, e drafthistory.Entry, images []string) []string {
			d.Brief, d.Name, d.Images = e.Text, e.Name, images
			return nil
		},
	}, req)
	if err != nil {
		serverError(w, err)
		return
	}
	if out.Reason != "" {
		wire.WriteJSON(w, ops.ChatDraftPickResult{Refused: true, Reason: out.Reason, Notices: []string{}})
		return
	}
	if out.Changed {
		c.publish()
	}
	wire.WriteJSON(w, ops.ChatDraftPickResult{Draft: out.Draft, Notices: out.Notices})
}

// saveHistory adds d to the Chat drafts list with copies of its images. The
// caller holds the draft's lock, so the text cannot change before the save.
// The resolved Dir is not kept: a restore never changes the page's folder.
// A failed save is the caller's call: an explicit delete refuses and keeps the
// draft, Start logs it and goes on.
func (c *ChatDrafts) saveHistory(d *ops.ChatDraft) error {
	_, err := c.history.Save(drafthistory.ChatDrafts, drafthistory.Entry{
		Text: d.Brief, Project: d.Project, Name: d.Name, Harness: d.Harness,
		Model: d.Model, Effort: d.Effort, Folder: d.Folder,
	}, drafthistory.ImageFiles(c.store.ImagesDir(d.ID), d.Images))
	return err
}
