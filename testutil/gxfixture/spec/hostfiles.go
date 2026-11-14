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

// HostFilesOutput is what the host-files scenario prints for its flows.
type HostFilesOutput struct {
	// Repo is demo's main checkout; SecondWorktree holds the file only the
	// second worktree has (SecondOnly, relative to it).
	Repo           string `json:"repo"`
	SecondWorktree string `json:"second_worktree"`
	SecondOnly     string `json:"second_only"`
	// TempDir holds shot-1.png, owned by the running user.
	TempDir string `json:"temp_dir"`
	// OtherOrigin is the second local origin the mock page loads a script from
	// and tries to fetch.
	OtherOrigin string `json:"other_origin"`
	// Ticket is the address of the ticket whose body holds the two paths.
	Ticket string `json:"ticket"`
}
