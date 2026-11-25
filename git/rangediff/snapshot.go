package rangediff

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// MaxSnapshotUntracked is the most untracked files a snapshot takes in. Above
// it, none go in.
const MaxSnapshotUntracked = 500

// Snapshot is a commit holding a worktree's tracked changes plus the kept
// untracked files, on top of HEAD.
type Snapshot struct {
	// SHA is the snapshot commit. Use it as Range.SecondSHA.
	SHA string
	// Dirty is true when the snapshot's tree differs from HEAD's tree.
	Dirty bool
	// LeftOut counts the untracked files not in the snapshot.
	LeftOut int
}

const snapshotIdent = "gx snapshot"

// SnapshotWorktree builds a snapshot commit of dir's worktree. It works on a
// private copy of the index, so the worktree's own index, lock files and file
// times stay untouched. Only loose objects in the shared object store are
// written.
func SnapshotWorktree(dir string) (Snapshot, error) {
	tmp, err := os.MkdirTemp("", "gx-snapshot-")
	if err != nil {
		return Snapshot{}, err
	}
	defer os.RemoveAll(tmp)

	copyPath := filepath.Join(tmp, "index")
	if err := copyIndex(dir, copyPath); err != nil {
		return Snapshot{}, err
	}
	env := append(run.NonInteractiveEnv(),
		"GIT_INDEX_FILE="+copyPath,
		"GIT_AUTHOR_NAME="+snapshotIdent, "GIT_AUTHOR_EMAIL=snapshot@gx.invalid",
		"GIT_COMMITTER_NAME="+snapshotIdent, "GIT_COMMITTER_EMAIL=snapshot@gx.invalid",
		// commit-tree reads commit.gpgsign from the user's config; never sign.
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=commit.gpgsign", "GIT_CONFIG_VALUE_0=false",
	)
	git := func(o run.RunOptions, args ...string) (string, error) {
		out, _, err := run.RunWith(dir, env, o, args...)
		return out, err
	}

	if _, err := git(run.RunOptions{}, "add", "-u"); err != nil {
		return Snapshot{}, err
	}
	kept, leftOut, err := keptUntracked(dir, env)
	if err != nil {
		return Snapshot{}, err
	}
	if len(kept) > 0 {
		in := strings.NewReader(strings.Join(kept, "\x00") + "\x00")
		if _, err := git(run.RunOptions{Stdin: in}, "--literal-pathspecs", "add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return Snapshot{}, err
		}
	}
	tree, err := git(run.RunOptions{}, "write-tree")
	if err != nil {
		return Snapshot{}, err
	}
	headTree, err := git(run.RunOptions{}, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return Snapshot{}, err
	}
	sha, err := git(run.RunOptions{}, "commit-tree", tree, "-p", "HEAD", "-m", "gx worktree snapshot")
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{SHA: sha, Dirty: tree != headTree, LeftOut: leftOut}, nil
}

// copyIndex copies dir's index to dst. A worktree with no index file gets an
// empty copy.
func copyIndex(dir, dst string) error {
	out, _, err := run.Exec(dir, []string{"rev-parse", "--path-format=absolute", "--git-path", "index"})
	if err != nil {
		return err
	}
	src, err := os.Open(out)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer src.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// keptUntracked lists the unignored untracked files that go in the snapshot
// and counts the ones left out.
func keptUntracked(dir string, env []string) (kept []string, leftOut int, err error) {
	out, _, err := run.RunRaw(dir, env, "ls-files", "-o", "--exclude-standard", "-z")
	if err != nil {
		return nil, 0, err
	}
	var all []string
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) > 0 {
			all = append(all, string(p))
		}
	}
	if len(all) > MaxSnapshotUntracked {
		return nil, len(all), nil
	}
	for _, p := range all {
		st, err := os.Lstat(filepath.Join(dir, p))
		if err != nil {
			return nil, 0, fmt.Errorf("snapshot: %w", err)
		}
		if st.IsDir() || st.Size() > MaxBlobBytes {
			leftOut++
			continue
		}
		kept = append(kept, p)
	}
	return kept, leftOut, nil
}
