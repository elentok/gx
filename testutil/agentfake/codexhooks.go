package agentfake

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/codexui"
)

// The four Codex startup dialogs a launch can show before it is ready. A knob
// set to "1" shows its dialog; each waits for a key. No stream recorded one, so
// the text only carries the dialog's name.
const (
	CodexTrustEnv       = "GX_AGENTFAKE_CODEX_TRUST"
	CodexHooksReviewEnv = "GX_AGENTFAKE_CODEX_HOOKS_REVIEW"
	CodexUpdateEnv      = "GX_AGENTFAKE_CODEX_UPDATE"
	CodexModelNudgeEnv  = "GX_AGENTFAKE_CODEX_MODEL_NUDGE"

	CodexTrustDialog       = "Trust this directory? Press Enter to continue"
	CodexHooksReviewDialog = "Hooks review: Press Enter to continue"
	CodexUpdateDialog      = "Update available: Press Enter to continue"
	CodexModelNudgeDialog  = "Model nudge: Press Enter to continue"
)

// codexDialogs lists the dialogs the env asks for, in the order Codex shows them.
func codexDialogs() []string {
	var out []string
	for _, d := range []struct{ env, text string }{
		{CodexTrustEnv, CodexTrustDialog},
		{CodexHooksReviewEnv, CodexHooksReviewDialog},
		{CodexUpdateEnv, CodexUpdateDialog},
		{CodexModelNudgeEnv, CodexModelNudgeDialog},
	} {
		if os.Getenv(d.env) == "1" {
			out = append(out, d.text)
		}
	}
	return out
}

// hookDenyExit is the exit code of a Codex hook that blocks or denies.
const hookDenyExit = 2

// CodexHookPayload is the JSON a Codex hook reads on stdin. The field names
// come from Codex's hook documentation; no recorded fixture holds one.
type CodexHookPayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name,omitempty"`
	ToolInput      *CodexToolInput `json:"tool_input,omitempty"`
}

// CodexToolInput is a Bash tool call's input.
type CodexToolInput struct {
	Command string `json:"command"`
}

// CodexHookRun is one hook command the fake ran, as logged for a test.
type CodexHookRun struct {
	Event   string           `json:"event"`
	Command string           `json:"command"`
	Payload CodexHookPayload `json:"payload"`
	Stdout  string           `json:"stdout"`
	Stderr  string           `json:"stderr"`
	Exit    int              `json:"exit"`
}

// CodexHookLogPath holds one JSON CodexHookRun per line for the session.
func CodexHookLogPath(codexHome, sessionID string) string {
	return filepath.Join(codexHome, "agentfake", sessionID+".hooks")
}

