package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/settingsapi"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

var filesRootsKey = registry.Key("server", "web", "files", "roots")

// rootsMu serializes the read-modify-write of the extra-roots list.
var rootsMu sync.Mutex

func (h hostedFiles) listRoots(w http.ResponseWriter, _ *http.Request) {
	wire.WriteJSON(w, ops.HostedRootsResult{Roots: h.allRoots()})
}

// builtinRoots lists the roots the checker always allows.
func (h hostedFiles) builtinRoots() []string {
	c := h.checker()
	var out []string
	for _, p := range append(c.BuiltinRoots(), c.TempRoots()...) {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func (h hostedFiles) extraRoots() []string { return h.s.settings.Load().WebFilesRoots }

func (h hostedFiles) allRoots() []ops.HostedRoot {
	roots := []ops.HostedRoot{}
	for _, p := range h.builtinRoots() {
		roots = append(roots, ops.HostedRoot{Path: p, Source: ops.HostedRootBuiltIn})
	}
	for _, p := range h.extraRoots() {
		roots = append(roots, ops.HostedRoot{Path: p, Source: ops.HostedRootConfig})
	}
	return roots
}

func (h hostedFiles) addRoot(w http.ResponseWriter, r *http.Request) {
	path, ok := decodeRootPath(w, r)
	if !ok {
		return
	}
	if reason, msg := h.checkAddable(path); reason != "" {
		wire.WriteJSON(w, ops.HostedRootsResult{}.Refuse(reason, msg))
		return
	}
	h.writeRoots(w, func(cur []string) []string {
		if slices.Contains(cur, path) {
			return cur
		}
		return append(slices.Clone(cur), path)
	})
}

func (h hostedFiles) removeRoot(w http.ResponseWriter, r *http.Request) {
	path, ok := decodeRootPath(w, r)
	if !ok {
		return
	}
	if slices.Contains(h.extraRoots(), path) {
		h.writeRoots(w, func(cur []string) []string {
			return slices.DeleteFunc(slices.Clone(cur), func(p string) bool { return p == path })
		})
		return
	}
	reason, msg := ops.ReasonRootNotAdded, path+" is not a configured root"
	if slices.Contains(resolvedPaths(h.builtinRoots()), resolvedPath(path)) {
		reason, msg = ops.ReasonRootBuiltIn, path+" is a built-in root; built-in roots are always on"
	}
	wire.WriteJSON(w, ops.HostedRootsResult{}.Refuse(reason, msg))
}

// decodeRootPath reads the request path and cleans it. It answers the request
// itself when the body is bad.
func decodeRootPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req ops.HostedRootRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return "", false
	}
	if req.Path == "" {
		return "", true
	}
	return filepath.Clean(req.Path), true
}

func (h hostedFiles) checkAddable(path string) (reason, msg string) {
	if !filepath.IsAbs(path) {
		return ops.ReasonRootNotAbsolute, "give an absolute folder path"
	}
	resolved := resolvedPath(path)
	if resolved == string(filepath.Separator) {
		return ops.ReasonRootIsSlash, "the whole disk cannot be a root"
	}
	if home, err := config.DirsFrom(h.s.cfg.Env).Home(); err == nil && resolved == resolvedPath(home) {
		return ops.ReasonRootIsHome, "the home folder itself cannot be a root; add a subfolder of it"
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return ops.ReasonRootMissing, path + " does not exist"
	case !info.IsDir():
		return ops.ReasonRootNotFolder, path + " is not a folder"
	}
	return "", ""
}

// writeRoots saves the list edit f makes, reloads config so it is live, and
// answers with every root.
func (h hostedFiles) writeRoots(w http.ResponseWriter, f func(cur []string) []string) {
	rootsMu.Lock()
	defer rootsMu.Unlock()
	next := f(h.extraRoots())
	var change settingsapi.Change
	if len(next) == 0 {
		change = settingsapi.Change{Key: filesRootsKey, Remove: true}
	} else {
		val, _ := json.Marshal(next)
		change = settingsapi.Change{Key: filesRootsKey, Value: val}
	}
	if _, err := h.s.settingsWriter.Write(settingsapi.Target{}, []settingsapi.Change{change}); err != nil {
		wire.WriteJSON(w, ops.HostedRootsResult{}.Refuse(ops.ReasonRootWriteFailed, err.Error()))
		return
	}
	if rr := h.s.settings.Reload(settings.TriggerWeb); rr.Refused {
		wire.WriteJSON(w, ops.HostedRootsResult{}.Refuse(ops.ReasonRootWriteFailed, "saved, but the server could not apply it: "+rr.Message))
		return
	}
	wire.WriteJSON(w, ops.HostedRootsResult{Roots: h.allRoots()})
}

func resolvedPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func resolvedPaths(ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = resolvedPath(p)
	}
	return out
}
