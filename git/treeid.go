package git

import (
	"os"
	"path/filepath"
)

// TreeID writes a tree of dir's whole working tree (tracked and untracked
// files, minus ignored ones) and returns its id. The tree is built in a
// temporary index, so the real index and every ref stay untouched, and the id
// is the same for the same content however it is staged or committed.
func TreeID(dir string) (string, error) {
	tmp, err := os.MkdirTemp("", "gx-treeid-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := append(NonInteractiveEnv(), "GIT_INDEX_FILE="+filepath.Join(tmp, "index"))
	if _, err := RevParse(dir, "HEAD"); err == nil {
		if _, _, err := runEnv(dir, env, []string{"read-tree", "HEAD"}); err != nil {
			return "", err
		}
	}
	if _, _, err := runEnv(dir, env, []string{"add", "-A"}); err != nil {
		return "", err
	}
	tree, _, err := runEnv(dir, env, []string{"write-tree"})
	return tree, err
}
