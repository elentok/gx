// Package tips serves the tips page's list: advice for the person running gx,
// filtered to the server machine's OS.
package tips

import (
	"encoding/json"
	"net/http"
	"runtime"
	"slices"

	"github.com/elentok/gx/server/wire"
)

// spotlight is the only tip. Later tips are one more entry in list.
var spotlight = wire.Tip{
	ID:       "spotlight",
	Title:    "Keep worktrees out of Spotlight",
	Platform: "darwin",
	Body: "Spotlight indexes every worktree, which burns CPU and disk. To skip them:\n" +
		"1. Open System Settings, then Spotlight.\n" +
		"2. Open Search Privacy.\n" +
		"3. Add each folder below.",
}

// Deps is what Tips reads from the Server.
type Deps struct {
	// Projects lists the registered project names.
	Projects func() []string
	// WorktreeDir is the folder holding a project's worktrees; empty skips it.
	WorktreeDir func(project string) string
	// GOOS is the server machine's OS; empty means the real one.
	GOOS string
	// Extra is appended to the built-in list. Tests use it to prove the list
	// is the only source.
	Extra []wire.Tip
}

// Tips serves the tips list.
type Tips struct{ d Deps }

// New builds Tips.
func New(d Deps) *Tips {
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	return &Tips{d: d}
}

// Routes lists the tips endpoint. The getter is called per request, so a
// route can be listed before its Tips is built.
func Routes(get func() *Tips) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/tips", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().read(w, r) }},
	}
}

func (t *Tips) read(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(t.List())
}

// List is the tips for this OS with their folders filled in. It is never nil.
func (t *Tips) List() []wire.Tip {
	out := []wire.Tip{}
	for _, tip := range append([]wire.Tip{spotlight}, t.d.Extra...) {
		if tip.Platform != t.d.GOOS {
			continue
		}
		if tip.ID == spotlight.ID {
			tip.Folders = t.worktreeFolders()
		}
		out = append(out, tip)
	}
	return out
}

func (t *Tips) worktreeFolders() []string {
	folders := []string{}
	for _, p := range t.d.Projects() {
		if dir := t.d.WorktreeDir(p); dir != "" && !slices.Contains(folders, dir) {
			folders = append(folders, dir)
		}
	}
	return folders
}
