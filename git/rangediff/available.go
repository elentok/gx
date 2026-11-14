package rangediff

import "github.com/elentok/gx/git/internal/run"

// IsObjectID reports whether s is a full lowercase object id. A typed ref of
// that shape names a commit by SHA (a stamped range), not a branch or tag.
func IsObjectID(s string) bool { return isObjectID(s) }

// CommitsAvailable reports whether every SHA is a commit in the object store.
func CommitsAvailable(dir string, shas ...string) bool {
	for _, sha := range shas {
		if !isObjectID(sha) {
			return false
		}
		if _, _, err := run.Exec(dir, []string{"cat-file", "-e", sha + "^{commit}"}); err != nil {
			return false
		}
	}
	return true
}
