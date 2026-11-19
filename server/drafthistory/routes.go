package drafthistory

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// Routes serves the history list to the web UI. It is web-only: no Handler, so
// no CLI verb is required.
func Routes(get func() *Store) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/draft-history/list", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { list(get(), w, r) }},
	}
}

func list(s *Store, w http.ResponseWriter, r *http.Request) {
	var req ops.DraftHistoryListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	l := List(req.List)
	switch l {
	case ChatDrafts, DesignDrafts, Sent:
	default:
		http.Error(w, "unknown draft history list "+req.List, http.StatusBadRequest)
		return
	}
	items, err := s.List(l)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := ops.DraftHistoryListResult{List: req.List, Max: MaxEntries, Entries: make([]ops.DraftHistoryEntry, len(items))}
	for i, it := range items {
		out.Entries[i] = ops.DraftHistoryEntry{
			ID: it.ID, SavedAt: it.SavedAt, Text: it.Text, ImageCount: it.ImageCount,
			ImagesDropped: it.ImagesDropped, Project: it.Project, Slug: it.Slug, Name: it.Name,
			Harness: it.Harness, Model: it.Model, Effort: it.Effort, Folder: it.Folder, Composer: string(it.Composer),
		}
	}
	wire.WriteJSON(w, out)
}
