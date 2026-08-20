package herdrfake

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var codexRolloutEpoch = time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)

type CodexQuota struct {
	UsedPercent float64
	ResetsAt    time.Time
}

type CodexUsage struct {
	ContextTokens int
	TotalTokens   int
	Primary       CodexQuota
	Secondary     CodexQuota
}

type CodexRolloutOptions struct {
	Cwd            string
	WorkspaceID    string
	TabID          string
	PaneID         string
	AgentID        string
	AgentName      string
	SessionID      string
	InitialUsage   CodexUsage
	CompactedUsage CodexUsage
	FinalUsage     CodexUsage

	// RejectPromptWhenBlocked models herdr 0.8.2's agent_blocked guard: when
	// true, a prompt sent while the agent's status is "blocked" is rejected
	// with the real error envelope instead of being accepted. Off by default
	// so existing scenarios (written before herdr enforced this guard) keep
	// passing unchanged; tickets that need the guard opt in explicitly.
	RejectPromptWhenBlocked bool
}

type codexRolloutPhase string

const (
	codexPhaseIdle                codexRolloutPhase = "idle"
	codexPhaseRunning             codexRolloutPhase = "running"
	codexPhaseGenericBlocked      codexRolloutPhase = "generic-blocked"
	codexPhaseCompactConfirmation codexRolloutPhase = "compact-confirmation"
	codexPhaseCompactContinuation codexRolloutPhase = "compact-continuation"
	codexPhaseContinuation        codexRolloutPhase = "continuation"
	codexPhaseFinalPending        codexRolloutPhase = "final-pending"
	codexPhaseFinal               codexRolloutPhase = "final"
)

type CodexRollout struct {
	state     *State
	opts      CodexRolloutOptions
	sessionID string
	path      string
	started   bool
	line      int
	phase     codexRolloutPhase
}

func RegisterCodexRollout(t testing.TB, state *State, opts CodexRolloutOptions) *CodexRollout {
	t.Helper()
	if opts.Cwd == "" || opts.PaneID == "" || opts.AgentID == "" || opts.AgentName == "" {
		t.Fatal("RegisterCodexRollout requires cwd, pane, agent id, and agent name")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		t.Fatal("RegisterCodexRollout requires CODEX_HOME")
	}
	if opts.SessionID == "" {
		opts.SessionID = stableCodexSessionID(opts.Cwd, opts.PaneID, opts.AgentID)
	}
	path := filepath.Join(
		codexHome,
		"sessions",
		codexRolloutEpoch.Format("2006"),
		codexRolloutEpoch.Format("01"),
		codexRolloutEpoch.Format("02"),
		"rollout-"+codexRolloutEpoch.Format("2006-01-02T15-04-05")+"-"+opts.SessionID+".jsonl",
	)
	rollout := &CodexRollout{state: state, opts: opts, sessionID: opts.SessionID, path: path, phase: codexPhaseIdle}
	state.Register("agent", "start", rollout.start)
	state.Register("agent", "prompt", rollout.prompt)
	state.Register("agent", "wait", rollout.wait)
	state.Register("agent", "send-keys", rollout.sendKeys)
	return rollout
}

func (c *CodexRollout) SessionID() string {
	return c.sessionID
}

func (c *CodexRollout) Path() string {
	return c.path
}

func (c *CodexRollout) start(state *State, argv []string) (any, Identities, error) {
	if len(argv) < 3 || argv[2] != c.opts.AgentName {
		return nil, Identities{}, fmt.Errorf("unexpected Codex start: %v", argv)
	}
	if !c.started {
		if err := c.writeSessionMeta(); err != nil {
			return nil, Identities{}, err
		}
		if err := c.writeUsage(c.opts.InitialUsage); err != nil {
			return nil, Identities{}, err
		}
		c.started = true
	}
	state.Agents[c.opts.AgentID] = &Agent{
		ID: c.opts.AgentID, PaneID: c.opts.PaneID, Name: c.opts.AgentName,
		Kind: "codex", Status: "idle", SessionID: c.sessionID,
	}
	state.Sessions[c.sessionID] = &Session{ID: c.sessionID, AgentID: c.opts.AgentID}
	return c.agentResult("idle"), c.identities(), nil
}

func (c *CodexRollout) prompt(state *State, argv []string) (any, Identities, error) {
	if len(argv) < 4 || argv[2] != c.opts.PaneID {
		return nil, Identities{}, fmt.Errorf("unexpected Codex prompt: %v", argv)
	}
	if c.opts.RejectPromptWhenBlocked {
		if agent := state.Agents[c.opts.AgentID]; agent != nil && agent.Status == "blocked" {
			return nil, c.identities(), AgentBlockedError(c.opts.AgentName)
		}
	}
	status := "working"
	switch {
	case argv[3] == "/compact":
		c.phase = codexPhaseCompactConfirmation
		status = "blocked"
	case containsFinishUp(argv[3]):
		c.phase = codexPhaseContinuation
	default:
		c.phase = codexPhaseRunning
	}
	c.setStatus(state, status)
	return c.agentResult(status), c.identities(), nil
}

