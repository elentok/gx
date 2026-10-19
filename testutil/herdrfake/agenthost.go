package herdrfake

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// RegisterAgentHost installs the workspace/tab/agent commands an
// agentrunner adapter drives, over a minimal agent model: an agent is idle
// after start, working after a prompt, and blocked when a test says so
// (SetAgentStatus). StateChangeSeq advances on every status change, like
// herdr's. "agent wait" never blocks: it answers at once or reports a timeout,
// so callers poll.
func RegisterAgentHost(s *State) {
	s.Register("workspace", "list", func(s *State, _ []string) (any, Identities, error) {
		type ws struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		}
		var out []ws
		for _, w := range sortedValues(s.Workspaces) {
			out = append(out, ws{w.ID, w.Label})
		}
		return map[string]any{"workspaces": out}, Identities{}, nil
	})
	s.Register("workspace", "create", func(s *State, argv []string) (any, Identities, error) {
		w := &Workspace{ID: s.nextID("w", len(s.Workspaces)), Label: flag(argv, "--label"), Cwd: flag(argv, "--cwd")}
		s.Workspaces[w.ID] = w
		return map[string]any{"workspace": map[string]string{"workspace_id": w.ID}}, Identities{WorkspaceID: w.ID}, nil
	})
	s.Register("tab", "create", func(s *State, argv []string) (any, Identities, error) {
		wsID := flag(argv, "--workspace")
		if s.Workspaces[wsID] == nil {
			return nil, Identities{}, fmt.Errorf("workspace not found: %s", wsID)
		}
		tab := &Tab{ID: s.nextID("t", len(s.Tabs)), WorkspaceID: wsID, Label: flag(argv, "--label"), Number: len(s.Tabs) + 1}
		pane := &Pane{ID: s.nextID("p", len(s.Panes)), TabID: tab.ID, Cwd: flag(argv, "--cwd")}
		s.Tabs[tab.ID] = tab
		s.Panes[pane.ID] = pane
		return map[string]any{"tab": tabJSON(tab), "root_pane": map[string]string{"pane_id": pane.ID}},
			Identities{WorkspaceID: wsID, TabID: tab.ID, PaneID: pane.ID}, nil
	})
	s.Register("tab", "list", func(s *State, argv []string) (any, Identities, error) {
		wsID := flag(argv, "--workspace")
		tabs := []map[string]any{}
		for _, tab := range sortedValues(s.Tabs) {
			if wsID == "" || tab.WorkspaceID == wsID {
				tabs = append(tabs, tabJSON(tab))
			}
		}
		return map[string]any{"tabs": tabs}, Identities{WorkspaceID: wsID}, nil
	})
	s.Register("tab", "close", func(s *State, argv []string) (any, Identities, error) {
		id := argv[2]
		if s.Tabs[id] == nil {
			return nil, Identities{}, fmt.Errorf("tab not found: %s", id)
		}
		delete(s.Tabs, id)
		for pid, p := range s.Panes {
			if p.TabID == id {
				delete(s.Panes, pid)
				for aid, a := range s.Agents {
					if a.PaneID == pid {
						delete(s.Agents, aid)
					}
				}
			}
		}
		return map[string]any{}, Identities{TabID: id}, nil
	})
	s.Register("agent", "start", func(s *State, argv []string) (any, Identities, error) {
		name, paneID := argv[2], flag(argv, "--pane")
		if a := s.findAgent(name); a != nil {
			p := s.Panes[a.PaneID]
			return nil, Identities{}, errorEnvelope("agent_name_taken",
				fmt.Sprintf("agent name %s is taken; candidates: pane_id=%s cwd=%s status=%s", name, p.ID, p.Cwd, a.Status))
		}
		if s.Panes[paneID] == nil {
			return nil, Identities{}, fmt.Errorf("pane not found: %s", paneID)
		}
		a := &Agent{ID: s.nextID("a", len(s.Agents)), PaneID: paneID, Name: name, Kind: flag(argv, "--kind"), Status: "idle"}
		s.Agents[a.ID] = a
		return s.agentJSON(a), Identities{PaneID: paneID, AgentID: a.ID}, nil
	})
	s.Register("agent", "list", func(s *State, _ []string) (any, Identities, error) {
		agents := []map[string]any{}
		for _, a := range sortedValues(s.Agents) {
			agents = append(agents, s.agentInfo(a))
		}
		return map[string]any{"agents": agents}, Identities{}, nil
	})
	s.Register("agent", "get", func(s *State, argv []string) (any, Identities, error) {
		a, err := s.targetAgent(argv)
		if err != nil {
			return nil, Identities{}, err
		}
		return s.agentJSON(a), Identities{AgentID: a.ID}, nil
	})
	s.Register("agent", "wait", func(s *State, argv []string) (any, Identities, error) {
		a, err := s.targetAgent(argv)
		if err != nil {
			return nil, Identities{}, err
		}
		until := flags(argv, "--until")
		if len(until) == 0 {
			until = []string{"idle", "done", "blocked"}
		}
		if !slices.Contains(until, a.Status) {
			return nil, Identities{AgentID: a.ID}, fmt.Errorf("timed out waiting for %s", a.Name)
		}
		return s.agentJSON(a), Identities{AgentID: a.ID}, nil
	})
	s.Register("agent", "prompt", func(s *State, argv []string) (any, Identities, error) {
		a, err := s.targetAgent(argv)
		if err != nil {
			return nil, Identities{}, err
		}
		if a.Status == "blocked" {
			return nil, Identities{AgentID: a.ID}, AgentBlockedError(argv[2])
		}
		if a.Stalled {
			return nil, Identities{AgentID: a.ID}, errorEnvelope("agent_prompt_stalled", "no observed state change after prompt")
		}
		a.Received = append(a.Received, argv[3])
		switch text := argv[3]; {
		case a.FastNext:
			// The turn starts and ends between two polls: the seq moves but no
			// status edge is ever visible, so herdr's wait stalls.
			a.FastNext = false
			a.Seq += 2
			return nil, Identities{AgentID: a.ID}, errorEnvelope("agent_prompt_stalled", "no observed state change after prompt")
		case a.ConfirmsCompact && text == "/compact":
			a.setStatus("blocked", "compact_confirm")
		case strings.HasPrefix(text, "/") && text != "/compact":
			a.slashCommand(text)
		default:
			a.setStatus("working", "")
		}
		return s.agentJSON(a), Identities{AgentID: a.ID}, nil
	})
	s.Register("agent", "send-keys", func(s *State, argv []string) (any, Identities, error) {
		a, err := s.targetAgent(argv)
		if err != nil {
			return nil, Identities{}, err
		}
		s.deliverKeys(a, argv[3:])
		return map[string]any{}, Identities{AgentID: a.ID}, nil
	})
}

