package worktrees

import (
	"encoding/json"
	"fmt"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// cmdHerdrSession focuses the herdr workspace labeled name, creating one
// rooted at path if none exists yet.
func cmdHerdrSession(name, path string) tea.Cmd {
	return func() tea.Msg {
		id, err := herdrFindWorkspace(name)
		if err != nil {
			return terminalResultMsg{err: err}
		}
		if id != "" {
			err := exec.Command("herdr", "workspace", "focus", id).Run()
			return terminalResultMsg{err: err}
		}
		err = exec.Command("herdr", "workspace", "create", "--cwd", path, "--label", name, "--focus").Run()
		return terminalResultMsg{err: err}
	}
}

// herdrFindWorkspace returns the workspace_id of the herdr workspace labeled
// label, or "" if none exists.
func herdrFindWorkspace(label string) (string, error) {
	out, err := exec.Command("herdr", "workspace", "list").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("$ herdr workspace list\n\n%w\n\n%s", err, string(out))
	}
	var resp struct {
		Result struct {
			Workspaces []struct {
				WorkspaceID string `json:"workspace_id"`
				Label       string `json:"label"`
			} `json:"workspaces"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parsing herdr workspace list output: %w", err)
	}
	for _, ws := range resp.Result.Workspaces {
		if ws.Label == label {
			return ws.WorkspaceID, nil
		}
	}
	return "", nil
}
