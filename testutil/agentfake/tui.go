package agentfake

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// TUIEnv, set to "1", makes a test binary that calls TUIClaude from its
// TestMain act as interactive claude on a PTY.
const TUIEnv = "GX_AGENTFAKE_TUI"

// Launch knobs for the TUI fake. They are env vars rather than controls
// because they act before the fake's first status write, before a test could
// send anything.
const (
	// TUITrustEnv, set to "1", shows the trust dialog as claude does: "No,
	// exit" selected first, arrows move the cursor, Enter picks.
	TUITrustEnv = "GX_AGENTFAKE_TUI_TRUST"
	// TUITrustDropKeyEnv, set to "1", drops the first key at the trust
	// dialog, as claude does with one typed before it reads input.
	TUITrustDropKeyEnv = "GX_AGENTFAKE_TUI_TRUST_DROP_KEY"
	// TUINoPidFileEnv, set to "1", never writes the pid status file.
	TUINoPidFileEnv = "GX_AGENTFAKE_TUI_NO_PID_FILE"
	// TUIMouseEnv, set to "1", runs in the alternate screen with SGR mouse
	// reporting on, as claude does, and counts the wheel events it reads on
	// screen. An Esc then only interrupts as part of a sequence.
	TUIMouseEnv = "GX_AGENTFAKE_TUI_MOUSE"
	// TUIDraftEnv, when not empty, seeds the input with that draft and draws
	// the input row as claude does: a styled marker, and a dim placeholder while
	// empty, so a draft check can decide.
	TUIDraftEnv = "GX_AGENTFAKE_TUI_DRAFT"
)

// PermissionPromptPrefix, at the start of a submitted prompt, makes the TUI
// fake ask permission to run the rest of the prompt as a Bash command, with no
// control needed.
const PermissionPromptPrefix = "fakeclaude:permission "

// PermissionCommand is the Bash command a BehaviourPermission worker asks to run.
const PermissionCommand = "rg fixture"

// BashPermissionDialog is the screen rows of a Bash permission prompt for command.
func BashPermissionDialog(command string) []string {
	return []string{
		"────────────────────",
		"Bash command",
		"  " + command,
		"Do you want to proceed?",
		"❯ 1. Yes",
		"  2. No",
		"Esc to cancel",
	}
}

// QuestionMenuDialog is the screen rows of a question menu: a numbered choice
// list that is not an approvable permission card.
func QuestionMenuDialog() []string {
	return []string{
		"────────────────────",
		"Question",
		"  Which option should the fake worker take?",
		"Do you want option A or option B?",
		"❯ 1. Option A",
		"  2. Option B",
		"Esc to cancel",
	}
}

// ActionExit makes the TUI fake exit with Control.Code.
const ActionExit = "exit"

// Pid status file values, as real claude writes them.
const (
	StatusBusy    = "busy"
	StatusIdle    = "idle"
	StatusWaiting = "waiting"
)