// ReadCodexHookLog is every hook run the Codex fake running sessionID logged.
func ReadCodexHookLog(codexHome, sessionID string) ([]CodexHookRun, error) {
	data, err := os.ReadFile(CodexHookLogPath(codexHome, sessionID))
	if err != nil {
		return nil, err
	}
	var runs []CodexHookRun
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var r CodexHookRun
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

type codexHook struct{ matcher, command string }

// parseCodexHooks reads the value of a `-c hooks.<Event>=[...]` argument. It
// knows only the shape gx writes: inline groups with an optional matcher and
// command strings, and takes no other TOML.
func parseCodexHooks(value string) ([]codexHook, error) {
	var hooks []codexHook
	depth, matcher := 0, ""
	for i := 0; i < len(value); {
		switch rest := value[i:]; {
		case rest[0] == '"':
			_, n, err := readTOMLString(rest)
			if err != nil {
				return nil, err
			}
			i += n
		case rest[0] == '{':
			depth++
			if depth == 1 {
				matcher = ""
			}
			i++
		case rest[0] == '}':
			depth--
			i++
		case strings.HasPrefix(rest, "matcher=") || strings.HasPrefix(rest, "command="):
			key, str := rest[:strings.IndexByte(rest, '=')], rest[strings.IndexByte(rest, '=')+1:]
			s, n, err := readTOMLString(str)
			if err != nil {
				return nil, err
			}
			if key == "matcher" {
				matcher = s
			} else {
				hooks = append(hooks, codexHook{matcher, s})
			}
			i += len(key) + 1 + n
		default:
			i++
		}
	}
	return hooks, nil
}

// readTOMLString reads the basic string at the start of s and returns it with
// the bytes it took, quotes included.
func readTOMLString(s string) (string, int, error) {
	if s == "" || s[0] != '"' {
		return "", 0, fmt.Errorf("fakecodex: expected a TOML string at %q", s)
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return b.String(), i + 1, nil
		case '\\':
			i++
			if i == len(s) {
				return "", 0, errors.New("fakecodex: bad TOML escape")
			}
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, errors.New("fakecodex: unterminated TOML string")
}

// payload is the stdin of a hook for event; tool is empty outside tool events.
func (c *codexFake) payload(event, tool, command string) CodexHookPayload {
	p := CodexHookPayload{
		SessionID:      c.id,
		TranscriptPath: c.rollout,
		Cwd:            c.cwd,
		HookEventName:  event,
		ToolName:       tool,
	}
	if command != "" {
		p.ToolInput = &CodexToolInput{Command: command}
	}
	return p
}

// runHooks runs each hook of event whose matcher fits tool, logs the runs and
// returns them. A command's own failure is a result, not an error.
func (c *codexFake) runHooks(event, tool string, payload CodexHookPayload) ([]CodexHookRun, error) {
	stdin, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var runs []CodexHookRun
	for _, h := range c.hooks[event] {
		if h.matcher != "" {
			ok, err := regexp.MatchString("^(?:"+h.matcher+")$", tool)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		cmd := exec.Command("sh", "-c", h.command)
		cmd.Dir = c.cwd
		cmd.Stdin = bytes.NewReader(stdin)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		run := CodexHookRun{Event: event, Command: h.command, Payload: payload}
		var exit *exec.ExitError
		if err := cmd.Run(); errors.As(err, &exit) {
			run.Exit = exit.ExitCode()
		} else if err != nil {
			return nil, err
		}
		run.Stdout, run.Stderr = stdout.String(), stderr.String()
		if err := c.logHook(run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (c *codexFake) logHook(run CodexHookRun) error {
	line, err := json.Marshal(run)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(CodexHookLogPath(c.home, c.id), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// denial reports whether a PermissionRequest's runs deny the call, and why: an
// exit 2 or a deny decision in a command's stdout JSON.
func denial(runs []CodexHookRun) (reason string, denied bool) {
	for _, r := range runs {
		var out struct {
			Output struct {
				Decision struct {
					Behavior string `json:"behavior"`
					Message  string `json:"message"`
				} `json:"decision"`
			} `json:"hookSpecificOutput"`
		}
		if json.Unmarshal([]byte(r.Stdout), &out) == nil && out.Output.Decision.Behavior == "deny" {
			return cmp.Or(out.Output.Decision.Message, strings.TrimSpace(r.Stderr)), true
		}
		if r.Exit == hookDenyExit {
			return strings.TrimSpace(r.Stderr), true
		}
	}
	return "", false
}

func (c *codexFake) sessionStart() error {
	_, err := c.runHooks("SessionStart", "", c.payload("SessionStart", "", ""))
	return err
}

// toolCall runs a tool through its PreToolUse hooks. An exit 2 blocks it and
// the hook's stderr lands on the screen.
func (c *codexFake) toolCall(tool, command string) error {
	if tool == "" {
		tool = "Bash"
	}
	runs, err := c.runHooks("PreToolUse", tool, c.payload("PreToolUse", tool, command))
	if err != nil {
		return err
	}
	for _, r := range runs {
		if r.Exit == hookDenyExit {
			fmt.Fprint(c.out, strings.TrimSpace(r.Stderr)+"\r\n")
			return nil
		}
	}
	fmt.Fprint(c.out, "ran: "+command+"\r\n")
	args, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return err
	}
	return c.writeLine("response_item", map[string]any{"type": "function_call", "name": tool, "arguments": string(args)})
}

// permission asks for approval to run command and calls allowed once it is
// given. The fake stays blocked while the PermissionRequest hooks run; with
// none configured it stays blocked until a key arrives. A deny ends the turn.
func (c *codexFake) permission(command string, allowed func()) error {
	writeAppReport(c.out, codexAppName, "blocked", "permission", "")
	resume := func() {
		c.report("working")
		if allowed != nil {
			allowed()
		}
	}
	if len(c.hooks["PermissionRequest"]) == 0 {
		// The dialog Codex 0.166.0 draws when no hook answered, per the spec's
		// text; the keys are codexui's, the one place that names them.
		fmt.Fprint(c.out, "\r\n"+codexui.Title+"\r\n"+command+"\r\n")
		var wait func(key byte) error
		wait = func(key byte) error {
			switch string(key) {
			case codexui.AllowKey:
				resume()
				return nil
			case codexui.DenyKey:
				fmt.Fprint(c.out, "denied by the person\r\n")
				return c.finishTurn()
			}
			c.onKey = wait
			return nil
		}
		c.onKey = wait
		return nil
	}
	runs, err := c.runHooks("PermissionRequest", "Bash", c.payload("PermissionRequest", "Bash", command))
	if err != nil {
		return err
	}
	if reason, denied := denial(runs); denied {
		fmt.Fprint(c.out, "denied: "+reason+"\r\n")
		return c.finishTurn()
	}
	resume()
	return nil
}

func (c *codexFake) stopHooks() error {
	_, err := c.runHooks("Stop", "", c.payload("Stop", "", ""))
	return err
}

// tokenCount writes a rate-limit sample for the primary window.
func (c *codexFake) tokenCount(usedPercent float64, resetsAt int64) error {
	if resetsAt == 0 {
		resetsAt = time.Now().Add(codexRateWindowMins * time.Minute).Unix()
	}
	return c.writeLine("event_msg", map[string]any{
		"type": "token_count",
		"rate_limits": map[string]any{"primary": map[string]any{
			"used_percent":   usedPercent,
			"window_minutes": codexRateWindowMins,
			"resets_at":      resetsAt,
		}},
	})
}

// compact writes the line Codex records when it compacts the context.
func (c *codexFake) compact() error {
	return c.writeLine("compacted", map[string]any{
		"message":             "",
		"replacement_history": []any{c.message("user", "input_text", "summary of the session")},
	})
}

// taskError ends the turn with an error, as Codex does for a bad model or a
// hit limit. The error state stays until the next turn, as in the recorded
// stream: there is no idle report after it.
func (c *codexFake) taskError(message, info string) error {
	c.working = false
	if err := c.writeLine("event_msg", map[string]any{
		"type":    "task_complete",
		"turn_id": c.turnID(),
		"error":   map[string]any{"message": message, "codex_error_info": info},
	}); err != nil {
		return err
	}
	c.setTitle(c.title)
	writeAppReport(c.out, codexAppName, "error", "", "")
	return nil
}

// workerPrompt reports whether text is a skill prompt (`$gx-… <address>`) for a
// fake that runs in a worktree carrying a worker config.
func (c *codexFake) workerPrompt(text string) (string, WorkerConfig, bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "$gx-") || fields[0] == "$gx-investigate" {
		return "", WorkerConfig{}, false
	}
	cfg, err := LoadWorkerConfig(c.cwd)
	if err != nil {
		return "", WorkerConfig{}, false
	}
	return fields[len(fields)-1], cfg, true
}

// submit starts the turn a typed prompt asks for. A skill prompt does the
// ticket's work as the worker config says.
func (c *codexFake) submit(text string) error {
	if w := c.resumeWork; w != nil {
		c.resumeWork = nil
		if err := c.startTurn(text); err != nil {
			return err
		}
		go c.work(w.cfg, w.address)
		return nil
	}
	if isDesignPrompt(text) || (c.design.active && !strings.HasPrefix(text, "$")) {
		return c.designTurn(text)
	}
	address, cfg, ok := c.workerPrompt(text)
	if !ok {
		return c.startTurn(text)
	}
	if err := c.startTurn(text); err != nil {
		return err
	}
	switch cfg.Behaviour[address] {
	case BehaviourPermission:
		return c.permission(PermissionCommand, func() { go c.work(cfg, address) })
	case BehaviourRateLimit:
		c.resumeWork = &pendingWork{cfg, address}
		reset := DefaultRateLimitReset
		if d, err := time.ParseDuration(cfg.RateLimitReset); err == nil {
			reset = d
		}
		if err := c.tokenCount(100, time.Now().Add(reset).Unix()); err != nil {
			return err
		}
		return c.taskError("You've hit your usage limit.", "usage_limit_exceeded")
	}
	if u := cfg.CodexUsage; u != nil {
		if err := c.tokenCount(u.UsedPercentage, u.ResetsAt); err != nil {
			return err
		}
	}
	go c.work(cfg, address)
	return nil
}

// designTurn plays the next scripted gx-design turn: its message is the reply
// the turn ends with, a moment after it starts.
func (c *codexFake) designTurn(text string) error {
	script := c.design.turnScript(c.cwd, text)
	if isDesignPrompt(text) {
		c.design.active = true
	}
	busy := designTurnTime
	if c.design.turns < len(script) && script[c.design.turns] == DesignTurnSlow {
		busy = designSlowTime
	}
	reply := c.design.message(text, script)
	if err := c.startTurn(text); err != nil {
		return err
	}
	c.reply = reply
	go func() {
		time.Sleep(busy)
		_ = SendControl(c.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}

// work does the ticket's work off the input loop, then ends the turn down the
// control channel like a test would.
func (c *codexFake) work(cfg WorkerConfig, address string) {
	if err := WorkTicket(cfg, address); err != nil {
		fmt.Fprintln(os.Stderr, "fakecodex worker:", err)
	}
	if cfg.Behaviour[address] == BehaviourFail {
		_ = SendControl(c.ctlPath, Control{Action: ActionExit, Code: 1})
		return
	}
	_ = SendControl(c.ctlPath, Control{Action: ActionFinish})
}
