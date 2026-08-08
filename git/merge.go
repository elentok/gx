package git

// MergeFastForward runs `git merge --ff-only branch` in dir. ok is true on
// success. A refusal because branch is not a fast-forward of dir's current
// HEAD is an expected outcome, not a failure: it's reported as ok=false,
// err=nil. Fast-forwardability is decided up front via IsAncestor rather
// than by matching merge stderr, since git localizes that text under
// non-English LC_ALL/LANG. Any other failure is returned as err.
func MergeFastForward(dir, branch string) (ok bool, err error) {
	ancestor, err := IsAncestor(dir, "HEAD", branch)
	if err != nil {
		return false, err
	}
	if !ancestor {
		return false, nil
	}
	if _, _, err := run(dir, []string{"merge", "--ff-only", branch}); err != nil {
		return false, err
	}
	return true, nil
}
