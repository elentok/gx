package status

// hasStashableChanges reports whether the current file list holds anything that
// the requested stash variant would capture. Untracked files are excluded: the
// stash is run without --include-untracked (out of scope), so `git stash push`
// ignores them and a tree of only-untracked files is "nothing to stash".
func (m Model) hasStashableChanges(stagedOnly bool) bool {
	for _, f := range m.statusData.files {
		if f.HasStagedChanges() {
			return true
		}
		if !stagedOnly && !f.IsUntracked() && f.HasUnstagedChanges() {
			return true
		}
	}
	return false
}
