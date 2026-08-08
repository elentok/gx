package git

import "strings"

// MergeFastForward runs `git merge --ff-only branch` in dir. ok is true on
// success. A refusal because branch is not a fast-forward of dir's current
// HEAD is an expected outcome, not a failure: it's reported as ok=false,
// err=nil (detected via git's stable "Not possible to fast-forward" stderr
// message, since the exit code alone doesn't distinguish it from other
// failures). Any other failure is returned as err.
func MergeFastForward(dir, branch string) (ok bool, err error) {
	_, stderr, err := run(dir, []string{"merge", "--ff-only", branch})
	if err == nil {
		return true, nil
	}
	if strings.Contains(stderr, "Not possible to fast-forward") {
		return false, nil
	}
	return false, err
}
