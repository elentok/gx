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
