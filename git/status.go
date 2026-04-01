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

// DefaultMainRemoteRef returns the best remote mainline ref for repoRoot.
// It prefers origin/<default-branch>, then falls back to origin/main/master if present.
func DefaultMainRemoteRef(repoRoot string) string {
	defaultBranch := RemoteDefaultBranch(repoRoot)
	candidates := []string{defaultBranch, "main", "master"}
	seen := map[string]bool{}
	for _, branch := range candidates {
		branch = strings.TrimSpace(branch)
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		if runAllowFail(repoRoot, []string{"rev-parse", "--verify", "refs/remotes/origin/" + branch}) != "" {
			return "origin/" + branch
		}
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
