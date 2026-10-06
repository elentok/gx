package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
)

// ErrNoProject means no project in the ticket store points at the repo.
var ErrNoProject = errors.New("no ticket-store project for this repo; run `gx tickets migrate`")

// ProjectDir finds the project directory in the ticket store whose
// project.json names repoRoot as its repo.
func ProjectDir(storePath, repoRoot string) (string, error) {
	if storePath == "" {
		return "", errors.New("ticket-store.path is not set")
	}
	entries, err := os.ReadDir(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoProject
		}
		return "", fmt.Errorf("read ticket store %s: %w", storePath, err)
	}
	want := canonicalPath(repoRoot)
	for _, e := range entries {
		dir := filepath.Join(storePath, e.Name())
		if !e.IsDir() {
			continue
		}
		pf, err := config.ReadProjectFile(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if pf.Repo != nil && canonicalPath(*pf.Repo) == want {
			return dir, nil
		}
	}
	return "", ErrNoProject
}

// canonicalPath makes equal directories compare equal across symlinks (e.g.
// macOS /var vs /private/var) and trailing slashes.
func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}
