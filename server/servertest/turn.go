package servertest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/transcript"
)

// Turn is one agent turn: the `agent prompt` the server sent to a launched agent.
type Turn struct {
	Address string // the ticket the skill prompt names
	Cwd     string // the worktree the agent's tab was opened in
	Text    string // the whole prompt, notes included

	control   turnControl
	h         *Harness
	pty       bool   // the turn runs on PTY; the controls need it
	sessionID string // the agent's conversation id; set on PTY only
}

// newTurn builds the Turn for an `agent prompt` whose text names the ticket as
// its first argument, on either runner.
func newTurn(h *Harness, text, cwd, sessionID string) *Turn {
	_, rest, _ := strings.Cut(text, " ")
	addr, _, _ := strings.Cut(rest, "\n")
	addr = strings.TrimPrefix(addr, "--ticket ") // gx-merge's form
	return &Turn{Address: addr, Cwd: cwd, Text: text, h: h, pty: h.pty, sessionID: sessionID}
}

// unsupported fails the test, naming control, when the turn is on herdr, where
// the PTY-only controls cannot act.
func (t *Turn) unsupported(control string) bool {
	if !t.pty {
		t.h.tb.Errorf("servertest: Turn.%s is not supported on herdr; run this test on PTY (drop WithHerdr)", control)
	}
	return !t.pty
}

// turnControl is what a hook asked of the turn besides ending it. The harness
// applies it to the TUI fake once the prompt is in, since the hook runs before
// the prompt reaches the fake.
type turnControl struct {
	// busy keeps the turn open past the prompt instead of ending it at once.
	busy bool
	// after is sent once the prompt is in.
	after *agentfake.Control
}

// Stall makes the turn start and never end: the run stays working. PTY only.
func (t *Turn) Stall() {
	if !t.unsupported("Stall") {
		t.control = turnControl{busy: true}
	}
}

// Block makes the agent wait on a permission prompt for reason. PTY only.
func (t *Turn) Block(reason string) {
	if t.unsupported("Block") {
		return
	}
	t.control = turnControl{busy: true, after: &agentfake.Control{
		Action: agentfake.ActionBlock, Dialog: agentfake.BashPermissionDialog(reason),
	}}
}

// Exit makes the agent process exit with code, ending the session. PTY only.
func (t *Turn) Exit(code int) {
	if !t.unsupported("Exit") {
		t.control = turnControl{busy: true, after: &agentfake.Control{Action: agentfake.ActionExit, Code: code}}
	}
}

// Hold keeps the turn open until the returned release is called, which ends
// it. PTY only.
func (t *Turn) Hold() (release func()) {
	if t.unsupported("Hold") {
		return func() {}
	}
	t.control = turnControl{busy: true}
	return func() {
		err := agentfake.SendTUIControl(t.h.Env("HOME"), t.sessionID, agentfake.Control{Action: agentfake.ActionFinish})
		if err != nil {
			t.h.tb.Errorf("servertest: release the held turn: %v", err)
		}
	}
}

// Transcript appends an assistant line saying text to the agent's conversation
// transcript, where the server reads the agent's last words. PTY only.
func (t *Turn) Transcript(text string) {
	if t.unsupported("Transcript") {
		return
	}
	now := time.Now().UTC()
	line, _ := json.Marshal(map[string]any{
		"type": "assistant",
		// The chat read keys messages on uuid and orders them by timestamp.
		"uuid":      fmt.Sprintf("turn-%d", now.UnixNano()),
		"timestamp": now.Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	})
	// Recovery looks under HOME, the chat read under CLAUDE_CONFIG_DIR.
	for _, path := range []string{
		transcript.PathIn(t.h.Env("HOME"), t.Cwd, t.sessionID),
		filepath.Join(t.h.Env("CLAUDE_CONFIG_DIR"), "projects", transcript.Slugify(t.Cwd), t.sessionID+".jsonl"),
	} {
		if err := appendLine(path, append(line, '\n')); err != nil {
			t.h.tb.Errorf("servertest: write the transcript: %v", err)
		}
	}
}

func appendLine(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(line)
	return errors.Join(err, f.Close())
}

// ptyOnly reports whether the harness runs on PTY; on herdr it fails the test
// naming hook, as the turn controls do.
func (h *Harness) ptyOnly(hook string) bool {
	if !h.pty {
		h.tb.Errorf("servertest: Harness.%s is not supported on herdr; run this test on PTY (drop WithHerdr)", hook)
	}
	return h.pty
}

