package sessions

import (
	"cmp"
	"net/http"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/modelrules"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// SessionModelsRead serves the model rows of the session's own harness. It asks
// the cache for a background refresh and answers from the list it holds, so it
// never waits for a probe. A Queue agent id is an unknown session here, like in
// the write routes. It is a function, not a method, to keep Manager's method
// count under its limit.
func SessionModelsRead(mg *Manager, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// The settings page has no session: "harness" alone reads that harness's list with no current model.
	if h := q.Get("harness"); q.Get("session") == "" && (h == string(harness.Claude) || h == string(harness.Codex)) {
		cache := mg.chat.Models()
		cache.Refresh(h)
		wire.WriteJSON(w, modelsResponse(cache, h, "", ""))
		return
	}
	_, meta, res, err := mg.SessionDir(q.Get("session"))
	if res.Refused || err != nil {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		chat.WriteRefusal(w, res)
		return
	}
	if meta.Harness != string(harness.Claude) && meta.Harness != string(harness.Codex) {
		chat.WriteRefusal(w, Refusal(wire.ReasonPromptHarnessNotSupported, "this harness has no model list"))
		return
	}
	cache := mg.chat.Models()
	cache.Refresh(meta.Harness)
	info := mg.SessionInfo(meta)
	wire.WriteJSON(w, modelsResponse(cache, meta.Harness, info.Model, info.Effort))
}

// modelsResponse builds the reply from the cache's current list. Refreshing is
// read last, so a read that started a refresh says so.
func modelsResponse(cache *commandlist.ModelCache, harnessName, shown, effort string) wire.SessionModelsResponse {
	list, _ := cache.Models(harnessName)
	entries, _ := cache.Entries(harnessName)
	codex := harnessName == string(harness.Codex)

	out := wire.SessionModelsResponse{Models: []wire.SessionModelRow{}, Aliases: []wire.ModelAlias{}}
	seen := map[string]bool{}
	current := resolvedModelID(entries, shown)
	for _, m := range list {
		id := cmp.Or(m.ID, m.Value)
		if m.Hidden || m.Value == "default" || seen[id] {
			continue
		}
		seen[id] = true
		out.Models = append(out.Models, modelRow(entries, id, m.Efforts, m.DefaultEffort, effort, codex, id == current))
	}
	if current != "" && !seen[current] {
		out.Models = append(out.Models, modelRow(entries, current, nil, "", effort, codex, true))
	}

	if claude, ok := cache.Models(string(harness.Claude)); ok {
		claudeEntries, _ := cache.Entries(string(harness.Claude))
		for _, m := range claude {
			if m.Hidden || m.Value == "default" || m.ID == "" || m.Value == m.ID {
				continue
			}
			out.Aliases = append(out.Aliases, wire.ModelAlias{Alias: m.Value, ID: m.ID, Latest: latestID(claudeEntries, m.ID)})
		}
	}
	out.Refreshing = cache.Refreshing(harnessName)
	return out
}

func modelRow(entries []modelrules.Entry, id string, efforts []string, codexDefault, currentEffort string, codex, current bool) wire.SessionModelRow {
	row := wire.SessionModelRow{
		ID: id, Current: current, Efforts: append([]string{}, efforts...),
		Latest:       latestID(entries, id),
		SwitchEffort: modelrules.Effort(currentEffort, efforts, codex, codexDefault),
	}
	if family, v, ok := modelrules.ParseID(id); ok {
		row.Family = family
		row.Version = &wire.ModelVersion{Major: v.Major, Minor: v.Minor}
	}
	return row
}

// latestID is the resolved id of the latest model of id's family, or "".
func latestID(entries []modelrules.Entry, id string) string {
	latest, ok := modelrules.Latest(id, entries)
	if !ok {
		return ""
	}
	return resolvedModelID(entries, latest)
}
