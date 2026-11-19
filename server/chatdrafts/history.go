package chatdrafts

import (
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire/ops"
)

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
