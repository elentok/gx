package git

// RevParse resolves ref to its full commit hash in dir.
func RevParse(dir, ref string) (string, error) {
	out, _, err := run(dir, []string{"rev-parse", ref})
	return out, err
}

// CherryPickRange cherry-picks the commit range fromExclusive..toInclusive
// (in order) onto dir's current branch via `git cherry-pick`. On a conflict,
// the returned error is a *RunError and dir is left mid-cherry-pick with
// conflict markers in place, same as running the command by hand.
func CherryPickRange(dir, fromExclusive, toInclusive string) error {
	_, _, err := run(dir, []string{"cherry-pick", fromExclusive + ".." + toInclusive})
	return err
}

// CherryPickInProgress reports whether dir has a cherry-pick sequence
// currently stopped on a conflict (CHERRY_PICK_HEAD present).
func CherryPickInProgress(dir string) (bool, error) {
	_, _, err := run(dir, []string{"rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD"})
	if err != nil {
		if _, ok := err.(*RunError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
