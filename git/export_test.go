package git

// IdentifyDirSlow is the git-forking path, the oracle for IdentifyDir's fast path.
var IdentifyDirSlow = identifyDirSlow

// FastIdentified reports whether IdentifyDir would skip git for dir.
func FastIdentified(dir string) bool { _, ok := fastWorktreeFields(dir); return ok }

// ResolveRevFast is IdentifyDir's sibling for refs: the no-fork answer for rev.
var ResolveRevFast = resolveRev

// CurrentBranchFast is the no-fork CurrentBranch.
var CurrentBranchFast = currentBranchFast
