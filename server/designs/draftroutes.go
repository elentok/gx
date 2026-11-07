package designs

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
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
		wire.WriteJSON(w, wire.DesignDraftSaveResult{Refused: true, Reason: wire.ReasonDraftGone})
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		wire.WriteJSON(w, wire.DesignDraftSaveResult{Draft: d})
	}
}

func (s *Designs) draftSave(w http.ResponseWriter, r *http.Request) {
	var req wire.DesignDraftSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var res wire.DesignDraftSaveResult
	var err error
	if req.ID == "" {
		// The form's first save of a new draft has no id yet.
		var d *wire.DesignDraft
		if d, err = s.drafts.Create(req.Project, req.Slug, req.Idea, req.Name); err == nil {
			res = wire.DesignDraftSaveResult{Draft: d, Deleted: d == nil}
		}
	} else {
		res, err = s.drafts.Save(req.ID, req)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (s *Designs) lastProjectRead(w http.ResponseWriter, _ *http.Request) {
	p, err := s.drafts.LastProject()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, wire.DesignLastProject{Project: p})
}

func (s *Designs) lastProjectSet(w http.ResponseWriter, r *http.Request) {
	var req wire.DesignLastProject
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
	var req wire.SessionImageRequest
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

// mapImagesDir is the images/ folder of the map epic named by ?epic=project:epic.
func (s *Designs) mapImagesDir(r *http.Request) (string, bool) {
	project, epic, ok := strings.Cut(r.URL.Query().Get("epic"), ":")
	if !ok || project == "" || epic == "" || strings.ContainsAny(epic, `/\`) || epic == ".." {
		return "", false
	}
	dir, reason, _ := s.mapEpicDir(project, epic)
	if reason != "" {
		return "", false
	}
	return filepath.Join(dir, epic, "images"), true
}

// mapImageList serves GET /v1/designs/images?epic=project:epic: the ids and
// file names in the map's images/ folder. Anything unusable lists nothing.
func (s *Designs) mapImageList(w http.ResponseWriter, r *http.Request) {
	list := []wire.SessionImageResult{}
	if folder, ok := s.mapImagesDir(r); ok {
		list = imagestore.List(folder)
	}
	wire.WriteJSON(w, list)
}

// mapImageServe serves GET /v1/designs/images/read?epic=project:epic&id=<image>.
func (s *Designs) mapImageServe(w http.ResponseWriter, r *http.Request) {
	folder, ok := s.mapImagesDir(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	imagestore.Serve(w, r, folder, r.URL.Query().Get("id"))
}

func (s *Designs) draftDelete(w http.ResponseWriter, r *http.Request) {
	var req wire.DesignDraftDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.drafts.Delete(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, wire.DesignDraftSaveResult{Deleted: true})
}
