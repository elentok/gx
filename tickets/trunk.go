package tickets

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

// ErrTrunkMissing is the one error every caller reports when trunk can't be
// used: it is not a valid branch name or has no local branch.
type ErrTrunkMissing struct {
	Project         string // empty for a repo no project registers
	Repo            string
	Trunk           string
	FromProjectFile bool // false when the trunk was detected from the repo
	Invalid         bool // not a valid branch name
	RemoteOnly      bool // origin/<trunk> exists, the local branch doesn't
}

func (e *ErrTrunkMissing) Error() string {
	subject := fmt.Sprintf("project %q", e.Project)
	if e.Project == "" {
		subject = "repo " + e.Repo
	}
	source := fmt.Sprintf("trunk %q from project.json", e.Trunk)
	if !e.FromProjectFile {
		source = fmt.Sprintf("detected trunk %q", e.Trunk)
	}
	problem := "is not a local branch"
	if e.Invalid {
		problem = "is not a valid branch name"
	}
	msg := fmt.Sprintf("%s: %s %s", subject, source, problem)
	if e.RemoteOnly {
		msg += fmt.Sprintf("; create it with %q", "git branch "+e.Trunk+" origin/"+e.Trunk)
	}
	switch {
	case e.Project == "":
		msg += `; run "gx project add ." then set "trunk" in its project.json`
	case !e.FromProjectFile:
		msg += `; set "trunk" in project.json`
	}
	return msg
}

// Trunk resolves projectDir's trunk in repoRoot: project.json's trunk, else
// the repo's detected default. project.json is read on every call, never
// cached, so an edit applies at once; a detected value is never written back.
// The git reads are cached on the ref files they depend on (trunk_cache.go),
// so a repo edit applies at once too.
func Trunk(projectDir, repoRoot string) (string, error) {
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve trunk: %w", err)
	}
	if pf.Trunk != nil && *pf.Trunk != "" {
		return checkTrunk(&ErrTrunkMissing{Project: ProjectName(projectDir), Repo: repoRoot, Trunk: *pf.Trunk, FromProjectFile: true})
	}
	return checkTrunk(&ErrTrunkMissing{Project: ProjectName(projectDir), Repo: repoRoot, Trunk: remoteDefaultBranch(repoRoot)})
}

// TrunkFor resolves the trunk of dir's repo. A repo no project registers uses
// its detected default, still existence-checked.
func TrunkFor(dir string) (string, error) {
	repo, err := git.FindRepo(dir)
	if err != nil {
		return "", fmt.Errorf("not inside a git repo: %w", err)
	}
	return trunkForRoot(repo.Root)
}

// OpenRepo is FindRepo with Trunk set from the resolver. A resolver error
// still returns a usable Repo, with an empty Trunk, so callers that can
// degrade keep it; only a FindRepo failure returns no Repo.
func OpenRepo(dir string) (*git.Repo, error) {
	return openRepoWith(config.Load, dir)
}

// OpenRepoDirs is OpenRepo with the config and data dirs taken from dirs
// instead of the process env.
func OpenRepoDirs(dirs config.Dirs, dir string) (*git.Repo, error) {
	return openRepoWith(dirs.Load, dir)
}

func openRepoWith(load func() (config.Config, error), dir string) (*git.Repo, error) {
	repo, err := git.FindRepo(dir)
	if err != nil {
		return nil, err
	}
	repo.Trunk, err = trunkWith(load, repo.Root)
	return repo, err
}

func trunkForRoot(repoRoot string) (string, error) {
	return trunkWith(config.Load, repoRoot)
}

func trunkWith(load func() (config.Config, error), repoRoot string) (string, error) {
	projectDir, err := rootForRepoWith(load, repoRoot)
	if errors.Is(err, ErrNoProject) {
		return checkTrunk(&ErrTrunkMissing{Repo: repoRoot, Trunk: remoteDefaultBranch(repoRoot)})
	}
	if err != nil {
		return "", err
	}
	return Trunk(projectDir, repoRoot)
}

// checkTrunk returns candidate.Trunk when it names an existing local branch,
// else candidate itself filled in as the error.
func checkTrunk(candidate *ErrTrunkMissing) (string, error) {
	trunk := candidate.Trunk
	if !validBranchName(trunk) {
		candidate.Invalid = true
		return "", candidate
	}
	if refExists(candidate.Repo, "refs/heads/"+trunk) {
		return trunk, nil
	}
	candidate.RemoteOnly = refExists(candidate.Repo, "refs/remotes/origin/"+trunk)
	return "", candidate
}

// ProjectRepoRoot is the root of the project's registered repo, or "" when the
// project has none or the repo is gone.
func ProjectRepoRoot(storePath, project string) string {
	projectDir, err := ProjectDirByName(storePath, project)
	if err != nil {
		return ""
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil || pf.Repo == nil {
		return ""
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return ""
	}
	return repo.Root
}

// BranchExists reports whether repo has a local branch of that name. It is
// cached on the ref files, so a listing can ask once per epic.
func BranchExists(repo, branch string) bool {
	return refExists(repo, "refs/heads/"+branch)
}

// TrunkCheckout is the directory a design session runs in: the repo root of a
// normal repo, or the worktree with trunk checked out in a bare one. It is empty
// when the project, its repo, its trunk or the trunk's worktree is missing, so
// callers fall back to asking.
func TrunkCheckout(storePath, project string) string {
	projectDir, err := ProjectDirByName(storePath, project)
	if err != nil {
		return ""
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil || pf.Repo == nil {
		return ""
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return ""
	}
	if !repo.IsBare {
		return repo.Root
	}
	trunk, err := Trunk(projectDir, repo.Root)
	if err != nil {
		return ""
	}
	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		return ""
	}
	for _, wt := range wts {
		if !wt.IsBare && wt.Branch == trunk {
			return wt.Path
		}
	}
	return ""
}