// PidFile is the shape of claude's ~/.claude/sessions/<pid>.json, limited to
// the fields gx reads plus the ones that identify the process. Status is
// absent while starting; WaitingFor is absent unless Status is waiting.
type PidFile struct {
	Pid             int    `json:"pid"`
	SessionID       string `json:"sessionId"`
	Cwd             string `json:"cwd"`
	StartedAt       int64  `json:"startedAt"`
	ProcStart       string `json:"procStart"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Entrypoint      string `json:"entrypoint"`
	Status          string `json:"status,omitempty"`
	WaitingFor      string `json:"waitingFor,omitempty"`
	UpdatedAt       int64  `json:"updatedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt,omitempty"`
}

// PidFilePath is where claude running as pid under home writes its status.
func PidFilePath(home string, pid int) string {
	return filepath.Join(home, ".claude", "sessions", strconv.Itoa(pid)+".json")
}

// pidPathFor is PidFilePath, moved under CLAUDE_CONFIG_DIR when it is set, as
// real claude does.
func pidPathFor(home string, pid int) string {
	if dir, err := harness.ClaudeSessionsDirIn(os.Environ()); err == nil {
		return filepath.Join(dir, strconv.Itoa(pid)+".json")
	}
	return PidFilePath(home, pid)
}

// TUIControlPath is the side-channel FIFO of the TUI fake running sessionID
// under home. Controls never travel on the PTY, so typed input stays exactly
// what the runner sent.
func TUIControlPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".ctl")
}

// TUIInputPath logs every key the TUI fake running sessionID under home read
// off its terminal.
func TUIInputPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".in")
}

// TUIAckPath logs one byte per control the TUI fake running sessionID under
// home has handled.
func TUIAckPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".ack")
}

// TUIArgsPath holds the argv (a JSON array) the TUI fake running sessionID
// under home was started with.
func TUIArgsPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".args")
}

// ReadTUIArgs is the argv the TUI fake running sessionID under home was
// started with.
func ReadTUIArgs(home, sessionID string) ([]string, error) {
	data, err := os.ReadFile(TUIArgsPath(home, sessionID))
	if err != nil {
		return nil, err
	}
	var args []string
	return args, json.Unmarshal(data, &args)
}

// ReportedModel is the model the TUI fake reports on its replies when it was
// started without --model; StaleModel makes it exit at once, as claude does
// for a model it no longer knows.
const (
	ReportedModel = "claude-fake-1"
	StaleModel    = "stale-model"
)

// SendTUIControl writes c to the side channel of the TUI fake running
// sessionID under home, waiting for the fake to create it and then to handle
// c. The control FIFO and the PTY's keys reach the fake on separate paths, so
// a key sent before the ack can beat the control. ActionExit gets no ack.
func SendTUIControl(home, sessionID string, c Control) error {
	ackPath := TUIAckPath(home, sessionID)
	var handled int64
	if st, err := os.Stat(ackPath); err == nil {
		handled = st.Size()
	}
	ctlPath := TUIControlPath(home, sessionID)
	if err := sendControlWhenReady(func() (string, bool) {
		_, err := os.Stat(ctlPath)
		return ctlPath, err == nil
	}, c); err != nil {
		return err
	}
	if c.Action == ActionExit {
		return nil
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if st, err := os.Stat(ackPath); err == nil && st.Size() > handled {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agentfake: control %s not handled", c.Action)
		}
	}
}

// sendControlWhenReady writes c to the side-channel FIFO that ready reports,
// waiting until ready says the fake has it open. A FIFO that merely exists has
// no reader yet, and a write to it fails with ENXIO.
func sendControlWhenReady(ready func() (path string, ok bool), c Control) error {
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		path, ok := ready()
		if ok {
			return SendControl(path, c)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agentfake: %s never opened", path)
		}
	}
}

// TUIClaude runs the fake interactive claude on the process's terminal and
// exits with the code an exit control asked for.
func TUIClaude() {
	code, err := runTUIClaude()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type tuiOptions struct {
	sessionID string
	trust     bool
	dropKey   bool
	noPidFile bool
	mouse     bool
	// draft seeds the input; a non-empty one also styles the input row.
	draft string
	// model is --model, reported on every reply.
	model string
	// settings is --settings: inline JSON or a path to a JSON file.
	settings string
}

// parseTUIArgs accepts exactly the PTY runner's launch flags, so a new flag
// fails the conformance suite instead of passing unseen.
func parseTUIArgs(args []string) (tuiOptions, error) {
	fs := flag.NewFlagSet("claude", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "")
	resume := fs.String("resume", "", "")
	fs.String("permission-mode", "", "")
	settings := fs.String("settings", "", "")
	fs.String("disallowedTools", "", "")
	fs.String("append-system-prompt", "", "")
	fs.String("append-system-prompt-file", "", "")
	model := fs.String("model", "", "")
	fs.String("effort", "", "")
	fs.String("remote-control", "", "")
	if err := fs.Parse(args); err != nil {
		return tuiOptions{}, err
	}
	// Real claude fails with both unless --fork-session; --resume keeps the id.
	if *sessionID != "" && *resume != "" {
		return tuiOptions{}, errors.New("agentfake: --session-id and --resume are exclusive")
	}
	if *sessionID == "" {
		*sessionID = *resume
	}
	if *sessionID == "" {
		return tuiOptions{}, errors.New("agentfake: --session-id or --resume is required")
	}
	return tuiOptions{
		sessionID: *sessionID,
		trust:     os.Getenv(TUITrustEnv) == "1",
		dropKey:   os.Getenv(TUITrustDropKeyEnv) == "1",
		noPidFile: os.Getenv(TUINoPidFileEnv) == "1",
		mouse:     os.Getenv(TUIMouseEnv) == "1",
		draft:     os.Getenv(TUIDraftEnv),
		model:     cmp.Or(*model, ReportedModel),
		settings:  *settings,
	}, nil
}

func runTUIClaude() (int, error) {
	opts, err := parseTUIArgs(os.Args[1:])
	if err != nil {
		return 0, err
	}
	// Claude reads its terminal raw; a cooked one would hold a lone Esc
	// until the next Enter.
	if term.IsTerminal(os.Stdin.Fd()) {
		if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
			return 0, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	ctlPath := TUIControlPath(home, opts.sessionID)
	if err := os.MkdirAll(filepath.Dir(ctlPath), 0o755); err != nil {
		return 0, err
	}
	if err := syscall.Mkfifo(ctlPath, 0o600); err != nil {
		return 0, err
	}
	defer os.Remove(ctlPath)
	// Read-write so the FIFO never reads EOF between two senders.
	ctl, err := os.OpenFile(ctlPath, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer ctl.Close()
	in, err := os.Create(TUIInputPath(home, opts.sessionID))
	if err != nil {
		return 0, err
	}
	defer in.Close()
	ack, err := os.Create(TUIAckPath(home, opts.sessionID))
	if err != nil {
		return 0, err
	}
	defer ack.Close()
	argv, err := json.Marshal(os.Args[1:])
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(TUIArgsPath(home, opts.sessionID), argv, 0o644); err != nil {
		return 0, err
	}
	if opts.model == StaleModel {
		return 0, fmt.Errorf("agentfake: model %q is not available", opts.model)
	}

	now := time.Now()
	t := &tui{
		out:        os.Stdout,
		ack:        ack,
		opts:       opts,
		pidPath:    pidPathFor(home, os.Getpid()),
		ctlPath:    ctlPath,
		transcript: transcript.PathIn(home, cwd, opts.sessionID),
		pid: PidFile{
			Pid:        os.Getpid(),
			SessionID:  opts.sessionID,
			Cwd:        cwd,
			StartedAt:  now.UnixMilli(),
			ProcStart:  now.Format(time.ANSIC),
			Version:    "2.1.296",
			Kind:       "interactive",
			Entrypoint: "cli",
		},
	}
	t.typed.WriteString(opts.draft)
	return t.run(io.TeeReader(os.Stdin, in), ctl)
}

type tui struct {
	out io.Writer
	// ack gets one byte after each handled control.
	ack        io.Writer
	opts       tuiOptions
	pidPath    string
	ctlPath    string
	transcript string
	pid        PidFile
	stalled    bool
	// fast makes the next turn end as it starts.
	fast bool
	// dialogNext makes the next "/" command open a dialog.
	dialogNext bool
	// busyNext makes the next "/" command start a turn that stays busy until a
	// control ends it.
	busyNext bool
	// dialog replaces the default waiting rows while set by ActionBlock.
	dialog []string
	// trustYes is whether the trust dialog's cursor is on "Yes"; arrow
	// collects an arrow key's escape sequence.
	trustYes bool
	arrow    []byte
	typed    strings.Builder
	prompt   string
	// seq collects an escape sequence in mouse mode; wheelUp and wheelDown
	// count the wheel events read.
	seq                []byte
	wheelUp, wheelDown int
	design             designState
	// resume is the ticket a ratelimit or question-menu worker finishes on the
	// next prompt, whatever that prompt says.
	resume *pendingWork
}

type pendingWork struct {
	cfg     WorkerConfig
	address string
}

// designState is set once a /gx-design prompt arrives; rounds counts the ones
// played.
type designState struct {
	active bool
	rounds int
	turns  int
	// script is the turn script of the map this session designs, from
	// WorkerConfig.DesignByMap; nil falls back to WorkerConfig.Design.
	script []string
}

// tuiEvent is one key from the terminal, one control, or the end of input.
type tuiEvent struct {
	key     byte
	control *Control
	resized bool
	err     error
}

func (t *tui) run(in io.Reader, ctl io.Reader) (int, error) {
	events := tuiEvents(in, ctl)
	if t.opts.mouse {
		fmt.Fprint(t.out, "\x1b[?1049h\x1b[?1000h\x1b[?1006h")
	}
	if t.opts.trust {
		t.draw()
	} else if err := t.startSession(); err != nil {
		return 0, err
	}
	for ev := range events {
		switch {
		case ev.err != nil:
			// The PTY closed: claude exits cleanly.
			if ptyClosed(ev.err) {
				return 0, t.exit(0)
			}
			return 0, ev.err
		case ev.resized:
			t.draw()
		case ev.control != nil:
			if ev.control.Action == ActionExit {
				return ev.control.Code, t.exit(ev.control.Code)
			}
			if err := t.handleControl(*ev.control); err != nil {
				return 0, err
			}
			if _, err := t.ack.Write([]byte{'.'}); err != nil {
				return 0, err
			}
		case t.opts.trust:
			if quit, err := t.trustKey(ev.key); quit || err != nil {
				return 1, err
			}
		case t.opts.mouse && (ev.key == escKey || len(t.seq) > 0):
			if err := t.mouseSeqKey(ev.key); err != nil {
				return 0, err
			}
		case ev.key == '\r':
			// A raw LF is a line break in the input, as in claude, not Enter.
			if err := t.submit(); err != nil {
				return 0, err
			}
		case ev.key == escKey:
			if err := t.interrupt(); err != nil {
				return 0, err
			}
		case ev.key == ctrlC:
			if err := t.cancel(); err != nil {
				return 0, err
			}
		case ev.key == ctrlU:
			t.typed.Reset()
			t.draw()
		default:
			t.typed.WriteByte(ev.key)
			// Echoed as claude does, the terminal's cursor after it.
			t.draw()
		}
	}
	return 0, nil
}

func ptyClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO)
}

// tuiEvents merges the keys read from in with the controls read from ctl.
func tuiEvents(in io.Reader, ctl io.Reader) <-chan tuiEvent {
	events := make(chan tuiEvent)
	go func() {
		r := bufio.NewReader(in)
		for {
			b, err := r.ReadByte()
			if err != nil {
				events <- tuiEvent{err: err}
				return
			}
			events <- tuiEvent{key: b}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(ctl)
		for scanner.Scan() {
			var c Control
			if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
				events <- tuiEvent{err: fmt.Errorf("bad control line %q: %w", scanner.Text(), err)}
				return
			}
			events <- tuiEvent{control: &c}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			events <- tuiEvent{resized: true}
		}
	}()
	return events
}

// mouseSeqKey collects an escape sequence: an SGR mouse report counts its
// wheel turns; anything else is an Esc, and interrupts.
func (t *tui) mouseSeqKey(key byte) error {
	t.seq = append(t.seq, key)
	s := string(t.seq)
	if s == "\x1b" || s == "\x1b[" || (strings.HasPrefix(s, "\x1b[<") && key != 'M' && key != 'm') {
		return nil
	}
	t.seq = nil
	if !strings.HasPrefix(s, "\x1b[<") {
		return t.interrupt()
	}
	switch button, _, _ := strings.Cut(s[len("\x1b[<"):], ";"); button {
	case "64":
		t.wheelUp++
	case "65":
		t.wheelDown++
	}
	t.draw()
	return nil
}

// trustKey takes one key at the trust dialog and reports whether it picked
// "No, exit". Arrow keys toggle between the two options.
func (t *tui) trustKey(key byte) (bool, error) {
	if t.opts.dropKey {
		t.opts.dropKey = false
		return false, nil
	}
	if key == escKey || len(t.arrow) > 0 {
		t.arrow = append(t.arrow, key)
		if len(t.arrow) == len("\x1b[B") {
			if s := string(t.arrow); s == "\x1b[A" || s == "\x1b[B" {
				t.trustYes = !t.trustYes
				t.draw()
			}
			t.arrow = nil
		}
		return false, nil
	}
	if key != '\r' && key != '\n' {
		return false, nil
	}
	if !t.trustYes {
		return true, nil
	}
	return false, t.startSession()
}

// startSession writes the pid file, first without a status, as claude does
// while it loads, then idle.
func (t *tui) startSession() error {
	t.opts.trust = false
	if err := t.writePid(); err != nil {
		return err
	}
	return t.setStatus(StatusIdle, "")
}

// submit is Enter. A stalled fake leaves the text unsent in the input box.
func (t *tui) submit() error {
	if t.stalled {
		t.draw()
		return nil
	}
	text := t.typed.String()
	t.typed.Reset()
	if text == "" && t.pid.Status != StatusWaiting {
		return nil
	}
	switch t.pid.Status {
	case StatusIdle:
		if w := t.resume; w != nil && text != "" {
			t.resume = nil
			t.prompt = text
			go t.work(w.cfg, w.address)
			return t.setStatus(StatusBusy, "")
		}
		kind, address, cfg := t.classify(text)
		switch kind {
		case promptDesign:
			return t.designTurn(text)
		case promptWorker:
			t.prompt = text
			switch cfg.Behaviour[address] {
			case BehaviourPermission:
				t.dialog = BashPermissionDialog(PermissionCommand)
				return t.setStatus(StatusWaiting, "permission prompt")
			case BehaviourQuestionMenu:
				t.resume = &pendingWork{cfg, address}
				t.dialog = QuestionMenuDialog()
				return t.setStatus(StatusWaiting, "question menu")
			case BehaviourRateLimit:
				t.resume = &pendingWork{cfg, address}
				return t.rateLimit(cfg)
			}
			go t.work(cfg, address)
			return t.setStatus(StatusBusy, "")
		case promptSlash:
			return t.slashCommand(text)
		case promptPermission:
			t.prompt = text
			t.dialog = BashPermissionDialog(strings.TrimPrefix(text, PermissionPromptPrefix))
			return t.setStatus(StatusWaiting, "permission prompt")
		}
		t.prompt = text
		if t.fast {
			return t.fastTurn()
		}
		if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil && cfg.Chat {
			return t.chatTurn(text)
		}
		return t.setStatus(StatusBusy, "")
	case StatusWaiting:
		// Enter picks the dialog's first option, "Yes".
		return t.setStatus(StatusBusy, "")
	}
	return nil
}

// promptKind is how an idle fake handles a submitted prompt.
type promptKind int

const (
	promptPlain promptKind = iota
	promptDesign
	promptWorker
	promptSlash
	promptPermission
)

// classify picks the prompt's handler. The order matters: a design prompt also
// looks like `/skill <address>`, so it comes before the worker check, and a
// worker prompt is also a slash command. For a worker prompt it returns the
// ticket address and the worker config.
func (t *tui) classify(text string) (promptKind, string, WorkerConfig) {
	if isDesignPrompt(text) || (t.design.active && text != "" && !strings.HasPrefix(text, "/")) {
		return promptDesign, "", WorkerConfig{}
	}
	if address, cfg, ok := t.workerPrompt(text); ok {
		return promptWorker, address, cfg
	}
	if strings.HasPrefix(text, "/") && text != "/compact" {
		return promptSlash, "", WorkerConfig{}
	}
	if strings.HasPrefix(text, PermissionPromptPrefix) {
		return promptPermission, "", WorkerConfig{}
	}
	return promptPlain, "", WorkerConfig{}
}

// workerPrompt reports whether text is a skill prompt (`/skill <address>`) for
// a fake that runs in a worktree carrying a worker config, and returns the
// ticket address and that config.
func (t *tui) workerPrompt(text string) (string, WorkerConfig, bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "/") {
		return "", WorkerConfig{}, false
	}
	// A fix agent's prompt ends in free text, not a ticket address.
	if fields[0] == "/gx-investigate" {
		return "", WorkerConfig{}, false
	}
	cfg, err := LoadWorkerConfig(t.pid.Cwd)
	if err != nil {
		return "", WorkerConfig{}, false
	}
	return fields[len(fields)-1], cfg, true
}

// work does the ticket's work off the input loop, then sends the turn's end
// down the control channel like a test would.
func (t *tui) work(cfg WorkerConfig, address string) {
	if err := WorkTicket(cfg, address); err != nil {
		fmt.Fprintln(os.Stderr, "fakeclaude worker:", err)
	}
	if cfg.Behaviour[address] == BehaviourFail {
		_ = SendControl(t.ctlPath, Control{Action: ActionExit, Code: 1})
		return
	}
	_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
}

// fastTurn runs an armed fast turn: working, then done, with no busy status.
func (t *tui) fastTurn() error {
	t.fast = false
	writeReport(t.out, "working", "", "")
	writeReport(t.out, "done", "", "")
	return nil
}

// slashCommand handles a "/" command at idle as claude does: it starts no
// turn. /clear starts a new conversation; a dialog command waits on a dialog.
// The server launches every iteration with a slash prompt, so an armed fast or
// busy turn makes one run a turn anyway.
func (t *tui) slashCommand(text string) error {
	switch {
	case t.dialogNext:
		t.dialogNext = false
		return t.setStatus(StatusWaiting, "input needed")
	case text == "/clear":
		t.pid.SessionID += "-cleared"
		return t.writePid()
	case t.fast:
		return t.fastTurn()
	case t.busyNext:
		t.busyNext = false
		t.prompt = text
		return t.setStatus(StatusBusy, "")
	}
	return nil
}

const escKey = 0x1b

const (
	ctrlC = 0x03
	ctrlU = 0x15
)

// interrupt is Esc: it denies a dialog and keeps the session. On a running
// turn it does nothing: only Ctrl+C cancels that. A stalled fake leaves a
// dialog open.
func (t *tui) interrupt() error {
	t.typed.Reset()
	if t.pid.Status != StatusWaiting || t.stalled {
		return nil
	}
	return t.setStatus(StatusIdle, "")
}

// cancel is Ctrl+C: it ends a running turn and restores the cancelled prompt
// into the input line. On an idle prompt it clears the whole input, as real
// claude does. A stalled fake ignores it.
func (t *tui) cancel() error {
	if t.stalled {
		return nil
	}
	if t.pid.Status == StatusIdle {
		t.typed.Reset()
		t.draw()
		return nil
	}
	if t.pid.Status != StatusBusy {
		return nil
	}
	t.typed.Reset()
	t.typed.WriteString(t.prompt)
	if err := t.setStatus(StatusIdle, ""); err != nil {
		return err
	}
	t.draw()
	return nil
}

func (t *tui) handleControl(c Control) error {
	switch c.Action {
	case ActionFinish:
		if err := t.setStatus(StatusIdle, ""); err != nil {
			return err
		}
		t.runStatusLine()
		return nil
	case ActionBlock:
		waitingFor := c.WaitingFor
		if waitingFor == "" {
			waitingFor = "permission prompt"
		}
		t.dialog = c.Dialog
		return t.setStatus(StatusWaiting, waitingFor)
	case ActionStall:
		t.stalled = true
		return nil
	case ActionBackground:
		return t.writeBackgroundMarker(c.Running)
	case ActionFast:
		t.fast = true
		return nil
	case ActionDialog:
		t.dialogNext = true
		return nil
	case ActionBusy:
		t.busyNext = true
		return nil
	case ActionReport:
		writeReport(t.out, c.State, c.Kind, c.Message)
		return nil
	case ActionConversation:
		t.pid.SessionID = c.ConversationID
		return t.writePid()
	case ActionAPIError:
		return t.apiError(c)
	default:
		return fmt.Errorf("unknown control action %q", c.Action)
	}
}

func (t *tui) setStatus(status, waitingFor string) error {
	t.pid.Status = status
	t.pid.WaitingFor = waitingFor
	t.pid.StatusUpdatedAt = time.Now().UnixMilli()
	t.draw()
	return t.writePid()
}

// writePid replaces the pid file in one rename, so a reader never sees half
// of it.
func (t *tui) writePid() error {
	if t.opts.noPidFile {
		return nil
	}
	t.pid.UpdatedAt = time.Now().UnixMilli()
	data, err := json.Marshal(t.pid)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.pidPath), 0o755); err != nil {
		return err
	}
	tmp := t.pidPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.pidPath)
}

// exit removes the pid file on a clean exit, as claude does; a crash leaves
// it behind.
func (t *tui) exit(code int) error {
	if code != 0 || t.opts.noPidFile {
		return nil
	}
	if err := os.Remove(t.pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeBackgroundMarker appends the transcript line claude writes when a
// background task starts, or its completion notification.
func (t *tui) writeBackgroundMarker(running bool) error {
	const taskID, toolUseID = "b1", "toolu_b1"
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
	if !running {
		content := `<task-notification>\n<task-id>` + taskID + `</task-id>\n<tool-use-id>` + toolUseID + `</tool-use-id>\n<status>completed</status>\n</task-notification>`
		line = `{"isSidechain":false,"type":"user","timestamp":"` + ts + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
	}
	if err := os.MkdirAll(filepath.Dir(t.transcript), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.transcript, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// colored wraps s in red when the terminal supports color. Like claude
// (supports-color), it decides from TERM and COLORTERM alone, so a session
// without them is colorless.
func colored(s string) string {
	term := os.Getenv("TERM")
	if os.Getenv("COLORTERM") == "" && (term == "" || term == "dumb") {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

// draw clears the terminal and redraws the screen for the current state.
func (t *tui) draw() {
	title := idleTitle
	var rows []string
	switch {
	case t.opts.trust:
		no, yes := "❯ No, exit", "  Yes, I trust this folder"
		if t.trustYes {
			no, yes = "  No, exit", "❯ Yes, I trust this folder"
		}
		rows = []string{"Quick safety check: Is this a project you created or one you trust?", "", no, yes}
	case t.pid.Status == StatusWaiting:
		rows = []string{
			"> " + t.prompt,
			"Waiting for " + t.pid.WaitingFor,
			"Do you want to proceed?",
			"❯ 1. Yes",
			"  2. No",
		}
		if t.dialog != nil {
			rows = t.dialog
		}
	case t.pid.Status == StatusBusy:
		title = workingTitle
		rows = []string{"> " + t.prompt, "✻ Working…"}
	case t.opts.draft != "":
		input := promptMarker + t.typed.String()
		if t.typed.Len() == 0 {
			input = promptMarker + "\x1b[2m" + ProgramPlaceholder + "\x1b[0m"
		}
		rows = []string{colored("✻ Welcome to Claude Code"), "", input}
	default:
		rows = []string{colored("✻ Welcome to Claude Code"), "", "> " + t.typed.String()}
	}
	if t.opts.mouse {
		rows = append(rows, fmt.Sprintf("wheel up %d down %d", t.wheelUp, t.wheelDown))
	}
	if cols, _, err := term.GetSize(os.Stdout.Fd()); err == nil && cols > 0 {
		// Claude lays its text out for the width it has and redraws when it changes.
		wrapped := make([]string, len(rows))
		for i, row := range rows {
			wrapped[i] = strings.ReplaceAll(ansi.Wordwrap(row, cols, ""), "\n", "\r\n")
		}
		rows = wrapped
	}
	fmt.Fprint(t.out, "\x1b[2J\x1b[H"+title+strings.Join(rows, "\r\n"))
}
