package herdr

// FindWorkspace returns the workspace_id of the herdr workspace labeled
// label, or "" if none exists.
func FindWorkspace(label string) (string, error) {
	var result struct {
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"workspaces"`
	}
	if err := runJSON([]string{"workspace", "list"}, &result); err != nil {
		return "", err
	}
	for _, ws := range result.Workspaces {
		if ws.Label == label {
			return ws.WorkspaceID, nil
		}
	}
	return "", nil
}

// FindOrCreateWorkspace focuses the herdr workspace labeled label, creating
// one rooted at cwd (and focusing it) if none exists yet. Returns the
// workspace's id either way.
func FindOrCreateWorkspace(label, cwd string) (string, error) {
	id, err := FindWorkspace(label)
	if err != nil {
		return "", err
	}
	if id != "" {
		if _, err := run("workspace", "focus", id); err != nil {
			return "", err
		}
		return id, nil
	}

	var result struct {
		Workspace struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspace"`
	}
	if err := runJSON([]string{"workspace", "create", "--cwd", cwd, "--label", label, "--focus"}, &result); err != nil {
		return "", err
	}
	return result.Workspace.WorkspaceID, nil
}

// EnsureWorkspace returns the id of the herdr workspace labeled label,
// creating one rooted at cwd if none exists yet — without focusing it either
// way. Used by callers (e.g. a ralph-loop run launched from an already-
// visible gx TUI) that want the workspace to exist but don't want the
// running session yanked over to it.
func EnsureWorkspace(label, cwd string) (string, error) {
	id, err := FindWorkspace(label)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}

	var result struct {
		Workspace struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspace"`
	}
	if err := runJSON([]string{"workspace", "create", "--cwd", cwd, "--label", label}, &result); err != nil {
		return "", err
	}
	return result.Workspace.WorkspaceID, nil
}
