package rangediff

import "github.com/elentok/gx/git/internal/run"

// CommitsAvailable reports whether every SHA is a commit in the object store.
func CommitsAvailable(dir string, shas ...string) bool {
	for _, sha := range shas {
		if !IsObjectID(sha) {
			return false
		}
		if _, _, err := run.Exec(dir, []string{"cat-file", "-e", sha + "^{commit}"}); err != nil {
			return false
		}
	}
	return true
}
