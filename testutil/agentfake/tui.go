package agentfake

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/transcript"
)

// TUIEnv, set to "1", makes a test binary that calls TUIClaude from its
// TestMain act as interactive claude on a PTY.
const TUIEnv = "GX_AGENTFAKE_TUI"

// Launch knobs for the TUI fake. They are env vars rather than controls
// because they act before the fake's first status write, before a test could
// send anything.
const (
	// TUITrustEnv, set to "1", shows the trust dialog until Enter.
	TUITrustEnv = "GX_AGENTFAKE_TUI_TRUST"
	// TUINoPidFileEnv, set to "1", never writes the pid status file.
	TUINoPidFileEnv = "GX_AGENTFAKE_TUI_NO_PID_FILE"
)

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

// TUIControlPath is the side-channel FIFO of the TUI fake running sessionID
// under home. Controls never travel on the PTY, so typed input stays exactly
// what the runner sent.
func TUIControlPath(home, sessionID string) string {
	return filepath.Join(home, ".claude", "agentfake", sessionID+".ctl")
}

// SendTUIControl writes c to the side channel of the TUI fake running
// sessionID under home, waiting for the fake to create it.
func SendTUIControl(home, sessionID string, c Control) error {
	path := TUIControlPath(home, sessionID)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			break
		} else if time.Now().After(deadline) {
			return err
		}
	}
	return SendControl(path, c)
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
	noPidFile bool
}

// parseTUIArgs accepts exactly the PTY runner's launch flags, so a new flag
// fails the conformance suite instead of passing unseen.
func parseTUIArgs(args []string) (tuiOptions, error) {
	fs := flag.NewFlagSet("claude", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "")
	fs.String("permission-mode", "", "")
	fs.String("settings", "", "")
	fs.String("disallowedTools", "", "")
	if err := fs.Parse(args); err != nil {
		return tuiOptions{}, err
	}
	if *sessionID == "" {
		return tuiOptions{}, errors.New("agentfake: --session-id is required")
	}
	return tuiOptions{
		sessionID: *sessionID,
		trust:     os.Getenv(TUITrustEnv) == "1",
		noPidFile: os.Getenv(TUINoPidFileEnv) == "1",
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

	now := time.Now()
	t := &tui{
		out:        os.Stdout,
		opts:       opts,
		pidPath:    PidFilePath(home, os.Getpid()),
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
	return t.run(os.Stdin, ctl)
}

type tui struct {
	out        io.Writer
	opts       tuiOptions
	pidPath    string
	transcript string
	pid        PidFile
	stalled    bool
	typed      strings.Builder
	prompt     string
}

// tuiEvent is one key from the terminal, one control, or the end of input.
type tuiEvent struct {
	key     byte
	control *Control
	err     error
}

func (t *tui) run(in io.Reader, ctl io.Reader) (int, error) {
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

	if t.opts.trust {
		t.draw()
	} else if err := t.startSession(); err != nil {
		return 0, err
	}
	for ev := range events {
		switch {
		case ev.err != nil:
			// The PTY closed: claude exits cleanly.
			if errors.Is(ev.err, io.EOF) || errors.Is(ev.err, syscall.EIO) {
				return 0, t.exit(0)
			}
			return 0, ev.err
		case ev.control != nil:
			if ev.control.Action == ActionExit {
				return ev.control.Code, t.exit(ev.control.Code)
			}
			if err := t.handleControl(*ev.control); err != nil {
				return 0, err
			}
		case ev.key == '\r' || ev.key == '\n':
			if t.opts.trust {
				if err := t.startSession(); err != nil {
					return 0, err
				}
				continue
			}
			if err := t.submit(); err != nil {
				return 0, err
			}
		case ev.key == escKey:
			if err := t.interrupt(); err != nil {
				return 0, err
			}
		default:
			if !t.opts.trust {
				t.typed.WriteByte(ev.key)
			}
		}
	}
	return 0, nil
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

func (t *tui) submit() error {
	text := t.typed.String()
	t.typed.Reset()
	if t.stalled || (text == "" && t.pid.Status != StatusWaiting) {
		return nil
	}
	switch t.pid.Status {
	case StatusIdle:
		t.prompt = text
		return t.setStatus(StatusBusy, "")
	case StatusWaiting:
		// Enter picks the dialog's first option, "Yes".
		return t.setStatus(StatusBusy, "")
	}
	return nil
}

const escKey = 0x1b

// interrupt is Esc: it ends a running turn, or denies a dialog, and keeps
// the session.
func (t *tui) interrupt() error {
	t.typed.Reset()
	if t.pid.Status != StatusBusy && t.pid.Status != StatusWaiting {
		return nil
	}
	return t.setStatus(StatusIdle, "")
}

func (t *tui) handleControl(c Control) error {
	switch c.Action {
	case ActionFinish:
		return t.setStatus(StatusIdle, "")
	case ActionBlock:
		waitingFor := c.WaitingFor
		if waitingFor == "" {
			waitingFor = "permission prompt"
		}
		return t.setStatus(StatusWaiting, waitingFor)
	case ActionStall:
		t.stalled = true
		return nil
	case ActionBackground:
		return t.writeBackgroundMarker(c.Running)
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
	line := `{"isSidechain":false,"timestamp":"` + ts + `","message":{"content":[{"tool_use_id":"` + toolUseID + `","type":"tool_result","content":"...","is_error":false}]},"toolUseResult":{"stdout":"","backgroundTaskId":"` + taskID + `"}}`
	if !running {
		content := `<task-notification>\n<task-id>` + taskID + `</task-id>\n<tool-use-id>` + toolUseID + `</tool-use-id>\n<status>completed</status>\n</task-notification>`
		line = `{"isSidechain":false,"timestamp":"` + ts + `","origin":{"kind":"task-notification"},"message":{"content":"` + content + `"}}`
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

// draw clears the terminal and redraws the screen for the current state.
func (t *tui) draw() {
	title := idleTitle
	var rows []string
	switch {
	case t.opts.trust:
		rows = []string{
			"Quick safety check: Is this a project you created or one you trust?",
			"",
			"❯ 1. Yes, I trust this folder",
			"  2. No, exit",
		}
	case t.pid.Status == StatusWaiting:
		rows = []string{
			"> " + t.prompt,
			"Waiting for " + t.pid.WaitingFor,
			"Do you want to proceed?",
			"❯ 1. Yes",
			"  2. No",
		}
	case t.pid.Status == StatusBusy:
		title = workingTitle
		rows = []string{"> " + t.prompt, "✻ Working…"}
	default:
		rows = []string{"✻ Welcome to Claude Code", "", "> "}
	}
	fmt.Fprint(t.out, "\x1b[2J\x1b[H"+title+strings.Join(rows, "\r\n"))
}
