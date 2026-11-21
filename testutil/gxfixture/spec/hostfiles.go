package spec

// HostFilesShapes are the --shape values of the host-files scenario, the first
// is the default. no-main-worktree leaves demo's trunk checked out nowhere, so
// a project base resolves no relative path (see hostfiles.go in the fixture program).
var HostFilesShapes = []string{"main-worktree", "no-main-worktree"}

// HostFilesEpic and HostFilesTicket are the epic and ticket of demo whose body
// names a relative and an absolute path.
const (
	HostFilesEpic   = "files-epic"
	HostFilesTicket = "01"
)
