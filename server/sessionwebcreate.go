package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
)

// WebCreateSessionRequest is the body of POST /v1/sessions/create on the web
// mux. It has no ticket, harness or worktree-bookkeeping fields: the page may
// only start a plain chat session in a project's root or one of its worktrees.
type WebCreateSessionRequest struct {
	Project string `json:"project"`
	// Dir is the project root or one of its worktrees; Worktree replaces it.
	Dir      string `json:"dir,omitempty"`
	Worktree string `json:"worktree,omitempty"`
	Name     string `json:"name,omitempty"`
	// Prompt is typed at the session's first Idle.
	Prompt string `json:"prompt,omitempty"`
	// Tier is light, worker or strongest; empty means worker.
	Tier ralphloop.Tier `json:"tier,omitempty"`
}

// sessionCreateWeb is sessionCreateWrite for the web mux. A body with any
// other field is refused, not dropped.
func (s *Server) sessionCreateWeb(w http.ResponseWriter, r *http.Request) {
	var req WebCreateSessionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var res wire.SessionResult
	if err := dec.Decode(&req); err != nil {
		res = sessionRefusal(wire.ReasonSessionBadRequest, err.Error())
	} else {
		var err error
		if res, err = s.createWebSession(req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, res)
}

// checkProjectDir returns a refusal when project is not configured or a
// non-empty dir is neither its root nor one of its worktrees; nil means ok.
func (s *Server) checkProjectDir(project, dir string) (*wire.SessionResult, error) {
	list, err := s.listProjectWorktrees()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(list, func(p wire.ProjectWorktrees) bool { return p.Project == project })
	if i < 0 {
		res := sessionRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %q is not configured", project))
		return &res, nil
	}
	if dir != "" {
		clean := filepath.Clean(dir)
		known := filepath.Clean(list[i].Root) == clean || slices.ContainsFunc(list[i].Worktrees, func(wt string) bool { return filepath.Clean(wt) == clean })
		if !known {
			res := sessionRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("dir %s is not the root or a worktree of %s", dir, project))
			return &res, nil
		}
	}
	return nil, nil
}

func (s *Server) createWebSession(req WebCreateSessionRequest) (wire.SessionResult, error) {
	prompt, refusal := validatePromptText(req.Prompt, "the first prompt")
	if refusal != nil {
		return *refusal, nil
	}
	refusal, err := s.checkProjectDir(req.Project, req.Dir)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if refusal != nil {
		return *refusal, nil
	}
	return s.createSession(wire.CreateSessionRequest{
		Project:  req.Project,
		Dir:      req.Dir,
		Worktree: req.Worktree,
		Name:     req.Name,
		Prompt:   prompt,
		Tier:     req.Tier,
	}, sessionCtx{})
}
