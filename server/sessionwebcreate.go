package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

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
	var res SessionResult
	if err := dec.Decode(&req); err != nil {
		res = sessionRefusal(ReasonSessionBadRequest, err.Error())
	} else {
		var err error
		if res, err = s.createWebSession(req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, res)
}

func (s *Server) createWebSession(req WebCreateSessionRequest) (SessionResult, error) {
	prompt, refusal := validatePromptText(req.Prompt, "the first prompt")
	if refusal != nil {
		return *refusal, nil
	}
	list, err := s.listProjectWorktrees()
	if err != nil {
		return SessionResult{}, err
	}
	i := slices.IndexFunc(list, func(p ProjectWorktrees) bool { return p.Project == req.Project })
	if i < 0 {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("project %q is not configured", req.Project)), nil
	}
	if req.Dir != "" {
		dir := filepath.Clean(req.Dir)
		known := filepath.Clean(list[i].Root) == dir || slices.ContainsFunc(list[i].Worktrees, func(wt string) bool { return filepath.Clean(wt) == dir })
		if !known {
			return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("dir %s is not the root or a worktree of %s", req.Dir, req.Project)), nil
		}
	}
	return s.createSession(CreateSessionRequest{
		Project:  req.Project,
		Dir:      req.Dir,
		Worktree: req.Worktree,
		Name:     req.Name,
		Prompt:   prompt,
		Tier:     req.Tier,
		Harness:  s.settings.Load().chatHarness(),
	})
}
