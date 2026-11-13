package settingsapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// GlobalTarget is the `target` value that names the global config file.
const GlobalTarget = "global"

// Routes lists the settings page endpoints. pending reports the startup-only
// keys changed since the server started. reload applies a saved config.json
// to the running server (the web trigger). The getter is called per request.
func Routes(w func() *Writer, pending func() []string, reload func() wire.ReloadResult) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/settings", Web: wire.WebRead, Handler: func(rw http.ResponseWriter, r *http.Request) {
			var t Target
			switch target := r.URL.Query().Get("target"); target {
			case GlobalTarget:
			case "":
				http.Error(rw, "target is required: global or a project name", http.StatusBadRequest)
				return
			default:
				t.Project = target
				if _, err := w().ProjectDir(target); err != nil {
					http.Error(rw, err.Error(), http.StatusNotFound)
					return
				}
			}
			resp, err := w().Read(t, pending())
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(rw, resp)
		}},
		{Pattern: "POST /v1/settings", Web: wire.WebWrite, Handler: func(rw http.ResponseWriter, r *http.Request) {
			var req ops.SettingsWriteRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(rw, "bad request: "+err.Error(), http.StatusBadRequest)
				return
			}
			var t Target
			t.Project = req.Project
			if t.Project != "" {
				if _, err := w().ProjectDir(t.Project); err != nil {
					http.Error(rw, err.Error(), http.StatusNotFound)
					return
				}
			}
			wire.WriteJSON(rw, write(w(), t, req.Changes, pending, reload))
		}},
	}
}

// write saves the changes, then reloads config.json once for a global write.
func write(w *Writer, t Target, in []ops.SettingChange, pending func() []string, reload func() wire.ReloadResult) ops.SettingsWriteResult {
	// A hook may reload inside the write; reuse its result rather than reload twice.
	var reloaded *wire.ReloadResult
	t.Reload = func() wire.ReloadResult {
		rr := reload()
		reloaded = &rr
		return rr
	}
	changes, err := toChanges(w.Registry, in)
	var res Result
	if err == nil {
		res, err = w.Write(t, changes)
	}
	if err != nil {
		if ref, ok := errors.AsType[*Refusal](err); ok {
			keys := make([]string, len(ref.Keys))
			for i, k := range ref.Keys {
				keys[i] = k.String()
			}
			return ops.SettingsWriteResult{Refused: true, Keys: keys, Reason: ref.Reason}
		}
		return ops.SettingsWriteResult{Refused: true, Reason: err.Error()}
	}
	out := ops.SettingsWriteResult{Values: make(map[string]json.RawMessage, len(res.Values)), Notices: res.Notices}
	for k, v := range res.Values {
		out.Values[k] = v
	}
	if t.Project != "" {
		return out
	}
	if reloaded == nil {
		t.Reload()
	}
	if rr := *reloaded; rr.Refused {
		out.Notices = append(out.Notices, "Saved, but the server could not apply it: "+rr.Message)
		return out
	}
	for _, k := range pending() {
		if _, saved := res.Values[k]; saved {
			out.RestartNeeded = append(out.RestartNeeded, k)
		}
	}
	return out
}

func toChanges(reg *registry.Registry, in []ops.SettingChange) ([]Change, error) {
	byKey := map[string]registry.KeyPath{}
	for _, e := range reg.All() {
		byKey[e.Key.String()] = e.Key
	}
	changes := make([]Change, len(in))
	for i, c := range in {
		key, ok := byKey[c.Key]
		if !ok {
			return nil, &Refusal{Keys: []registry.KeyPath{registry.Key(c.Key)}, Reason: "Unknown setting."}
		}
		changes[i] = Change{Key: key, Value: c.Value, Remove: c.Remove}
	}
	return changes, nil
}
