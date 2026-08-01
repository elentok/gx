package herdr

// Tab describes a herdr tab.
type Tab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
	Label       string `json:"label"`
	Focused     bool   `json:"focused"`
	PaneCount   int    `json:"pane_count"`
	AgentStatus string `json:"agent_status"`
}

// TabCreateOptions are the flags for TabCreate.
type TabCreateOptions struct {
	WorkspaceID string
	Cwd         string
	Label       string
	Focus       bool
}

// CreatedTab is the result of TabCreate: the new tab plus its root pane id,
// needed to run a command (e.g. launch an agent) inside it.
type CreatedTab struct {
	Tab
	RootPaneID string
}

// TabCreate creates a new tab via `herdr tab create`.
func TabCreate(opts TabCreateOptions) (CreatedTab, error) {
	args := []string{"tab", "create"}
	args = appendFlag(args, "--workspace", opts.WorkspaceID)
	args = appendFlag(args, "--cwd", opts.Cwd)
	args = appendFlag(args, "--label", opts.Label)
	if opts.Focus {
		args = append(args, "--focus")
	}

	var result struct {
		Tab      Tab `json:"tab"`
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
	}
	if err := runJSON(args, &result); err != nil {
		return CreatedTab{}, err
	}
	return CreatedTab{
		Tab:        result.Tab,
		RootPaneID: result.RootPane.PaneID,
	}, nil
}

// TabList lists the tabs in workspaceID (or every tab across workspaces if
// workspaceID is "") via `herdr tab list`.
func TabList(workspaceID string) ([]Tab, error) {
	args := []string{"tab", "list"}
	args = appendFlag(args, "--workspace", workspaceID)

	var result struct {
		Tabs []Tab `json:"tabs"`
	}
	if err := runJSON(args, &result); err != nil {
		return nil, err
	}
	return result.Tabs, nil
}

// TabClose closes tabID via `herdr tab close`.
func TabClose(tabID string) error {
	_, err := run("tab", "close", tabID)
	return err
}
