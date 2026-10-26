package git

import (
	"fmt"
	"strconv"
	"strings"
)

// SyncStatusName describes the relationship between a branch and its upstream.
type SyncStatusName string

const (
	StatusSame     SyncStatusName = "same"
	StatusAhead    SyncStatusName = "ahead"
	StatusBehind   SyncStatusName = "behind"
	StatusDiverged SyncStatusName = "diverged"
	StatusUnknown  SyncStatusName = "unknown"
)

// SyncStatus holds the sync relationship between a local branch and a reference.
type SyncStatus struct {
	Name   SyncStatusName
	Ahead  int // commits the local branch has that upstream doesn't
	Behind int // commits upstream has that the local branch doesn't
}

// Pretty returns a human-readable summary of the sync status.
func (s SyncStatus) Pretty() string {
	switch s.Name {
	case StatusSame:
		return "synced"
	case StatusAhead:
		return fmt.Sprintf("%d ahead", s.Ahead)
	case StatusBehind:
		return fmt.Sprintf("%d behind", s.Behind)
	case StatusDiverged:
		return fmt.Sprintf("%d ahead, %d behind", s.Ahead, s.Behind)
	default:
		return "unknown"
	}
}

// ChangeKind classifies a file change in the working tree.
type ChangeKind string

const (
	ChangeModified  ChangeKind = "M"
	ChangeAdded     ChangeKind = "A"
	ChangeDeleted   ChangeKind = "D"
	ChangeRenamed   ChangeKind = "R"
	ChangeUntracked ChangeKind = "?"
)

// Change represents a single file change reported by git status.
type Change struct {
	Kind ChangeKind
	Path string
}

// WorktreeSyncStatus returns the sync status of a worktree branch compared to
// its upstream remote tracking branch. Returns StatusUnknown if no upstream is
// configured.
func WorktreeSyncStatus(repo Repo, branch string) (SyncStatus, error) {
	upstream := UpstreamBranch(repo.Root, branch)
	if upstream == "" {
		return SyncStatus{Name: StatusUnknown}, nil
	}
	return syncBetween(repo.Root, branch, upstream)
}

// BranchSyncStatusAgainstRef compares a local branch/ref against an explicit
// remote reference (for example: branch vs origin/main).
func BranchSyncStatusAgainstRef(repoRoot, localRef, remoteRef string) (SyncStatus, error) {
	if strings.TrimSpace(localRef) == "" || strings.TrimSpace(remoteRef) == "" {
		return SyncStatus{Name: StatusUnknown}, nil
	}
	return syncBetween(repoRoot, localRef, remoteRef)
}

// TrunkBaseRef returns origin/<trunk> when it exists, else the local trunk,
// else "" (also for an empty trunk). It never guesses another branch.
func TrunkBaseRef(repoRoot, trunk string) string {
	trunk = strings.TrimSpace(trunk)
	if trunk == "" {
		return ""
	}
	if runAllowFail(repoRoot, []string{"rev-parse", "--verify", "refs/remotes/origin/" + trunk}) != "" {
		return "origin/" + trunk
	}
	if runAllowFail(repoRoot, []string{"rev-parse", "--verify", "refs/heads/" + trunk}) != "" {
		return trunk
	}
	return ""
}

func syncBetween(repoRoot, localRef, remoteRef string) (SyncStatus, error) {
	localHash := runAllowFail(repoRoot, []string{"rev-parse", "--verify", localRef})
	remoteHash := runAllowFail(repoRoot, []string{"rev-parse", "--verify", remoteRef})

	if localHash == "" || remoteHash == "" {
		return SyncStatus{Name: StatusUnknown}, nil
	}
	if localHash == remoteHash {
		return SyncStatus{Name: StatusSame}, nil
	}

	ahead, err := revCount(repoRoot, remoteRef, localRef)
	if err != nil {
		return SyncStatus{Name: StatusUnknown}, err
	}
	behind, err := revCount(repoRoot, localRef, remoteRef)
	if err != nil {
		return SyncStatus{Name: StatusUnknown}, err
	}

	var name SyncStatusName
	switch {
	case ahead > 0 && behind > 0:
		name = StatusDiverged
	case ahead > 0:
		name = StatusAhead
	case behind > 0:
		name = StatusBehind
	default:
		name = StatusUnknown
	}

	return SyncStatus{Name: name, Ahead: ahead, Behind: behind}, nil
}

func revCount(repoRoot, fromRef, toRef string) (int, error) {
	out, _, err := run(repoRoot, []string{"rev-list", "--count", fromRef + ".." + toRef})
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("invalid rev-list count %q: %w", out, err)
	}
	return n, nil
}

// WorktreeStatusSummary returns counts of staged, unstaged, and untracked
// changes in the worktree at dir.
func WorktreeStatusSummary(dir string) (staged, unstaged, untracked int, err error) {
	out, _, err := runNoOptionalLocks(dir, []string{"status", "--porcelain=v1"})
	if err != nil {
		return 0, 0, 0, err
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			untracked++
		} else {
			if x != ' ' {
				staged++
			}
			if y != ' ' {
				unstaged++
			}
		}
	}
	return staged, unstaged, untracked, nil
}

// UncommittedPaths returns every path with uncommitted changes in the worktree:
// each untracked file inside a new directory, and both paths of a rename or copy.
func UncommittedPaths(worktreePath string) ([]string, error) {
	out, _, err := runNoOptionalLocks(worktreePath, []string{"status", "--porcelain=v1", "--untracked-files=all", "-z"})
	if err != nil {
		return nil, err
	}

	var paths []string
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		tok := parts[i]
		if len(tok) < 4 || tok[2] != ' ' {
			continue
		}
		paths = append(paths, tok[3:])
		// In -z format a rename or copy is followed by its source path.
		if strings.ContainsAny(tok[:2], "RC") && i+1 < len(parts) && parts[i+1] != "" {
			i++
			paths = append(paths, parts[i])
		}
	}
	return paths, nil
}

// UncommittedChanges returns modified, added, deleted, and untracked files in
// the worktree at the given path.
func UncommittedChanges(worktreePath string) ([]Change, error) {
	out, _, err := runNoOptionalLocks(worktreePath, []string{"status", "--porcelain=v1"})
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var changes []Change
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		xy := strings.TrimSpace(line[:2])
		path := line[3:]
		if xy == "" || path == "" {
			continue
		}

		kind := ChangeKind(string(xy[0]))
		if xy == "??" {
			kind = ChangeUntracked
		}

		changes = append(changes, Change{Kind: kind, Path: path})
	}
	return changes, nil
}
