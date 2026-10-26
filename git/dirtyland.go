package git

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BackupRefPrefix is where gx keeps its own backups of uncommitted files. It
// is never refs/stash, which every worktree of the repo shares.
const BackupRefPrefix = "refs/gx/backups/"

// SnapshotPaths writes a commit, on top of HEAD, in which each of paths holds
// its working-tree content in dir (a path missing there is absent from the
// commit). It touches only the object store and a temporary index: no ref, no
// file in dir. The returned commit is unreachable until a ref names it.
func SnapshotPaths(dir string, paths []string) (string, error) {
	tmp, err := os.MkdirTemp("", "gx-snapshot-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := append(NonInteractiveEnv(),
		"GIT_INDEX_FILE="+filepath.Join(tmp, "index"),
		"GIT_AUTHOR_NAME=gx", "GIT_AUTHOR_EMAIL=gx@localhost",
		"GIT_COMMITTER_NAME=gx", "GIT_COMMITTER_EMAIL=gx@localhost",
	)
	steps := [][]string{
		{"read-tree", "HEAD"},
		append([]string{"update-index", "--add", "--remove", "--"}, paths...),
	}
	for _, args := range steps {
		if _, _, err := runEnv(dir, env, args); err != nil {
			return "", err
		}
	}
	tree, _, err := runEnv(dir, env, []string{"write-tree"})
	if err != nil {
		return "", err
	}
	commit, _, err := runEnv(dir, env, []string{"commit-tree", tree, "-p", "HEAD", "-m", "gx backup of uncommitted files"})
	return commit, err
}

// MergeSnapshotMarked replays the changes snapshot made to HEAD onto branch
// without touching any file. tree is the merged tree; when clean is false the
// merge conflicts and each conflicting file in tree holds its conflict markers.
func MergeSnapshotMarked(dir, snapshot, branch string) (tree string, clean bool, err error) {
	out, _, err := run(dir, []string{"merge-tree", "--write-tree", "--merge-base=HEAD", snapshot, branch})
	var runErr *RunError
	if errors.As(err, &runErr) && runErr.Code == 1 {
		tree, _, _ = strings.Cut(runErr.Stdout, "\n")
		return tree, false, nil
	}
	if err != nil {
		return "", false, err
	}
	tree, _, _ = strings.Cut(out, "\n")
	return tree, true, nil
}

// treePaths is the subset of paths that exist in the tree-ish.
func treePaths(dir, treeish string, paths []string) (map[string]bool, error) {
	out, _, err := run(dir, append([]string{"ls-tree", "-r", "--name-only", "-z", treeish, "--"}, paths...))
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			in[p] = true
		}
	}
	return in, nil
}

// SetAside returns each of paths in dir to HEAD's content: a tracked file is
// reset, in the index too, and a file HEAD lacks is dropped from index and disk.
func SetAside(dir string, paths []string) error {
	tracked, err := treePaths(dir, "HEAD", paths)
	if err != nil {
		return err
	}
	var keep, drop []string
	for _, p := range paths {
		if tracked[p] {
			keep = append(keep, p)
		} else {
			drop = append(drop, p)
		}
	}
	if len(keep) > 0 {
		if _, _, err := run(dir, append([]string{"checkout", "HEAD", "--"}, keep...)); err != nil {
			return err
		}
	}
	if len(drop) > 0 {
		if _, _, err := run(dir, append([]string{"rm", "-q", "-f", "--cached", "--ignore-unmatch", "--"}, drop...)); err != nil {
			return err
		}
	}
	for _, p := range drop {
		if err := os.Remove(filepath.Join(dir, p)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// RestoreFromTree writes each of paths into dir's working tree as the tree-ish
// has it, and removes the ones the tree-ish lacks. The index is left alone.
func RestoreFromTree(dir, treeish string, paths []string) error {
	in, err := treePaths(dir, treeish, paths)
	if err != nil {
		return err
	}
	var write []string
	for _, p := range paths {
		if in[p] {
			write = append(write, p)
		} else if err := os.Remove(filepath.Join(dir, p)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if len(write) == 0 {
		return nil
	}
	_, _, err = run(dir, append([]string{"restore", "--source=" + treeish, "--worktree", "--"}, write...))
	return err
}

// UpdateRef points ref at the object sha, creating it.
func UpdateRef(repoRoot, ref, sha string) error {
	_, _, err := run(repoRoot, []string{"update-ref", ref, sha})
	return err
}

// DeleteRef removes ref; a missing ref is not an error.
func DeleteRef(repoRoot, ref string) error {
	_, _, err := run(repoRoot, []string{"update-ref", "-d", ref})
	return err
}

// BackupRef is a ref under BackupRefPrefix and when its commit was made.
type BackupRef struct {
	Name string
	Time time.Time
}

// ListBackupRefs lists the refs under BackupRefPrefix.
func ListBackupRefs(repoRoot string) ([]BackupRef, error) {
	out, _, err := run(repoRoot, []string{"for-each-ref", "--format=%(refname) %(committerdate:unix)", BackupRefPrefix})
	if err != nil {
		return nil, err
	}
	var refs []BackupRef
	for _, line := range strings.Split(out, "\n") {
		name, unix, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		secs, err := strconv.ParseInt(unix, 10, 64)
		if err != nil {
			continue
		}
		refs = append(refs, BackupRef{Name: name, Time: time.Unix(secs, 0)})
	}
	return refs, nil
}
