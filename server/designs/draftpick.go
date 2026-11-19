package designs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// slugTaken: the project has an epic of that name or a Design whose branch
// would clash with it (the checks openNewDesign makes). An unknown project
// takes nothing.
func (s *Designs) slugTaken(project, slug string) bool {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, slug)); err == nil {
		return true
	}
	return designClash(dir, project, slug) != ""
}

// draftPickHistory serves POST /v1/designs/drafts/pick-history: it restores a
// Design drafts entry's text, images and slug onto the page. The page's project
// stays, and a slug taken in it is cleared with a notice.
func (s *Designs) draftPickHistory(w http.ResponseWriter, r *http.Request) {
	var req ops.DraftHistoryPickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hist := s.drafts.History()
	p, err := hist.Load(drafthistory.DesignDrafts, req.EntryID)
	if errors.Is(err, os.ErrNotExist) {
		wire.WriteJSON(w, ops.DesignDraftPickResult{Refused: true, Reason: ops.ReasonHistoryEntryGone, Notices: []string{}})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	notices := []string{}
	changedAny := false
	d, reason, err := s.drafts.Pick(req.DraftID, req.Revision, func(d *ops.DesignDraft) (bool, error) {
		page := drafthistory.Entry{Text: d.Idea, Slug: d.Slug}
		res, err := hist.Restore(drafthistory.DesignDrafts, p, page, s.drafts.ImagesDir(d.ID), d.Images,
			func() error { return s.drafts.SaveToHistoryLocked(d.ID) })
		if err != nil || !res.Changed {
			return false, err
		}
		d.Idea, d.Images, d.Slug = p.Entry.Text, res.Images, p.Entry.Slug
		if d.Slug != "" && s.slugTaken(d.Project, d.Slug) {
			notices = append(notices, fmt.Sprintf("slug %q is already taken in %s, so it was cleared", d.Slug, d.Project))
			d.Slug = ""
		}
		if res.Missing > 0 {
			notices = append(notices, drafthistory.MissingNotice(res.Missing))
		}
		changedAny = true
		return true, nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reason != "" {
		wire.WriteJSON(w, ops.DesignDraftPickResult{Refused: true, Reason: reason, Notices: []string{}})
		return
	}
	if changedAny {
		publishDrafts(s.events)
	}
	wire.WriteJSON(w, ops.DesignDraftPickResult{Draft: d, Notices: notices})
}
