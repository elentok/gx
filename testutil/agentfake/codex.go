package agentfake

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/harness"
)

// CodexEnv, set to "1", makes a test binary that calls TUICodex from its
// TestMain act as interactive Codex on a PTY.
const CodexEnv = "GX_AGENTFAKE_CODEX"

// CodexIDEnv, when not empty, is the session id a fresh fake launch takes
// instead of a random one. A test needs it to address the fake's control FIFO,
// since real Codex has no --session-id to pass.
const CodexIDEnv = "GX_AGENTFAKE_CODEX_ID"

// CodexVersion is the Codex release the fake imitates; a test fixes it.
const CodexVersion = "0.166.0"

// codexMarker is Codex's bold input marker.
const codexMarker = "\x1b[1m›\x1b[0m "

const (
	// codexAppName is the app= value Codex's OSC 7501 reports carry.
	codexAppName = "codex"
	// codexSpinner is one frame of the title spinner shown while working.
	codexSpinner = "⠋"
	// Token figures a turn reports: the context grows by this much per turn.
	codexTurnTokens     = 1000
	codexContextWindow  = 475000
	codexReply          = "done"
	codexRateWindowMins = 10080
)

// CodexArgsPath holds the argv (a JSON array) the Codex fake running sessionID
// was started with.
func CodexArgsPath(codexHome, sessionID string) string {
	return filepath.Join(codexHome, "agentfake", sessionID+".args")
}

// ReadCodexArgs is the argv the Codex fake running sessionID was started with.
func ReadCodexArgs(codexHome, sessionID string) ([]string, error) {
	data, err := os.ReadFile(CodexArgsPath(codexHome, sessionID))
	if err != nil {
		return nil, err
	}
	var args []string
	return args, json.Unmarshal(data, &args)
}

// TUICodex runs the fake interactive Codex on the process's terminal and exits
// with the code an exit control asked for.
func TUICodex() {
	code, err := runCodex()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type codexOptions struct {
	// resume is the id after "resume"; empty for a fresh launch.
	resume string
	// hooks are the inline `-c hooks.<Event>=...` commands by event.
	hooks map[string][]codexHook
}

// parseCodexArgs accepts exactly the launch flags the Codex harness passes, so
// a new flag fails a test instead of passing unseen.
func parseCodexArgs(args []string) (codexOptions, error) {
	var opts codexOptions
	if len(args) > 0 && args[0] == "resume" {
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return opts, errors.New("fakecodex: resume needs a session id")
		}
		opts.resume, args = args[1], args[2:]
	}
	flags := flag.NewFlagSet("codex", flag.ContinueOnError)
	for _, name := range []string{"sandbox", "s", "ask-for-approval", "a", "model", "m", "cd", "C"} {
		flags.String(name, "", "")
	}
	for _, name := range []string{"approve-for-me", "dangerously-bypass-hook-trust"} {
		flags.Bool(name, false, "")
	}
	flags.Func("c", "", func(setting string) error {
		key, value, _ := strings.Cut(setting, "=")
		event, isHook := strings.CutPrefix(key, "hooks.")
		if !isHook {
			return nil
		}
		hooks, err := parseCodexHooks(value)
		if opts.hooks == nil {
			opts.hooks = map[string][]codexHook{}
		}
		opts.hooks[event] = append(opts.hooks[event], hooks...)
		return err
	})
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	// Real Codex rejects these with --approve-for-me; keep the fake as strict so CI sees it.
	conflict := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "sandbox", "s", "ask-for-approval", "a":
			conflict = true
		}
	})
	if conflict && flags.Lookup("approve-for-me").Value.String() == "true" {
		return opts, errors.New("--sandbox and --ask-for-approval cannot be used with '--approve-for-me'")
	}
	if flags.NArg() > 0 {
		return opts, fmt.Errorf("fakecodex: unexpected argument %q", flags.Arg(0))
	}
	return opts, nil
}

// codexHome is Codex's home as the child reads it: CODEX_HOME, else ~/.codex.
func codexHome() (string, error) {
	env := os.Environ()
	if dir := harness.LookupEnv(env, "CODEX_HOME"); dir != "" {
		return dir, nil
	}
	home := harness.LookupEnv(env, "HOME")
	if home == "" {
		return "", errors.New("fakecodex: neither CODEX_HOME nor HOME is set")
	}
	return filepath.Join(home, ".codex"), nil
}

