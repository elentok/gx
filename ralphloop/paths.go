package ralphloop

// reconcilePaths bundles the filesystem locations verify needs: the scratch
// dir (to read run-log.jsonl), the feature worktree (to run git checks in) and
// the iteration worktree directory.
type reconcilePaths struct {
	ScratchDir      string
	FeatureWorktree string
	WorktreeDir     string
	RepoDir         string
}
