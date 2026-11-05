package projects

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/tickets/ticketmigrate"
)

func refuseSave(reason, msg string) (wire.SaveChecksResult, error) {
	return wire.SaveChecksResult{Refused: true, Reason: reason, Message: msg}, nil
}

func (c *Checks) Save(req wire.SaveChecksRequest) (wire.SaveChecksResult, error) {
	dir, err := c.projects.Dir(req.Project)
	if err != nil {
		return refuseSave(wire.ReasonUnknownProject, "no project named "+req.Project)
	}
	if len(req.Checks) == 0 {
		return refuseSave(wire.ReasonInvalidChecks, "checks is required")
	}
	if _, err := check.ParseConfig(req.Checks); err != nil {
		return refuseSave(wire.ReasonInvalidChecks, err.Error())
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil {
		return wire.SaveChecksResult{}, err
	}
	switch req.Target {
	case wire.SaveTargetProjectJSON:
		return saveChecksToProjectJSON(dir, pf, req)
	case wire.SaveTargetRepo:
		if pf.VCS != nil && *pf.VCS == config.VCSNone {
			return refuseSave(wire.ReasonVCSNone, req.Project+" has no VCS; save to project.json")
		}
		return c.SaveToRepo(dir, req)
	}
	return refuseSave(wire.ReasonBadTarget, "target must be "+wire.SaveTargetRepo+" or "+wire.SaveTargetProjectJSON)
}

func saveChecksToProjectJSON(dir string, pf config.ProjectFile, req wire.SaveChecksRequest) (wire.SaveChecksResult, error) {
	pf.Checks = req.Checks
	if err := ticketmigrate.WriteProjectFile(dir, pf); err != nil {
		return wire.SaveChecksResult{}, err
	}
	return wire.SaveChecksResult{Project: req.Project, Target: req.Target, Path: filepath.Join(dir, config.ProjectFileName)}, nil
}

func (c *Checks) SaveToRepo(dir string, req wire.SaveChecksRequest) (wire.SaveChecksResult, error) {
	root := ticketgit.TrunkCheckout(c.cfg().TicketStore, req.Project)
	repo := ticketgit.ProjectRepoRoot(c.cfg().TicketStore, req.Project)
	if root == "" || repo == "" {
		return refuseSave(wire.ReasonNoTrunk, req.Project+" has no trunk checkout")
	}
	trunk, err := ticketgit.Trunk(dir, repo)
	if err != nil {
		return refuseSave(wire.ReasonNoTrunk, err.Error())
	}
	defer c.guards.Trunks.Lock(repo, trunk)()

	branch, err := git.CurrentBranch(root)
	if err != nil {
		return refuseSave(wire.ReasonWrongBranch, "cannot read the branch of "+root+": "+err.Error())
	}
	if branch != trunk {
		return refuseSave(wire.ReasonWrongBranch, root+" is on "+branch+", not "+trunk)
	}
	status, _, err := git.Run(root, git.NonInteractiveEnv(), "status", "--porcelain", "--", check.RepoConfigPath)
	if err != nil {
		return refuseSave(wire.ReasonCommitFailed, err.Error())
	}
	if status != "" {
		return refuseSave(wire.ReasonDirtyChecks, check.RepoConfigPath+" has uncommitted changes in "+root)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, req.Checks, "", "  "); err != nil {
		return refuseSave(wire.ReasonInvalidChecks, err.Error())
	}
	pretty.WriteByte('\n')
	sha, err := commitChecksFile(root, pretty.Bytes())
	var ce *commitError
	switch {
	case errors.As(err, &ce):
		return refuseSave(wire.ReasonCommitFailed, ce.Error())
	case err != nil:
		return wire.SaveChecksResult{}, err
	}
	return wire.SaveChecksResult{Project: req.Project, Target: req.Target, Trunk: trunk, Commit: sha}, nil
}

// commitError is a git failure while committing the checks file, which the
// save reports as a refusal rather than a server error.
type commitError struct{ err error }

func (e *commitError) Error() string { return e.err.Error() }
func (e *commitError) Unwrap() error { return e.err }

// commitChecksFile writes body to the repo checks file in root and commits it.
// It returns an empty sha when the file already holds body. A failed add or
// commit restores the file and the index and returns a *commitError.
func commitChecksFile(root string, body []byte) (sha string, err error) {
	file := filepath.Join(root, check.RepoConfigPath)
	prev, readErr := os.ReadFile(file)
	if readErr == nil && bytes.Equal(prev, body) {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, body, 0o644); err != nil {
		return "", err
	}
	env := git.NonInteractiveEnv()
	for _, args := range [][]string{
		{"add", "--", check.RepoConfigPath},
		{"commit", "-q", "--only", "-m", "Save gx check config", "--", check.RepoConfigPath},
	} {
		if _, _, err := git.Run(root, env, args...); err != nil {
			_, _, _ = git.Run(root, env, "reset", "-q", "--", check.RepoConfigPath)
			if readErr == nil {
				_ = os.WriteFile(file, prev, 0o644)
			} else {
				_ = os.Remove(file)
			}
			return "", &commitError{err}
		}
	}
	sha, _, err = git.Run(root, env, "rev-parse", "--short", "HEAD")
	return sha, err
}
