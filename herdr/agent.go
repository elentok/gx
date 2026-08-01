package herdr

import "strconv"

// Agent describes a herdr agent pane, as returned by AgentPrompt.
type Agent struct {
	PaneID       string
	WorkspaceID  string
	TabID        string
	AgentStatus  string
	AgentSession string // agent_session.value: the underlying agent's session id (for Claude, its Claude Code session UUID)
}

// AgentPromptOptions are the arguments/flags for AgentPrompt.
type AgentPromptOptions struct {
	Target string
	Text   string
	// Wait, if true, blocks until the agent reaches one of the Until states
	// (or any of idle/done/blocked if Until is empty) before returning.
	Wait      bool
	Until     []string
	TimeoutMs int
}

// AgentPrompt submits a prompt to an agent via `herdr agent prompt`,
// optionally waiting (combining the submit and wait into one call) for the
// agent to settle into one of the requested states.
func AgentPrompt(opts AgentPromptOptions) (Agent, error) {
	args := []string{"agent", "prompt", opts.Target, opts.Text}
	if opts.Wait {
		args = append(args, "--wait")
	}
	for _, until := range opts.Until {
		args = append(args, "--until", until)
	}
	if opts.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMs))
	}

	return runAgentJSON(args)
}

// AgentStartOptions are the arguments/flags for AgentStart.
type AgentStartOptions struct {
	Name      string
	Kind      string // e.g. "claude"
	Pane      string
	TimeoutMs int
	// AgentArgs are passed through to the underlying agent executable after
	// `--`, e.g. []string{"--permission-mode", "auto"} for claude.
	AgentArgs []string
}

// AgentStart starts a supported interactive agent (e.g. claude) in an
// existing pane via `herdr agent start`. The pane must already be at an
// interactive shell prompt.
func AgentStart(opts AgentStartOptions) (Agent, error) {
	args := []string{"agent", "start", opts.Name, "--kind", opts.Kind, "--pane", opts.Pane}
	if opts.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMs))
	}
	if len(opts.AgentArgs) > 0 {
		args = append(args, "--")
		args = append(args, opts.AgentArgs...)
	}

	return runAgentJSON(args)
}

// AgentWaitOptions are the arguments/flags for AgentWait.
type AgentWaitOptions struct {
	Target    string
	Until     []string
	TimeoutMs int
}

// AgentWait blocks until target reaches one of the requested states (or any
// of idle/done/blocked if Until is empty) via `herdr agent wait`, without
// submitting a prompt. Used to wait out an already-working agent's turn,
// e.g. after AgentPrompt's own --wait only confirmed the turn started.
func AgentWait(opts AgentWaitOptions) (Agent, error) {
	args := []string{"agent", "wait", opts.Target}
	for _, until := range opts.Until {
		args = append(args, "--until", until)
	}
	if opts.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMs))
	}

	return runAgentJSON(args)
}

// runAgentJSON runs a herdr command whose JSON result has a top-level
// `agent` field of the AgentInfo shape (agent prompt/start/wait all do), and
// parses it into an Agent.
func runAgentJSON(args []string) (Agent, error) {
	var result struct {
		Agent struct {
			PaneID       string `json:"pane_id"`
			WorkspaceID  string `json:"workspace_id"`
			TabID        string `json:"tab_id"`
			AgentStatus  string `json:"agent_status"`
			AgentSession *struct {
				Value string `json:"value"`
			} `json:"agent_session"`
		} `json:"agent"`
	}
	if err := runJSON(args, &result); err != nil {
		return Agent{}, err
	}
	agent := Agent{
		PaneID:      result.Agent.PaneID,
		WorkspaceID: result.Agent.WorkspaceID,
		TabID:       result.Agent.TabID,
		AgentStatus: result.Agent.AgentStatus,
	}
	if result.Agent.AgentSession != nil {
		agent.AgentSession = result.Agent.AgentSession.Value
	}
	return agent, nil
}

// AgentSendKeys sends key presses to an agent via `herdr agent send-keys`.
func AgentSendKeys(target string, keys ...string) error {
	args := append([]string{"agent", "send-keys", target}, keys...)
	_, err := run(args...)
	return err
}

// AgentReadOptions are the flags for AgentRead.
type AgentReadOptions struct {
	Source string // "visible", "recent" (default), "recent-unwrapped", or "detection"
	Lines  int
	Format string // "text" (default) or "ansi"
}

// AgentRead reads an agent pane's terminal output via `herdr agent read`.
// Unlike the other herdr commands, this returns the raw terminal text
// directly rather than a JSON envelope.
func AgentRead(target string, opts AgentReadOptions) (string, error) {
	args := []string{"agent", "read", target}
	args = appendFlag(args, "--source", opts.Source)
	if opts.Lines > 0 {
		args = append(args, "--lines", strconv.Itoa(opts.Lines))
	}
	args = appendFlag(args, "--format", opts.Format)

	out, err := run(args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
