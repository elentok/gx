package herdr

// Worktree describes a herdr-managed git worktree, as returned by
// WorktreeCreate/WorktreeOpen.
type Worktree struct {
	WorkspaceID string
	TabID       string
	PaneID      string
	Path        string
	Branch      string
	Label       string
	AlreadyOpen bool // set by WorktreeOpen when the worktree was already open
}

// worktreeResultJSON is the JSON shape shared by `worktree create` and
// `worktree open` responses.
type worktreeResultJSON struct {
	AlreadyOpen bool `json:"already_open"`
	Workspace   struct {
		WorkspaceID string `json:"workspace_id"`
	} `json:"workspace"`
	Tab struct {
		TabID string `json:"tab_id"`
	} `json:"tab"`
	RootPane struct {
		PaneID string `json:"pane_id"`
	} `json:"root_pane"`
	Worktree struct {
		Path   string `json:"path"`
		Branch string `json:"branch"`
		Label  string `json:"label"`
	} `json:"worktree"`
}

func (r worktreeResultJSON) toWorktree() Worktree {
	return Worktree{
		WorkspaceID: r.Workspace.WorkspaceID,
		TabID:       r.Tab.TabID,
		PaneID:      r.RootPane.PaneID,
		Path:        r.Worktree.Path,
		Branch:      r.Worktree.Branch,
		Label:       r.Worktree.Label,
		AlreadyOpen: r.AlreadyOpen,
	}
}

// WorktreeCreateOptions are the flags for WorktreeCreate. WorkspaceID is the
// parent workspace to create the worktree's tab in; leave it "" to let herdr
// pick the current workspace.
type WorktreeCreateOptions struct {
	WorkspaceID string
	Cwd         string
	Branch      string
	Base        string
	Path        string
	Label       string
	Focus       bool
}

// WorktreeCreate creates and opens a new git worktree via `herdr worktree
// create`.
func WorktreeCreate(opts WorktreeCreateOptions) (Worktree, error) {
	args := []string{"worktree", "create"}
	if opts.WorkspaceID != "" {
		args = appendFlag(args, "--workspace", opts.WorkspaceID)
	} else {
		args = appendFlag(args, "--cwd", opts.Cwd)
	}
	args = appendFlag(args, "--branch", opts.Branch)
	args = appendFlag(args, "--base", opts.Base)
	args = appendFlag(args, "--path", opts.Path)
	args = appendFlag(args, "--label", opts.Label)
	if opts.Focus {
		args = append(args, "--focus")
	}

	var result worktreeResultJSON
	if err := runJSON(args, &result); err != nil {
		return Worktree{}, err
	}
	return result.toWorktree(), nil
}

// WorktreeOpenOptions are the flags for WorktreeOpen.
type WorktreeOpenOptions struct {
	WorkspaceID string
	Cwd         string
	Branch      string
	Path        string
	Label       string
	Focus       bool
}

// WorktreeOpen opens an existing git worktree via `herdr worktree open`.
func WorktreeOpen(opts WorktreeOpenOptions) (Worktree, error) {
	args := []string{"worktree", "open"}
	if opts.WorkspaceID != "" {
		args = appendFlag(args, "--workspace", opts.WorkspaceID)
	} else {
		args = appendFlag(args, "--cwd", opts.Cwd)
	}
	args = appendFlag(args, "--branch", opts.Branch)
	args = appendFlag(args, "--path", opts.Path)
	args = appendFlag(args, "--label", opts.Label)
	if opts.Focus {
		args = append(args, "--focus")
	}

	var result worktreeResultJSON
	if err := runJSON(args, &result); err != nil {
		return Worktree{}, err
	}
	return result.toWorktree(), nil
}

// WorktreeRemove removes the worktree checkout backing workspaceID via
// `herdr worktree remove`.
func WorktreeRemove(workspaceID string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", workspaceID}
	if force {
		args = append(args, "--force")
	}
	_, err := run(args...)
	return err
}
