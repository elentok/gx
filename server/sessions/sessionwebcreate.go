package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"
)

// WebCreateSessionRequest is the body of POST /v1/sessions/create on the web
// mux. It has no ticket or worktree-bookkeeping fields: the page may
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
	Tier agent.Tier `json:"tier,omitempty"`
	// Harness is the agent CLI to run, such as claude or codex; empty means claude.
	Harness string `json:"harness,omitempty"`
	// Workspace is the workspace the chat lands in; missing or unknown reads as Default.
	Workspace string `json:"workspace,omitempty"`
	// Model and Effort start the chat on exactly this model and effort, not the
	// tier's. Effort needs a Model.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// ImagePaths are server-side image files pasted before the prompt. Never
	// read from a request body.
	ImagePaths []string `json:"-"`
}

// SessionCreateWeb is sessionCreateWrite for the web mux. A body with any
// other field is refused, not dropped.
func (mg *Manager) SessionCreateWeb(w http.ResponseWriter, r *http.Request) {
	var req WebCreateSessionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var res wire.SessionResult
	if err := dec.Decode(&req); err != nil {
		res = Refusal(wire.ReasonSessionBadRequest, err.Error())
	} else {
		var err error
		if res, err = mg.CreateWebSession(req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	wire.WriteJSON(w, res)
}

// CheckProjectDir returns a refusal when project is not configured or a
// non-empty dir is neither its root nor one of its worktrees; nil means ok.
func CheckProjectDir(mg *Manager, project, dir string) (*wire.SessionResult, error) {
	list, err := mg.projects.ListWorktrees()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(list, func(p wire.ProjectWorktrees) bool { return p.Project == project })
	if i < 0 {
		res := Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %q is not configured", project))
		return &res, nil
	}
	if dir != "" {
		clean := filepath.Clean(dir)
		known := filepath.Clean(list[i].Root) == clean || slices.ContainsFunc(list[i].Worktrees, func(wt string) bool { return filepath.Clean(wt) == clean })
		if !known {
			res := Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("dir %s is not the root or a worktree of %s", dir, project))
			return &res, nil
		}
	}
	return nil, nil
}

// CreateWebSession is the web create route's logic, shared with chat draft Start.
func (mg *Manager) CreateWebSession(req WebCreateSessionRequest) (wire.SessionResult, error) {
	prompt, refusal := wire.ValidatePromptText(req.Prompt, "the first prompt")
	if refusal != nil {
		return *refusal, nil
	}
	if req.Effort != "" && req.Model == "" {
		return Refusal(wire.ReasonSessionBadRequest, "effort needs a model"), nil
	}
	refusal, err := CheckProjectDir(mg, req.Project, req.Dir)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if refusal != nil {
		return *refusal, nil
	}
	res, err := mg.CreateSession(wire.CreateSessionRequest{
		Project:  req.Project,
		Dir:      req.Dir,
		Worktree: req.Worktree,
		Name:     req.Name,
		Prompt:   prompt,
		Tier:     req.Tier,
		Harness:  req.Harness,
	}, SessionCtx{Model: req.Model, Effort: req.Effort, ImagePaths: req.ImagePaths})
	if err == nil && res.Session != nil && req.Workspace != "" && mg.workspaces != nil {
		// An unknown name is refused by SetMember and the chat stays in Default.
		if err := mg.workspaces.SetMember(res.Session.ID, req.Workspace); err != nil {
			mg.log.Warn("place new chat in its workspace", "session", res.Session.ID, "err", err)
		}
	}
	return res, err
}
