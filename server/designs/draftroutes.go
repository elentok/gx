package designs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/designs/drafts"
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
	res, err := s.drafts.Save(req.ID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
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