// rawHandler wraps h so "agent explain" (a bare JSON object) and "agent read"
// (raw pane text), which skip herdr's {"result": ...} envelope, answer from s
// directly.
func (s *State) rawHandler(h Handler) Handler {
	return func(argv []string) ([]byte, int) {
		if len(argv) < 3 || argv[0] != "agent" || (argv[1] != "explain" && argv[1] != "read") {
			return h(argv)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		a, err := s.targetAgent(argv)
		if err != nil {
			return CommandError(err.Error())
		}
		if argv[1] == "read" {
			// Prompts are not typed into the pane; only SetPaneText changes it.
			if a.PaneText == "" {
				return []byte("> "), 0
			}
			return []byte(a.PaneText), 0
		}
		resp := map[string]any{"state": a.Status}
		if a.Rule != "" {
			resp["matched_rule"] = map[string]string{"id": a.Rule}
		}
		b, _ := json.Marshal(resp)
		return b, 0
	}
}

// StartAgentHost registers the agent host commands on s and starts it like
// StartState.
func StartAgentHost(t *testing.T, s *State) *Coordinator {
	t.Helper()
	RegisterAgentHost(s)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go s.runWatchdog(stop)
	return Start(t, s.rawHandler(s.Handler()))
}

// SetAgentStatus sets the status of the agent named name, as the agent
// itself would by finishing a turn or raising a dialog. rule is the
// detection rule "agent explain" reports, for a blocked agent.
func (s *State) SetAgentStatus(name, status, rule string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	a.setStatus(status, rule)
	return nil
}

// StallAgent makes every later prompt to the agent named name fail with
// herdr's agent_prompt_stalled error, as for a submission that never reaches
// the pane. It also ignores Ctrl+C and Esc, so an interrupted turn never ends
// and a dialog stays open.
func (s *State) StallAgent(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	a.Stalled = true
	return nil
}

// ConfirmCompact makes the agent named name block on a "/compact" prompt, as
// Codex does while it asks to confirm. SetAgentStatus moves it on.
func (s *State) ConfirmCompact(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	a.ConfirmsCompact = true
	return nil
}

// SetPaneText sets what "agent read" returns for the agent named name, e.g.
// a rate-limit message.
func (s *State) SetPaneText(name, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	a.PaneText = text
	return nil
}

// Received returns the prompts the agent named name has received, in order.
func (s *State) Received(name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return slices.Clone(a.Received), nil
}

// FastTurn makes the next prompt to the agent named name start and end a turn
// with no status edge a poll could see.
func (s *State) FastTurn(name string) error {
	return s.withAgent(name, func(a *Agent) { a.FastNext = true })
}

// OpenDialogOnNextSlash makes the agent named name open a dialog on the next
// "/" command it receives, as /help does.
func (s *State) OpenDialogOnNextSlash(name string) error {
	return s.withAgent(name, func(a *Agent) { a.DialogNext = true })
}

// Keys returns every key the agent named name has received, in order.
func (s *State) Keys(name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return slices.Clone(a.Keys), nil
}

// SetInput sets the text on the input line of the agent named name, as typing
// would. Ctrl+C on an idle claude clears it; Ctrl+U clears its last line.
func (s *State) SetInput(name, text string) error {
	return s.withAgent(name, func(a *Agent) { a.Input = text })
}

// Input returns the text on the input line of the agent named name.
func (s *State) Input(name string) (string, error) {
	var text string
	err := s.withAgent(name, func(a *Agent) { text = a.Input })
	return text, err
}

// VanishAfterKey makes the agent named name disappear once it receives key, so
// later commands against it fail with agent_not_found.
func (s *State) VanishAfterKey(name, key string) error {
	return s.withAgent(name, func(a *Agent) { a.VanishOnKey = key })
}

// deliverKeys records keys and applies them to a, as claude (and, for Kind
// "codex", Codex) handles them. It reports whether the agent quit; a quit
// agent is gone from State. Must be called with s.mu held.
func (s *State) deliverKeys(a *Agent, keys []string) (quit bool) {
	codex := a.Kind == "codex"
	for _, key := range keys {
		a.Keys = append(a.Keys, key)
		switch {
		case a.Stalled && (key == "ctrl+c" || key == "esc"):
			// A stalled agent stays busy through an interrupt and keeps its
			// dialog open on Esc.
		case a.Status == "blocked" && key == "esc":
			a.setStatus("idle", "")
		case a.Status == "blocked" && key == "ctrl+c" && codex:
			a.setStatus("idle", "")
		case a.Status == "blocked" && !codex:
			// Any other answer dismisses the dialog and the turn carries on.
			a.setStatus("working", "")
		case key == "ctrl+c" && a.Status == "working":
			if codex {
				a.setStatus("done", "")
			} else {
				a.setStatus("idle", "")
			}
		case key == "ctrl+c" && a.Status == "idle" && codex:
			s.removeAgent(a)
			return true
		case key == "ctrl+c" && a.Status == "idle":
			a.Input = ""
		case key == "ctrl+u":
			// One line per press, as claude and Codex do.
			a.Input = a.Input[:max(strings.LastIndex(a.Input, "\n"), 0)]
		}
		if a.VanishOnKey == key {
			s.removeAgent(a)
			return true
		}
	}
	return false
}

func (s *State) removeAgent(a *Agent) {
	delete(s.Agents, a.ID)
}

func (s *State) withAgent(name string, f func(*Agent)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	f(a)
	return nil
}

// slashCommand handles a "/" command as the agents do: it starts no turn.
// /clear and /new start a new conversation, which moves the seq; a dialog
// command leaves the agent blocked.
func (a *Agent) slashCommand(text string) {
	switch {
	case a.DialogNext:
		a.DialogNext = false
		a.setStatus("blocked", "input_needed")
	case text == "/clear" || text == "/new":
		a.Seq++
	}
}

func (a *Agent) setStatus(status, rule string) {
	if a.Status != status {
		a.Seq++
	}
	a.Status, a.Rule = status, rule
}

func (s *State) findAgent(target string) *Agent {
	for _, a := range sortedValues(s.Agents) {
		if a.Name == target || a.PaneID == target {
			return a
		}
	}
	return nil
}

func (s *State) targetAgent(argv []string) (*Agent, error) {
	if len(argv) < 3 {
		return nil, fmt.Errorf("missing agent target")
	}
	if a := s.findAgent(argv[2]); a != nil {
		return a, nil
	}
	return nil, agentNotFound(argv[2])
}

func agentNotFound(target string) error {
	return errorEnvelope("agent_not_found", fmt.Sprintf("agent target %s not found", target))
}

func (s *State) agentJSON(a *Agent) map[string]any {
	return map[string]any{"agent": s.agentInfo(a)}
}

func (s *State) agentInfo(a *Agent) map[string]any {
	p := s.Panes[a.PaneID]
	tab := s.Tabs[p.TabID]
	return map[string]any{
		"name":             a.Name,
		"pane_id":          a.PaneID,
		"workspace_id":     tab.WorkspaceID,
		"tab_id":           tab.ID,
		"agent_status":     a.Status,
		"state_change_seq": a.Seq,
	}
}

func tabJSON(t *Tab) map[string]any {
	return map[string]any{"tab_id": t.ID, "workspace_id": t.WorkspaceID, "number": t.Number, "label": t.Label}
}

// nextID mints an id that is never reused, even after deletes, by skipping
// taken ones.
func (s *State) nextID(prefix string, n int) string {
	for i := n + 1; ; i++ {
		id := prefix + strconv.Itoa(i)
		if s.Workspaces[id] == nil && s.Tabs[id] == nil && s.Panes[id] == nil && s.Agents[id] == nil {
			return id
		}
	}
}

func flag(argv []string, name string) string {
	if v := flags(argv, name); len(v) > 0 {
		return v[0]
	}
	return ""
}

func flags(argv []string, name string) []string {
	var out []string
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--" {
			break
		}
		if argv[i] == name {
			out = append(out, argv[i+1])
		}
	}
	return out
}

func sortedValues[T any](m map[string]*T) []*T {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]*T, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func errorEnvelope(code, msg string) error {
	b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": msg}})
	return errorString(b)
}
