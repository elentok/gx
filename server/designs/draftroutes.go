package designs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

func (s *Designs) draftList(w http.ResponseWriter, _ *http.Request) {
	items, err := s.drafts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, items)
}

func (s *Designs) draftRead(w http.ResponseWriter, r *http.Request) {
	d, err := s.drafts.Get(r.URL.Query().Get("id"))
	switch {
	case errors.Is(err, drafts.ErrNotFound):
		wire.WriteJSON(w, s.withStarted(ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, r.URL.Query().Get("id")))
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		wire.WriteJSON(w, ops.DesignDraftSaveResult{Draft: d})
	}
}

func (s *Designs) draftSave(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var res ops.DesignDraftSaveResult
	var err error
	if req.ID == "" {
		// The form's first save of a new draft has no id yet.
		var d *ops.DesignDraft
		if d, err = s.drafts.Create("", "", req.Idea); err == nil {
			res = ops.DesignDraftSaveResult{Draft: d}
			if refused := s.joinNewDraft(d.ID, req.Workspace); refused {
				res = ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftUnknownWorkspace}
			} else {
				var removed []string
				removed, err = s.drafts.SweepEmpty(d.ID)
				s.dropDrafts(removed...)
				if len(removed) == 0 { // a sweep that removed drafts already announced the change
					publishDrafts(s.events)
				}
			}
		}
	} else {
		res, err = s.drafts.Save(req.ID, req)
		res = s.withStarted(res, req.ID)
		// The label, not the text: typing past the fifth word changes no row.
		if err == nil && res.LabelChanged {
			publishDrafts(s.events)
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// draftCreate serves POST /v1/designs/drafts/create: a draft with a project,
// slug and idea in one call, joining the workspace of FromSession. Unlike the
// form's first save it makes no empty draft, so it runs no SweepEmpty. A plain
// function, not a method: Designs sits at its method-count limit.
func draftCreate(s *Designs, w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if chatdrafts.HookArmed(s.forkHookDir, chatdrafts.HookSaveFail) {
		http.Error(w, chatdrafts.ErrHookSaveFail.Error(), http.StatusInternalServerError)
		return
	}
	refuse := func(reason string) {
		wire.WriteJSON(w, ops.DesignDraftSaveResult{Refused: true, Reason: reason})
	}
	switch {
	case len(req.Idea) > wire.MaxSessionPromptBytes:
		refuse(wire.ReasonPromptTextTooLong)
		return
	case strings.TrimSpace(req.Idea) == "":
		refuse("idea must not be empty")
		return
	case strings.TrimSpace(req.Project) == "":
		refuse("project must not be blank")
		return
	case !drafts.SlugRe.MatchString(req.Slug):
		refuse(fmt.Sprintf("%q is not a slug (want lowercase words joined by -)", req.Slug))
		return
	}
	d, err := s.drafts.Create(req.Project, req.Slug, req.Idea)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.workspaces != nil && req.FromSession != "" {
		// An unknown session resolves to Default, which joins nothing.
		s.joinNewDraft(d.ID, s.workspaces.WorkspaceOfSession(req.FromSession))
	}
	publishDrafts(s.events)
	wire.WriteJSON(w, ops.DesignDraftSaveResult{Draft: d})
}

// joinNewDraft puts a just-created draft in the named workspace. An empty or
// Default name, or no workspace store, joins nothing. An unknown name deletes
// the draft again and reports refused, so no draft is left behind.
func (s *Designs) joinNewDraft(id, workspace string) (refused bool) {
	if s.workspaces == nil || workspace == "" || workspace == workspaces.DefaultName {
		return false
	}
	if err := s.workspaces.SetMember(drafts.MemberKey(id), workspace); err != nil {
		if delErr := s.drafts.Delete(id); delErr != nil {
			s.log.Warn("delete draft of unknown workspace", "draft", id, "err", delErr)
		}
		return true
	}
	return false
}

// withStarted adds the Design a gone draft was started as, when a Start record exists.
func (s *Designs) withStarted(res ops.DesignDraftSaveResult, id string) ops.DesignDraftSaveResult {
	if res.Refused && res.Reason == ops.ReasonDraftGone {
		res.Started = s.drafts.Started(id)
	}
	return res
}

// draftTarget serves POST /v1/designs/drafts/target: the revision-free write
// of a draft's slug and project. A changed project is also the last project used.
func (s *Designs) draftTarget(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, getErr := s.drafts.Get(req.ID)
	res, err := s.drafts.SetTarget(req.ID, req.Project, req.Slug, req.KeepEditedAt)
	if err == nil && res.Draft != nil && req.Project != nil && (getErr != nil || before.Project != *req.Project) {
		err = s.drafts.SetLastProject(*req.Project)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if getErr == nil && res.Draft != nil && (before.Slug != res.Draft.Slug || before.Project != res.Draft.Project) {
		publishDrafts(s.events)
	}
	wire.WriteJSON(w, res)
}

func (s *Designs) lastProjectRead(w http.ResponseWriter, _ *http.Request) {
	p, err := s.drafts.LastProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, ops.DesignLastProject{Project: p})
}

func (s *Designs) lastProjectSet(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignLastProject
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.drafts.SetLastProject(req.Project); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, req)
}

// draftImageUpload serves POST /v1/designs/drafts/images?id=<draft>: the body
// is one image's raw bytes, capped like a session image upload.
func (s *Designs) draftImageUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imagestore.MaxBytes+64<<10))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		wire.WriteJSON(w, imagestore.TooLarge())
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.drafts.AddImage(r.URL.Query().Get("id"), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// draftImageRemove serves POST /v1/designs/drafts/images/delete.
func (s *Designs) draftImageRemove(w http.ResponseWriter, r *http.Request) {
	var req wire.DraftImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.drafts.RemoveImage(req.ID, req.Image)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// draftImageServe serves GET /v1/designs/drafts/images?id=<draft>&image=<image>.
// Get validates the draft id before it names a folder.
func (s *Designs) draftImageServe(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if _, err := s.drafts.Get(id); err != nil {
		http.NotFound(w, r)
		return
	}
	imagestore.Serve(w, r, s.drafts.ImagesDir(id), r.URL.Query().Get("image"))
}

// mapImages serves the images/ folder of the map epic named by
// ?epic=project:epic. With serve false it is GET /v1/designs/images: the ids
// and file names, where anything unusable lists nothing. With serve true it is
// GET /v1/designs/images/read?...&id=<image>.
func (s *Designs) mapImages(w http.ResponseWriter, r *http.Request, serve bool) {
	folder, ok := "", false
	project, epic, found := strings.Cut(r.URL.Query().Get("epic"), ":")
	if found && project != "" && epic != "" && !strings.ContainsAny(epic, `/\`) && epic != ".." {
		if dir, reason, _ := s.mapEpicDir(project, epic); reason == "" {
			folder, ok = filepath.Join(dir, epic, "images"), true
		}
	}
	switch {
	case serve && !ok:
		http.NotFound(w, r)
	case serve:
		imagestore.Serve(w, r, folder, r.URL.Query().Get("id"))
	case ok:
		wire.WriteJSON(w, imagestore.List(folder))
	default:
		wire.WriteJSON(w, []wire.SessionImageResult{})
	}
}

// draftRemoveIfEmpty serves POST /v1/designs/drafts/remove-if-empty.
func (s *Designs) draftRemoveIfEmpty(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftLeaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.drafts.DeleteIfEmpty(req.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if res.Deleted {
		s.dropDrafts(req.ID)
	}
	wire.WriteJSON(w, res)
}

// draftLease serves POST /v1/designs/drafts/lease: the open page's ping.
func (s *Designs) draftLease(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftLeaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.drafts.Lease(req.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (s *Designs) draftDelete(w http.ResponseWriter, r *http.Request) {
	var req ops.DesignDraftDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.drafts.DeleteSaving(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.dropDrafts(req.ID)
	wire.WriteJSON(w, ops.DesignDraftSaveResult{Deleted: true})
}
