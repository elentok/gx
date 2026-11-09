package herdr

// PaneRun runs command in pane's shell via `herdr pane run`. Only for a fresh
// pane gx just created: an agent_blocked pane must not be bypassed this way
// (see AgentBlockedError).
func (c CLI) PaneRun(pane string, command ...string) error {
	_, err := c.run(append([]string{"pane", "run", pane}, command...)...)
	return err
}