// FailNextStart makes the next agent start fail with err, so the launch fails
// the way a runner start error does. PTY only.
func (h *Harness) FailNextStart(err error) {
	if h.ptyOnly("FailNextStart") {
		h.failStart.Store(&err)
	}
}

// Stops is how many times the server stopped an agent. PTY only.
func (h *Harness) Stops() int {
	if !h.ptyOnly("Stops") {
		return 0
	}
	return int(h.stops.Load())
}

// BlockStops holds every agent stop, after it is counted, until release is
// called. The test's end releases it too. PTY only.
func (h *Harness) BlockStops() (release func()) {
	if !h.ptyOnly("BlockStops") {
		return func() {}
	}
	gate := make(chan struct{})
	h.stopGate.Store(&gate)
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	h.tb.Cleanup(release)
	return release
}

// AgentStart is one agent launch the server made.
type AgentStart struct {
	Label string // the agent's name, e.g. epic-a-iter-01
	Cwd   string // the worktree the agent started in
	Kind  string // the agent kind, e.g. claude
}

// Starts is every agent launch so far, in order, on PTY and on herdr (the
// latter once OnTurn answers its commands).
func (h *Harness) Starts() []AgentStart {
	h.startsMu.Lock()
	defer h.startsMu.Unlock()
	return slices.Clone(h.starts)
}

func (h *Harness) recordStart(s AgentStart) {
	h.startsMu.Lock()
	defer h.startsMu.Unlock()
	h.starts = append(h.starts, s)
}

// OnTurn runs agent for every turn (e.g. a commit in t.Cwd). The agent is idle
// again when the hook returns. On herdr it also answers the commands a launch
// makes, giving each tab its own pane and cwd.
func (h *Harness) OnTurn(agent func(t *Turn)) {
	if h.pty {
		h.turn.Store(&agent)
		return
	}
	// A test on its own runner never launches through herdr, so the hook would
	// never run and the test could pass without its script.
	if h.Herdr == nil {
		h.tb.Errorf("servertest: Harness.OnTurn needs PTY or WithHerdr")
		return
	}
	var mu sync.Mutex
	cwds := map[string]string{}  // pane -> cwd
	named := map[string]string{} // agent name -> pane
	turns := map[string]int{}    // pane -> state_change_seq
	tabs := 0
	reply := func(pane string) map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return map[string]any{"agent": map[string]any{
			"pane_id": pane, "tab_id": "t" + strings.TrimPrefix(pane, "p"),
			"agent_status": "idle", "state_change_seq": turns[pane],
		}}
	}
	ok := func() (any, herdrfake.Identities, error) { return map[string]any{}, herdrfake.Identities{}, nil }
	h.Herdr.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		pane, byName := named[argv[2]]
		_, byPane := cwds[argv[2]]
		mu.Unlock()
		switch {
		case byName:
			return reply(pane), herdrfake.Identities{}, nil
		case byPane:
			return reply(argv[2]), herdrfake.Identities{}, nil
		}
		return nil, herdrfake.Identities{}, AgentNotFound(argv[2])
	})
	h.Herdr.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "w1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "create", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		tabs++
		pane := "p" + strconv.Itoa(tabs)
		for i, a := range argv {
			if a == "--cwd" && i+1 < len(argv) {
				cwds[pane] = argv[i+1]
			}
		}
		return map[string]any{"tab": map[string]any{"tab_id": "t" + strconv.Itoa(tabs)}, "root_pane": map[string]any{"pane_id": pane}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		// Closing the tab ends the agent in its pane, so a relaunch starts fresh.
		pane := "p" + strings.TrimPrefix(argv[2], "t")
		mu.Lock()
		defer mu.Unlock()
		delete(cwds, pane)
		for name, p := range named {
			if p == pane {
				delete(named, name)
			}
		}
		return ok()
	})
	h.Herdr.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		pane := flagValue(argv, "--pane")
		mu.Lock()
		named[argv[2]] = pane
		cwd := cwds[pane]
		mu.Unlock()
		h.recordStart(AgentStart{Label: argv[2], Cwd: cwd, Kind: flagValue(argv, "--kind")})
		return reply(pane), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return reply(argv[2]), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		pane, text := argv[2], argv[3]
		mu.Lock()
		cwd := cwds[pane]
		turns[pane]++ // the prompt starts a turn
		mu.Unlock()
		agent(newTurn(h, text, cwd, ""))
		return reply(pane), herdrfake.Identities{}, nil
	})
}

// AgentNotFound is the error a fake `herdr agent get` returns for an unknown target.
func AgentNotFound(target string) error {
	return errors.New(`{"error":{"code":"agent_not_found","message":"agent target ` + target + ` not found"}}`)
}

func flagValue(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}
