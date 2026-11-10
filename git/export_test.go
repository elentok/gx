package git

// IdentifyDirSlow is the git-forking path, the oracle for IdentifyDir's fast path.
var IdentifyDirSlow = identifyDirSlow

// FastIdentified reports whether IdentifyDir would skip git for dir.
func FastIdentified(dir string) bool { _, ok := fastWorktreeFields(dir); return ok }
