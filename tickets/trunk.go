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
	switch {
	case e.RemoteOnly:
		msg += fmt.Sprintf("; create it with %q", "git branch "+e.Trunk+" origin/"+e.Trunk)
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
func Trunk(projectDir, repoRoot string) (string, error) {
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve trunk: %w", err)
	}
	if pf.Trunk != nil && *pf.Trunk != "" {
		return checkTrunk(&ErrTrunkMissing{Project: ProjectName(projectDir), Repo: repoRoot, Trunk: *pf.Trunk, FromProjectFile: true})
	}
	return checkTrunk(&ErrTrunkMissing{Project: ProjectName(projectDir), Repo: repoRoot, Trunk: git.RemoteDefaultBranch(repoRoot)})
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
	repo, err := git.FindRepo(dir)
	if err != nil {
		return nil, err
	}
	repo.Trunk, err = trunkForRoot(repo.Root)
	return repo, err
}

func trunkForRoot(repoRoot string) (string, error) {
	projectDir, err := rootForRepo(repoRoot)
	if errors.Is(err, ErrNoProject) {
		return checkTrunk(&ErrTrunkMissing{Repo: repoRoot, Trunk: git.RemoteDefaultBranch(repoRoot)})
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
	if !git.ValidBranchName(trunk) {
		candidate.Invalid = true
		return "", candidate
	}
	if git.RefExists(candidate.Repo, "refs/heads/"+trunk) {
		return trunk, nil
	}
	candidate.RemoteOnly = git.RefExists(candidate.Repo, "refs/remotes/origin/"+trunk)
	return "", candidate
}