func (c *CodexRollout) wait(state *State, argv []string) (any, Identities, error) {
	if len(argv) < 3 || argv[2] != c.opts.PaneID {
		return nil, Identities{}, fmt.Errorf("unexpected Codex wait: %v", argv)
	}
	switch c.phase {
	case codexPhaseRunning:
		return nil, c.identities(), fmt.Errorf("timed out waiting for agent status")
	case codexPhaseGenericBlocked:
		return c.agentResult("blocked"), c.identities(), nil
	case codexPhaseCompactConfirmation:
		c.phase = codexPhaseCompactContinuation
		c.setStatus(state, "working")
		return c.agentResult("working"), c.identities(), nil
	case codexPhaseCompactContinuation:
		if err := c.writeUsage(c.opts.CompactedUsage); err != nil {
			return nil, c.identities(), err
		}
		c.phase = codexPhaseIdle
		c.setStatus(state, "idle")
		return c.agentResult("idle"), c.identities(), nil
	case codexPhaseContinuation:
		if err := c.writeUsage(c.opts.FinalUsage); err != nil {
			return nil, c.identities(), err
		}
		c.phase = codexPhaseFinalPending
		return nil, c.identities(), fmt.Errorf("timed out waiting for agent status")
	case codexPhaseFinalPending:
		c.phase = codexPhaseFinal
		c.setStatus(state, "idle")
		return c.agentResult("idle"), c.identities(), nil
	case codexPhaseIdle, codexPhaseFinal:
		return c.agentResult("idle"), c.identities(), nil
	default:
		return nil, c.identities(), fmt.Errorf("unexpected Codex phase %q", c.phase)
	}
}

func (c *CodexRollout) sendKeys(state *State, argv []string) (any, Identities, error) {
	if len(argv) < 4 || argv[2] != c.opts.PaneID {
		return nil, Identities{}, fmt.Errorf("unexpected Codex send-keys: %v", argv)
	}
	for _, key := range argv[3:] {
		if key == "ctrl+c" {
			c.phase = codexPhaseGenericBlocked
			c.setStatus(state, "blocked")
			break
		}
	}
	return nil, c.identities(), nil
}

func (c *CodexRollout) setStatus(state *State, status string) {
	if agent := state.Agents[c.opts.AgentID]; agent != nil {
		agent.Status = status
	}
}

func containsFinishUp(prompt string) bool {
	return len(prompt) >= len("finish up") && containsFold(prompt, "finish up")
}

func containsFold(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		matched := true
		for j := range substring {
			a, b := value[i+j], substring[j]
			if 'A' <= a && a <= 'Z' {
				a += 'a' - 'A'
			}
			if a != b {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (c *CodexRollout) agentResult(status string) map[string]any {
	return map[string]any{"agent": map[string]any{
		"pane_id": c.opts.PaneID, "workspace_id": c.opts.WorkspaceID,
		"tab_id": c.opts.TabID, "agent_status": status,
		"agent_session": map[string]any{"value": c.sessionID},
	}}
}

func (c *CodexRollout) identities() Identities {
	return Identities{
		WorkspaceID: c.opts.WorkspaceID, TabID: c.opts.TabID, PaneID: c.opts.PaneID,
		AgentID: c.opts.AgentID, SessionID: c.sessionID,
	}
}

func (c *CodexRollout) writeSessionMeta() error {
	return c.appendLine(map[string]any{
		"type":    "session_meta",
		"payload": map[string]any{"id": c.sessionID, "cwd": c.opts.Cwd},
	})
}

func (c *CodexRollout) writeUsage(usage CodexUsage) error {
	quota := func(value CodexQuota) map[string]any {
		resetsAt := int64(0)
		if !value.ResetsAt.IsZero() {
			resetsAt = value.ResetsAt.Unix()
		}
		return map[string]any{"used_percent": value.UsedPercent, "resets_at": resetsAt}
	}
	return c.appendLine(map[string]any{
		"type": "event_msg",
		"payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{
				"last_token_usage":  map[string]any{"input_tokens": usage.ContextTokens},
				"total_token_usage": map[string]any{"total_tokens": usage.TotalTokens},
			},
			"rate_limits": map[string]any{"primary": quota(usage.Primary), "secondary": quota(usage.Secondary)},
		},
	})
}

func (c *CodexRollout) appendLine(value map[string]any) error {
	value["timestamp"] = codexRolloutEpoch.Add(time.Duration(c.line) * time.Millisecond).Format(time.RFC3339Nano)
	c.line++
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func stableCodexSessionID(cwd, paneID, agentID string) string {
	sum := sha256.Sum256([]byte(cwd + "\x00" + paneID + "\x00" + agentID))
	sum[6] = sum[6]&0x0f | 0x40
	sum[8] = sum[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
