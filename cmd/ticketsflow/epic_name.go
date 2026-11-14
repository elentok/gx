package ticketsflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/tickets/ticketgit"
)

// maxEpicNameSuffix bounds nextEpicName's search: base, base-2 … base-N.
const maxEpicNameSuffix = 99

// epicNameTaken says why name can't be a new epic of the project at
// projectDir, or "" when it is free. A name is taken when its epic directory
// exists, when the project's repo has a branch of that name, or when trunk has
// a commit whose Ralph-Loop-Ticket trailer names it. The last two outlive a
// merged epic's directory: reusing the name would give new tickets the
// addresses of merged ones, and the merge check would read the old trailers as
// theirs. A project with no repo, or a repo whose trunk can't be resolved,
// skips the git checks.
func epicNameTaken(projectDir, project, name string) (string, error) {
	if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
		return fmt.Sprintf("epic %q already exists", name), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil || pf.Repo == nil {
		return "", nil
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return "", nil
	}
	if ticketgit.BranchExists(repo.Root, name) {
		return fmt.Sprintf("epic name %q is taken: branch %q exists in %s", name, name, repo.Root), nil
	}
	trunk, err := ticketgit.Trunk(projectDir, repo.Root)
	if err != nil {
		return "", nil
	}
	sha, err := history.TrailerPrefixCommit(repo.Root, trunk, merge.TicketTrailerKey, project+":"+name+"/", name+"/")
	if err != nil {
		return "", err
	}
	if sha != "" {
		return fmt.Sprintf("epic name %q was used before: %s has its merged tickets (commit %.9s)", name, trunk, sha), nil
	}
	return "", nil
}

// refuseTakenEpicName is epicNameTaken as an error, naming the next free name.
func refuseTakenEpicName(projectDir, project, name string) error {
	why, err := epicNameTaken(projectDir, project, name)
	if err != nil || why == "" {
		return err
	}
	if next, err := nextEpicName(projectDir, project, name); err == nil {
		why += fmt.Sprintf("; pick another name, such as %q", next)
	}
	return errors.New(why)
}

// nextEpicName returns base when it is free, else the first free base-N for N
// from 2 up to maxEpicNameSuffix.
func nextEpicName(projectDir, project, base string) (string, error) {
	for n := 1; n <= maxEpicNameSuffix; n++ {
		name := base
		if n > 1 {
			name = base + "-" + strconv.Itoa(n)
		}
		why, err := epicNameTaken(projectDir, project, name)
		if err != nil {
			return "", err
		}
		if why == "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no free epic name from %s to %s-%d", base, base, maxEpicNameSuffix)
}