func runCodex() (int, error) {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("codex-cli " + CodexVersion)
		return 0, nil
	}
	opts, err := parseCodexArgs(args)
	if err != nil {
		return 0, err
	}
	if term.IsTerminal(os.Stdin.Fd()) {
		if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
			return 0, err
		}
	}
	home, err := codexHome()
	if err != nil {
		return 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	id := cmp.Or(opts.resume, os.Getenv(CodexIDEnv), fmt.Sprintf("fakecodex-%d", time.Now().UnixNano()))
	argv, err := json.Marshal(args)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(CodexArgsPath(home, id)), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(CodexArgsPath(home, id), argv, 0o644); err != nil {
		return 0, err
	}

	dir := programDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	// The generic fake Watch finds a program's conversation by pid.
	convPath := programConversationPath(dir, os.Getpid())
	if err := os.WriteFile(convPath, []byte(id), 0o644); err != nil {
		return 0, err
	}
	defer os.Remove(convPath)
	ctl, closeCtl, err := openProgramChannel(dir, id)
	if err != nil {
		return 0, err
	}
	defer closeCtl()

	c := &codexFake{
		out:     os.Stdout,
		id:      id,
		home:    home,
		cwd:     cwd,
		title:   filepath.Base(cwd),
		box:     newInputBox(os.Stdout, ""),
		hooks:   opts.hooks,
		dialogs: codexDialogs(),
		ctlPath: filepath.Join(dir, id+".ctl"),
	}
	c.box.marker = codexMarker
	if err := c.openRollout(home, opts.resume != ""); err != nil {
		return 0, err
	}
	return c.run(os.Stdin, ctl)
}

type codexFake struct {
	out   io.Writer
	id    string
	home  string
	cwd   string
	title string
	box   *inputBox
	hooks map[string][]codexHook
	// dialogs are the startup dialogs still waiting for a key; ctlPath is this
	// fake's own control channel, for work that runs off the input loop.
	dialogs []string
	ctlPath string
	// onKey, when set, takes the next key instead of the input box.
	onKey func(key byte) error
	// resumeWork is a ticket whose first prompt ended in a rate-limit error; the
	// next prompt does its work.
	resumeWork *pendingWork
	// design plays a gx-design session's scripted turns; reply is the assistant
	// message the running turn ends with, empty for the default.
	design designState
	reply  string
	// rollout is the session file; ordinal numbers its next line.
	rollout string
	ordinal int
	// turns counts the turns this session ran, including resumed ones.
	turns   int
	working bool
	prompt  string
	// background is a background terminal still running; input is the key layer.
	background bool
	input      codexInput
}

