package server

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/harness"
)

// SessionCommandEntry is one row of the command menu. Name has no leading
// "/" or "$".
type SessionCommandEntry struct {
	Name        string   `json:"name"`
	ArgHint     string   `json:"arg_hint"`
	Description string   `json:"description"`
	Source      string   `json:"source"` // built-in, skill or mcp
	Aliases     []string `json:"aliases"`
}

// SessionCommandsResponse is the menu for one session or project dir. Degraded
// means the harness did not answer and Entries are only the built-ins.
type SessionCommandsResponse struct {
	Entries  []SessionCommandEntry `json:"entries"`
	Degraded bool                  `json:"degraded"`
}

// sessionCommandsRead serves the command menu of a session (session=) or of a
// First prompt in a project dir (project= and dir=), for kind "/" (default)
// or "$". The read starts the harness in the dir, which runs the person's
// hooks, so a project dir is checked like a new session's.
func (s *Server) sessionCommandsRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind == "" {
		kind = "/"
	}
	id, project, dir := q.Get("session"), q.Get("project"), q.Get("dir")
	bySession := id != "" && project == "" && dir == ""
	byProject := id == "" && project != "" && dir != ""
	if (kind != "/" && kind != "$") || bySession == byProject {
		writeCommandsRefusal(w, sessionRefusal(ReasonSessionBadRequest, "want kind / or $, and either session or project with dir"))
		return
	}

	// A First prompt has no harness field: it starts Claude.
	name := string(harness.Claude)
	if bySession {
		_, meta, res, err := s.sessionDir(id)
		if res.Refused || err != nil {
			writeCommandsFailure(w, res, err)
			return
		}
		var ok bool
		if name, ok = s.commandHarness(meta.Harness); !ok {
			writeCommandsRefusal(w, sessionRefusal(ReasonPromptHarnessNotSupported, "this harness has no command list"))
			return
		}
		dir = meta.Dir
	} else {
		refusal, err := s.checkProjectDir(project, dir)
		if refusal != nil {
			writeCommandsFailure(w, *refusal, nil)
			return
		}
		if err != nil {
			writeCommandsFailure(w, SessionResult{}, err)
			return
		}
	}

	writeJSON(w, s.commandList(r.Context(), name, kind, dir))
}

// commandHarness maps a session's harness to the command-list harness: Codex,
// or Claude for the chat harness, which speaks the Claude format.
func (s *Server) commandHarness(name string) (string, bool) {
	switch {
	case name == string(harness.Codex):
		return string(harness.Codex), true
	case s.settings.Load().hasChatReader(name):
		return string(harness.Claude), true
	}
	return "", false
}

// commandList is the menu for harnessName in dir. Claude has only "/", and
// Codex's "/" is its built-ins, so neither starts a probe for the other kind.
func (s *Server) commandList(ctx context.Context, harnessName, kind, dir string) SessionCommandsResponse {
	var res commandlist.Result
	switch {
	case harnessName == string(harness.Claude) && kind == "/":
		res = s.commandLists.Get(ctx, commandlist.Key{Harness: harnessName, Dir: dir, Kind: kind},
			func(ctx context.Context, dir string) ([]commandlist.Entry, error) {
				return commandlist.ProbeClaude(ctx, dir, s.cfg.Env)
			})
	case harnessName == string(harness.Codex) && kind == "$":
		res = s.commandLists.Get(ctx, commandlist.Key{Harness: harnessName, Dir: dir, Kind: kind},
			func(ctx context.Context, dir string) ([]commandlist.Entry, error) {
				return commandlist.ProbeCodexSkills(ctx, dir, s.cfg.Env)
			})
	case harnessName == string(harness.Codex):
		res.Entries = commandlist.BuiltinsFor(harnessName, runtime.GOOS).Entries
	}
	out := SessionCommandsResponse{Entries: make([]SessionCommandEntry, 0, len(res.Entries)), Degraded: res.Degraded}
	for _, e := range res.Entries {
		out.Entries = append(out.Entries, SessionCommandEntry{
			Name: e.Name, ArgHint: e.ArgHint, Description: e.Description,
			Source: string(e.Source), Aliases: append([]string{}, e.Aliases...),
		})
	}
	return out
}

// writeCommandsFailure answers a refusal, or an internal error when err is set.
func writeCommandsFailure(w http.ResponseWriter, res SessionResult, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeCommandsRefusal(w, res)
}

// writeCommandsRefusal answers with the refusal as JSON under a 4xx status, so
// a caller can read its reason.
func writeCommandsRefusal(w http.ResponseWriter, res SessionResult) {
	status := http.StatusBadRequest
	if res.Reason == ReasonSessionUnknown {
		status = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}
