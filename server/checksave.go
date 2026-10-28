package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// Save targets of a check config.
const (
	SaveTargetRepo        = "repo"
	SaveTargetProjectJSON = "project.json"
)

// Refusal reasons of a check save.
const (
	ReasonInvalidChecks = "invalid-checks"
	ReasonBadTarget     = "bad-target"
	ReasonVCSNone       = "vcs-none"
	ReasonNoTrunk       = "no-trunk-checkout"
	ReasonWrongBranch   = "wrong-branch"
	ReasonDirtyChecks   = "dirty-checks"
	ReasonCommitFailed  = "commit-failed"
)

// SaveChecksRequest saves one project's check config. Checks has the shape of
// .gx/checks.json.
type SaveChecksRequest struct {
	Project string          `json:"project"`
	Target  string          `json:"target"`
	Checks  json.RawMessage `json:"checks"`
}

// SaveChecksResult says what a save did. A refusal saved nothing.
type SaveChecksResult struct {
	Project string `json:"project,omitempty"`
	Target  string `json:"target,omitempty"`
	// Trunk and Commit are set for a repo save. Commit is empty when the file
	// already held this config.
	Trunk  string `json:"trunk,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Path is the file written for a project.json save.
	Path    string `json:"path,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func refuseSave(reason, msg string) (SaveChecksResult, error) {
	return SaveChecksResult{Refused: true, Reason: reason, Message: msg}, nil
}

func (s *Server) checksSaveWrite(w http.ResponseWriter, r *http.Request) {
	var req SaveChecksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.saveChecks(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) saveChecks(req SaveChecksRequest) (SaveChecksResult, error) {
	dir, err := s.projectDir(req.Project)
	if err != nil {
		return refuseSave(ReasonUnknownProject, "no project named "+req.Project)
	}
	if len(req.Checks) == 0 {
		return refuseSave(ReasonInvalidChecks, "checks is required")
	}
	if _, err := check.ParseConfig(req.Checks); err != nil {
		return refuseSave(ReasonInvalidChecks, err.Error())
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil {
		return SaveChecksResult{}, err
	}
	switch req.Target {
	case SaveTargetProjectJSON:
		return saveChecksToProjectJSON(dir, pf, req)
	case SaveTargetRepo:
		if pf.VCS != nil && *pf.VCS == config.VCSNone {
			return refuseSave(ReasonVCSNone, req.Project+" has no VCS; save to project.json")
		}
		return s.saveChecksToRepo(dir, req)
	}
	return refuseSave(ReasonBadTarget, "target must be "+SaveTargetRepo+" or "+SaveTargetProjectJSON)
}

func saveChecksToProjectJSON(dir string, pf config.ProjectFile, req SaveChecksRequest) (SaveChecksResult, error) {
	pf.Checks = req.Checks
	if err := tickets.WriteProjectFile(dir, pf); err != nil {
		return SaveChecksResult{}, err
	}
	return SaveChecksResult{Project: req.Project, Target: req.Target, Path: filepath.Join(dir, config.ProjectFileName)}, nil
}

func (s *Server) saveChecksToRepo(dir string, req SaveChecksRequest) (SaveChecksResult, error) {
	root := tickets.TrunkCheckout(s.cfg.TicketStore, req.Project)
	repo := tickets.ProjectRepoRoot(s.cfg.TicketStore, req.Project)
	if root == "" || repo == "" {
		return refuseSave(ReasonNoTrunk, req.Project+" has no trunk checkout")
	}
	trunk, err := tickets.Trunk(dir, repo)
	if err != nil {
		return refuseSave(ReasonNoTrunk, err.Error())
	}
	defer s.trunkLocks.lock(repo, trunk)()

	if branch, err := git.CurrentBranch(root); err != nil || branch != trunk {
		return refuseSave(ReasonWrongBranch, root+" is on "+branch+", not "+trunk)
	}
	status, err := gitOut(root, "status", "--porcelain", "--", check.RepoConfigPath)
	if err != nil {
		return refuseSave(ReasonCommitFailed, err.Error())
	}
	if status != "" {
		return refuseSave(ReasonDirtyChecks, check.RepoConfigPath+" has uncommitted changes in "+root)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, req.Checks, "", "  "); err != nil {
		return refuseSave(ReasonInvalidChecks, err.Error())
	}
	pretty.WriteByte('\n')
	file := filepath.Join(root, check.RepoConfigPath)
	prev, readErr := os.ReadFile(file)
	if readErr == nil && bytes.Equal(prev, pretty.Bytes()) {
		return SaveChecksResult{Project: req.Project, Target: req.Target, Trunk: trunk}, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return SaveChecksResult{}, err
	}
	if err := os.WriteFile(file, pretty.Bytes(), 0o644); err != nil {
		return SaveChecksResult{}, err
	}
	rollback := func() {
		_, _ = gitOut(root, "reset", "-q", "--", check.RepoConfigPath)
		if readErr == nil {
			_ = os.WriteFile(file, prev, 0o644)
		} else {
			_ = os.Remove(file)
		}
	}
	if _, err := gitOut(root, "add", "--", check.RepoConfigPath); err != nil {
		rollback()
		return refuseSave(ReasonCommitFailed, err.Error())
	}
	if _, err := gitOut(root, "commit", "-q", "--only", "-m", "Save gx check config", "--", check.RepoConfigPath); err != nil {
		rollback()
		return refuseSave(ReasonCommitFailed, err.Error())
	}
	sha, err := gitOut(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return SaveChecksResult{}, err
	}
	return SaveChecksResult{Project: req.Project, Target: req.Target, Trunk: trunk, Commit: sha}, nil
}

// gitOut runs git in dir; the error carries git's output.
func gitOut(dir string, args ...string) (string, error) {
	stdout, stderr, err := git.Run(dir, git.NonInteractiveEnv(), args...)
	text := strings.TrimSpace(stdout + "\n" + stderr)
	if err != nil {
		return text, &gitError{args: args, out: text, err: err}
	}
	return text, nil
}

type gitError struct {
	args []string
	out  string
	err  error
}

func (e *gitError) Error() string {
	return "git " + strings.Join(e.args, " ") + ": " + e.out + " (" + e.err.Error() + ")"
}
