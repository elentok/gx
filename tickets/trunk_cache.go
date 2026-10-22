package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/git"
)

// The trunk resolver runs on every server snapshot, claim and merge, and each
// resolve costs several git processes. The git reads it makes are cached
// here, each with a stamp of the ref files that can change its answer. A
// lookup re-stats those files (no process) and asks git again on any change,
// so a repo edit applies at once, like a project.json edit.

// The git reads the cache wraps; a test swaps them to count calls.
var (
	gitRemoteDefaultBranch = git.RemoteDefaultBranch
	gitValidBranchName     = git.ValidBranchName
	gitRefExists           = git.RefExists
)

type refKey struct{ kind, repo, arg string }

type refEntry struct {
	stamp string
	value string
}

var (
	refCacheMu sync.Mutex
	refCache   = map[refKey]refEntry{}
)

// cachedRead returns compute's value, reusing the last one while files' stamp
// is unchanged. ok false (an unreadable layout) always computes.
func cachedRead(key refKey, files []string, ok bool, compute func() string) string {
	if !ok {
		return compute()
	}
	stamp := statStamp(files)
	refCacheMu.Lock()
	e, hit := refCache[key]
	refCacheMu.Unlock()
	if hit && e.stamp == stamp {
		return e.value
	}
	value := compute()
	refCacheMu.Lock()
	refCache[key] = refEntry{stamp: stamp, value: value}
	refCacheMu.Unlock()
	return value
}

// statStamp is each file's size and mtime, "-" for a missing one.
func statStamp(files []string) string {
	var b strings.Builder
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			b.WriteString("-|")
			continue
		}
		fmt.Fprintf(&b, "%d.%d|", fi.Size(), fi.ModTime().UnixNano())
	}
	return b.String()
}

// refStore is the directory holding repo's loose refs and packed-refs: its
// .git dir, or repo itself when bare. ok is false for any other layout (a
// linked worktree's .git file, the reftable backend), which is never cached.
func refStore(repo string) (string, bool) {
	dir := repo
	if fi, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		if !fi.IsDir() {
			return "", false
		}
		dir = filepath.Join(repo, ".git")
	}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(dir, "reftable")); err == nil {
		return "", false
	}
	return dir, true
}

func refFiles(store string, refs ...string) []string {
	files := []string{filepath.Join(store, "packed-refs")}
	for _, ref := range refs {
		files = append(files, filepath.Join(store, ref))
	}
	return files
}

// RefsStamp changes whenever a branch in repo moves, is created or is deleted,
// or any of extra changes. A git ref update replaces its file through a rename,
// so the refs/heads directories' mtimes catch it without stat-ing every
// branch. ok false (an unreadable layout) means the caller must not cache on it.
func RefsStamp(repo string, extra ...string) (stamp string, ok bool) {
	store, ok := refStore(repo)
	if !ok {
		return "", false
	}
	files := append(refFiles(store, "HEAD", "refs/remotes/origin/HEAD"), extra...)
	var dirs []string
	_ = filepath.WalkDir(filepath.Join(store, "refs", "heads"), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	return statStamp(append(files, dirs...)), true
}

// remoteDefaultBranch is git.RemoteDefaultBranch, cached on the refs it reads.
func remoteDefaultBranch(repo string) string {
	store, ok := refStore(repo)
	files := refFiles(store, "refs/remotes/origin/HEAD", "refs/heads/main", "refs/heads/master")
	return cachedRead(refKey{"default", repo, ""}, files, ok, func() string { return gitRemoteDefaultBranch(repo) })
}

// refExists is git.RefExists, cached on the ref's loose file and packed-refs.
func refExists(repo, ref string) bool {
	store, ok := refStore(repo)
	return cachedRead(refKey{"exists", repo, ref}, refFiles(store, ref), ok, func() string {
		if gitRefExists(repo, ref) {
			return "1"
		}
		return ""
	}) == "1"
}

// validBranchName is git.ValidBranchName, cached for good: git's rules for a
// name don't change while the process runs.
func validBranchName(name string) bool {
	return cachedRead(refKey{"valid", "", name}, nil, true, func() string {
		if gitValidBranchName(name) {
			return "1"
		}
		return ""
	}) == "1"
}