// openRollout finds the session's rollout under home, or creates it with its
// session_meta line. A resumed session appends to the file it already has.
func (c *codexFake) openRollout(home string, resume bool) error {
	sessions := filepath.Join(home, "sessions")
	if resume {
		err := filepath.WalkDir(sessions, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !e.IsDir() && c.rollout == "" && strings.HasSuffix(e.Name(), ".jsonl") && strings.Contains(e.Name(), c.id) {
				c.rollout = path
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if c.rollout != "" {
		data, err := os.ReadFile(c.rollout)
		if err != nil {
			return err
		}
		c.ordinal = bytes.Count(data, []byte("\n"))
		c.turns = bytes.Count(data, []byte(`"type":"task_complete"`))
		return nil
	}
	now := time.Now().UTC()
	c.rollout = filepath.Join(sessions, now.Format("2006"), now.Format("01"), now.Format("02"),
		"rollout-"+now.Format("2006-01-02T15-04-05")+"-"+c.id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(c.rollout), 0o755); err != nil {
		return err
	}
	return c.writeLine("session_meta", map[string]any{
		"session_id":     c.id,
		"id":             c.id,
		"timestamp":      timestamp(now),
		"cwd":            c.cwd,
		"originator":     "codex-tui",
		"cli_version":    CodexVersion,
		"source":         "cli",
		"model_provider": "openai",
	})
}

// turnID names the turn in progress, as Codex's task_complete lines do.
func (c *codexFake) turnID() string { return fmt.Sprintf("%s-turn-%d", c.id, c.turns) }

func timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// writeLine appends one rollout line.
func (c *codexFake) writeLine(typ string, payload map[string]any) error {
	line, err := json.Marshal(map[string]any{
		"timestamp": timestamp(time.Now()),
		"ordinal":   c.ordinal,
		"type":      typ,
		"payload":   payload,
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(c.rollout, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	c.ordinal++
	return nil
}

func (c *codexFake) message(role, contentType, text string) map[string]any {
	return map[string]any{
		"type":    "message",
		"role":    role,
		"content": []map[string]any{{"type": contentType, "text": text}},
	}
}

func (c *codexFake) setTitle(title string) { fmt.Fprint(c.out, "\x1b]0;"+title+"\a") }

func (c *codexFake) report(state string) { writeAppReport(c.out, codexAppName, state, "", "") }

func (c *codexFake) run(in io.Reader, ctl io.Reader) (int, error) {
	events := tuiEvents(in, ctl)
	c.setTitle(c.title)
	// Codex turns bracketed paste on, so a paste keeps its newlines.
	fmt.Fprint(c.out, "\x1b[?2004h")
	if err := c.sessionStart(); err != nil {
		return 0, err
	}
	c.showNext()
	for ev := range events {
		switch {
		case ev.err != nil:
			if ptyClosed(ev.err) {
				c.exit()
				return 0, nil
			}
			return 0, ev.err
		case ev.resized:
			if len(c.dialogs) == 0 {
				c.box.draw()
			}
		case ev.control != nil:
			if ev.control.Action == ActionExit {
				c.exit()
				return ev.control.Code, nil
			}
			if err := c.control(*ev.control); err != nil {
				return 0, err
			}
		case len(c.dialogs) > 0:
			c.dialogs = c.dialogs[1:]
			c.showNext()
		case c.onKey != nil:
			next := c.onKey
			c.onKey = nil
			if err := next(ev.key); err != nil {
				return 0, err
			}
		default:
			if err := c.key(ev.key); err != nil {
				return 0, err
			}
		}
	}
	return 0, nil
}

// showNext shows the first startup dialog still waiting, or, with none left,
// the idle prompt: Ready comes only after every dialog is answered.
func (c *codexFake) showNext() {
	if len(c.dialogs) > 0 {
		fmt.Fprint(c.out, c.dialogs[0]+"\r\n")
		return
	}
	c.report("idle")
	c.box.draw()
}

func (c *codexFake) exit() {
	c.setTitle("")
	fmt.Fprint(c.out, "\x1b]7501;state=clear\a")
}

// startTurn takes a submitted prompt. The turn stays working until an
// ActionFinish control ends it.
func (c *codexFake) startTurn(prompt string) error {
	c.working = true
	c.prompt = prompt
	c.turns++
	c.setTitle(codexSpinner + " " + c.title)
	c.report("working")
	turnID := fmt.Sprintf("%s-turn-%d", c.id, c.turns)
	if err := c.writeLine("event_msg", map[string]any{"type": "task_started", "turn_id": turnID}); err != nil {
		return err
	}
	return c.writeLine("response_item", c.message("user", "input_text", prompt))
}

// finishTurn writes the turn's reply, token usage and completion, then ends
// the turn on screen in the recorded order.
func (c *codexFake) finishTurn() error {
	if !c.working {
		return nil
	}
	c.working = false
	reply := cmp.Or(c.reply, codexReply)
	c.reply = ""
	if err := c.writeLine("response_item", c.message("assistant", "output_text", reply)); err != nil {
		return err
	}
	input := codexTurnTokens * c.turns
	usage := func(tokens int) map[string]any {
		return map[string]any{"input_tokens": tokens, "total_tokens": tokens}
	}
	// Each turn's whole context is its input; the session total sums them.
	total := codexTurnTokens * c.turns * (c.turns + 1) / 2
	if err := c.writeLine("event_msg", map[string]any{
		"type": "token_count",
		"info": map[string]any{
			"total_token_usage":    usage(total),
			"last_token_usage":     usage(input),
			"model_context_window": codexContextWindow,
		},
		"rate_limits": map[string]any{
			"primary": map[string]any{
				"used_percent":   10.0,
				"window_minutes": codexRateWindowMins,
				"resets_at":      time.Now().Add(codexRateWindowMins * time.Minute).Unix(),
			},
		},
	}); err != nil {
		return err
	}
	if err := c.writeLine("event_msg", map[string]any{"type": "task_complete", "turn_id": c.turnID(), "last_agent_message": reply}); err != nil {
		return err
	}
	if err := c.stopHooks(); err != nil {
		return err
	}
	if c.background {
		c.showResting()
		return c.runQueued()
	}
	c.setTitle(c.title)
	c.report("done")
	c.setTitle(c.prompt + " | " + c.title)
	c.report("idle")
	return c.runQueued()
}

func (c *codexFake) control(ctl Control) error {
	switch ctl.Action {
	case ActionFinish:
		return c.finishTurn()
	case ActionReport:
		writeAppReport(c.out, codexAppName, ctl.State, ctl.Kind, ctl.Message)
	case ActionTitle:
		c.setTitle(ctl.Title)
	case ActionBackground:
		c.backgroundControl(ctl.Running)
	case actionInterrupt:
		return c.interrupt()
	case ActionToolCall:
		return c.toolCall(ctl.Tool, ctl.Command)
	case ActionPermission:
		return c.permission(ctl.Command, nil)
	case ActionTokenCount:
		return c.tokenCount(ctl.UsedPercent, ctl.ResetsAt)
	case ActionCompact:
		return c.compact()
	case ActionTaskError:
		return c.taskError(ctl.Message, ctl.Error)
	default:
		return fmt.Errorf("unknown codex control action %q", ctl.Action)
	}
	return nil
}
