package herdr

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Agent describes a herdr agent pane, as returned by AgentPrompt.
type Agent struct {
	Name         string // empty for an agent herdr detected rather than started by name
	PaneID       string
	WorkspaceID  string
	TabID        string
	AgentStatus  string
	AgentSession string // agent_session.value: the underlying agent's session id (for Claude, its Claude Code session UUID)
	// StateChangeSeq is herdr's per-pane counter that only advances when the
	// pane's observed state actually changes, distinguishing "idle because it
	// genuinely finished a turn" from "idle because it never processed
	// anything since it was created".
	StateChangeSeq int
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
func (c CLI) AgentPrompt(opts AgentPromptOptions) (Agent, error) {
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

	return c.runAgentJSON(args)
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
func (c CLI) AgentStart(opts AgentStartOptions) (Agent, error) {
	args := []string{"agent", "start", opts.Name, "--kind", opts.Kind, "--pane", opts.Pane}
	if opts.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMs))
	}
	if len(opts.AgentArgs) > 0 {
		args = append(args, "--")
		args = append(args, opts.AgentArgs...)
	}

	return c.runAgentJSON(args)
}

// AgentWaitOptions are the arguments/flags for AgentWait.
type AgentWaitOptions struct {
	Target    string
	Until     []string
	TimeoutMs int
}

// AgentGet returns the current state of an existing agent without waiting for
// a transition or submitting another prompt.
func (c CLI) AgentGet(target string) (Agent, error) {
	return c.runAgentJSON([]string{"agent", "get", target})
}

// AgentWait blocks until target reaches one of the requested states (or any
// of idle/done/blocked if Until is empty) via `herdr agent wait`, without
// submitting a prompt. Used to wait out an already-working agent's turn,
// e.g. after AgentPrompt's own --wait only confirmed the turn started.
func (c CLI) AgentWait(opts AgentWaitOptions) (Agent, error) {
	args := []string{"agent", "wait", opts.Target}
	for _, until := range opts.Until {
		args = append(args, "--until", until)
	}
	if opts.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMs))
	}

	return c.runAgentJSON(args)
}

// AgentList returns every agent herdr knows, across all workspaces.
func (c CLI) AgentList() ([]Agent, error) {
	var result struct {
		Agents []agentInfo `json:"agents"`
	}
	if err := c.runJSON([]string{"agent", "list"}, &result); err != nil {
		return nil, err
	}
	agents := make([]Agent, len(result.Agents))
	for i, info := range result.Agents {
		agents[i] = info.agent()
	}
	return agents, nil
}

// agentInfo is herdr's AgentInfo JSON shape.
type agentInfo struct {
	Name         string `json:"name"`
	PaneID       string `json:"pane_id"`
	WorkspaceID  string `json:"workspace_id"`
	TabID        string `json:"tab_id"`
	AgentStatus  string `json:"agent_status"`
	AgentSession *struct {
		Value string `json:"value"`
	} `json:"agent_session"`
	StateChangeSeq int `json:"state_change_seq"`
}

func (info agentInfo) agent() Agent {
	agent := Agent{
		Name:           info.Name,
		PaneID:         info.PaneID,
		WorkspaceID:    info.WorkspaceID,
		TabID:          info.TabID,
		AgentStatus:    info.AgentStatus,
		StateChangeSeq: info.StateChangeSeq,
	}
	if info.AgentSession != nil {
		agent.AgentSession = info.AgentSession.Value
	}
	return agent
}

// runAgentJSON runs a herdr command whose JSON result has a top-level
// `agent` field of the AgentInfo shape (agent prompt/start/wait all do), and
// parses it into an Agent.
func (c CLI) runAgentJSON(args []string) (Agent, error) {
	var result struct {
		Agent agentInfo `json:"agent"`
	}
	if err := c.runJSON(args, &result); err != nil {
		return Agent{}, err
	}
	return result.Agent.agent(), nil
}

// PaneShellPID returns the pid of the shell running in pane, via `herdr pane
// process-info`.
func (c CLI) PaneShellPID(pane string) (int, error) {
	var result struct {
		ProcessInfo struct {
			ShellPID int `json:"shell_pid"`
		} `json:"process_info"`
	}
	if err := c.runJSON([]string{"pane", "process-info", "--pane", pane}, &result); err != nil {
		return 0, err
	}
	if result.ProcessInfo.ShellPID <= 0 {
		return 0, fmt.Errorf("herdr pane process-info %s: no shell_pid", pane)
	}
	return result.ProcessInfo.ShellPID, nil
}

// AgentExplainResult is herdr's own diagnosis of a pane's current state:
// which detection rule its pane monitor matched and the state that rule
// assigns.
type AgentExplainResult struct {
	State         string
	MatchedRuleID string
}

// AgentExplain reports which detection rule herdr's pane monitor matched for
// target's current state, via `herdr agent explain`. Unlike the other agent
// subcommands, explain's JSON is the bare diagnostic object rather than the
// {"result": ...} envelope runJSON expects, so it parses the response
// directly instead of going through runJSON.
func (c CLI) AgentExplain(target string) (AgentExplainResult, error) {
	out, err := c.run("agent", "explain", target, "--format", "json")
	if err != nil {
		return AgentExplainResult{}, err
	}
	var resp struct {
		State       string `json:"state"`
		MatchedRule *struct {
			ID string `json:"id"`
		} `json:"matched_rule"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return AgentExplainResult{}, fmt.Errorf("parsing herdr agent explain output: %w", err)
	}
	result := AgentExplainResult{State: resp.State}
	if resp.MatchedRule != nil {
		result.MatchedRuleID = resp.MatchedRule.ID
	}
	return result, nil
}

// AgentSendKeys sends key presses to an agent via `herdr agent send-keys`.
func (c CLI) AgentSendKeys(target string, keys ...string) error {
	args := append([]string{"agent", "send-keys", target}, keys...)
	_, err := c.run(args...)
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
func (c CLI) AgentRead(target string, opts AgentReadOptions) (string, error) {
	args := []string{"agent", "read", target}
	args = appendFlag(args, "--source", opts.Source)
	if opts.Lines > 0 {
		args = append(args, "--lines", strconv.Itoa(opts.Lines))
	}
	args = appendFlag(args, "--format", opts.Format)

	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
