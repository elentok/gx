package wire

// Tip is one entry of the tips page. Folders is filled by the call for tips
// that point at the project worktree folders.
type Tip struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Platform string   `json:"platform"` // GOOS value the tip applies to
	Folders  []string `json:"folders,omitempty"`
}
