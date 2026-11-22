package designs

import (
	"encoding/json"
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
	out, err := drafthistory.PickDraft(s.history, drafthistory.DraftPick[ops.DesignDraft]{
		List:    drafthistory.DesignDrafts,
		Pick:    s.drafts.Pick,
		Restore: s.drafts.RestoreHistory,
		Apply: func(d *ops.DesignDraft, e drafthistory.Entry, images []string) []string {
			d.Idea, d.Images, d.Slug = e.Text, images, e.Slug
			if d.Slug == "" || !s.slugTaken(d.Project, d.Slug) {
				return nil
			}
			notice := fmt.Sprintf("slug %q is already taken in %s, so it was cleared", d.Slug, d.Project)
			d.Slug = ""
			return []string{notice}
		},
	}, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out.Reason != "" {
		wire.WriteJSON(w, ops.DesignDraftPickResult{Refused: true, Reason: out.Reason, Notices: []string{}})
		return
	}
	if out.Changed {
		publishDrafts(s.events)
	}
	wire.WriteJSON(w, ops.DesignDraftPickResult{Draft: out.Draft, Notices: out.Notices})
}
