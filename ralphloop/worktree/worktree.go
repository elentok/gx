package worktree

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/deps"
)

// Lock serializes git worktree add/remove per process, as Run's worktreeMu
// does per run. It is the one instance: every caller takes this lock.
var Lock sync.Mutex

// WithWorktreeLock runs fn holding Lock, which PrepareFeature and
// PrepareIteration take around creating or reusing a feature worktree, so fn
// sees a worktree no claim is setting up.
func WithWorktreeLock(fn func() error) error {
	Lock.Lock()
	defer Lock.Unlock()
	return fn()
}

// DirInside reports whether dir is path or below it, through symlinks and by
// whole path elements. An empty or unresolvable side is not inside.
func DirInside(dir, path string) bool {
	if dir == "" || path == "" {
		return false
	}
	rd, errD := filepath.EvalSymlinks(dir)
	rp, errP := filepath.EvalSymlinks(path)
	if errD != nil || errP != nil {
		return false
	}
	rel, err := filepath.Rel(rp, rd)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// BranchExists reports whether branch is a resolvable ref in dir's repo,
// treating any RevParse error as "doesn't exist" — the same convention
// classifyDoneTicket uses to check the iteration branch's presence.
func BranchExists(d deps.Deps, dir, branch string) bool {
	_, err := d.RevParse(dir, branch)
	return err == nil
}

// FindIterationSession returns the iteration's live session, if the runner
// knows one. An unreachable runner reports none: recovery has to keep working
// when the host is down.
func FindIterationSession(d deps.Deps, epic, identifier string) (agentrunner.Session, bool) {
	if d.Runner == nil {
		return agentrunner.Session{}, false
	}
	s, ok, err := d.Runner.Find(IterationLabel(epic, identifier))
	if err != nil || !ok {
		return agentrunner.Session{}, false
	}
	return s, true
}

// RemoveIterationWorktree removes the leftover iteration worktree of
// epic/identifier, keeping its branch (it holds commits). A missing worktree is
// not an error.
func RemoveIterationWorktree(d deps.Deps, repoDir, epic, identifier string) error {
	wtDir, err := d.WorktreeDir(repoDir)
	if err != nil {
		return fmt.Errorf("resolving worktree directory for %q: %w", repoDir, err)
	}
	path := IterationWorktreePath(wtDir, epic, identifier)
	return WithWorktreeLock(func() error {
		exists, err := d.WorktreeExists(path)
		if err != nil {
			return fmt.Errorf("checking iteration worktree: %w", err)
		}
		if !exists {
			return nil
		}
		if err := d.RemoveWorktree(repoDir, path, true); err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
		return nil
	})
}
