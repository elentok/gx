package server

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/harness"
)

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
		writeCommandsRefusal(w, sessionRefusal(wire.ReasonSessionBadRequest, "want kind / or $, and either session or project with dir"))
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
			writeCommandsRefusal(w, sessionRefusal(wire.ReasonPromptHarnessNotSupported, "this harness has no command list"))
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
			writeCommandsFailure(w, wire.SessionResult{}, err)
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
	case s.settings.Load().HasChatReader(name):
		return string(harness.Claude), true
	}
	return "", false
}

// commandList is the menu for harnessName in dir. Claude has only "/", and
// Codex's "/" is its built-ins, so neither starts a probe for the other kind.
func (s *Server) commandList(ctx context.Context, harnessName, kind, dir string) wire.SessionCommandsResponse {
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
	out := wire.SessionCommandsResponse{Entries: make([]wire.SessionCommandEntry, 0, len(res.Entries)), Degraded: res.Degraded}
	for _, e := range res.Entries {
		out.Entries = append(out.Entries, wire.SessionCommandEntry{
			Name: e.Name, ArgHint: e.ArgHint, Description: e.Description,
			Source: string(e.Source), Aliases: append([]string{}, e.Aliases...),
		})
	}
	return out
}

// writeCommandsFailure answers a refusal, or an internal error when err is set.
func writeCommandsFailure(w http.ResponseWriter, res wire.SessionResult, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeCommandsRefusal(w, res)
}

// writeCommandsRefusal answers with the refusal as JSON under a 4xx status, so
// a caller can read its reason.
func writeCommandsRefusal(w http.ResponseWriter, res wire.SessionResult) {
	status := http.StatusBadRequest
	if res.Reason == wire.ReasonSessionUnknown {
		status = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}
