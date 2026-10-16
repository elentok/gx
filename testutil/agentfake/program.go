package agentfake

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/harness"
)

// ProgramEnv, set to "1", makes a test binary that calls Program from its
// TestMain act as the fake harness's program on a PTY.
const ProgramEnv = "GX_AGENTFAKE_PROGRAM"

// ProgramTrustEnv, set to "1", makes the fake program show ProgramTrustPrompt
// until Enter, before it probes for OSC 7501.
const ProgramTrustEnv = "GX_AGENTFAKE_PROGRAM_TRUST"

// ProgramTrustPrompt is the fake program's trust dialog.
const ProgramTrustPrompt = "Do you trust the files in this folder?"

// Actions only the fake program takes.
const (
	// ActionReport writes an OSC 7501 report of Control.State, Kind and
	// Message.
	ActionReport = "report"
	// ActionTitle sets the OSC 0 title to Control.Title.
	ActionTitle = "title"
	// ActionWrite writes Control.Output as is.
	ActionWrite = "write"
	// ActionConversation reports Control.ConversationID as the program's
	// conversation id, as a harness does on /clear.
	ActionConversation = "conversation"
	// ActionSize writes the terminal's size as "size=<cols>x<rows>".
	ActionSize = "size"
)

// ProgramDir holds the fake program's side channels. It is under the temp
// dir rather than HOME so the holder, the program and the test agree on it
// without passing HOME around.
func ProgramDir() string {
	return filepath.Join(os.TempDir(), "gx-agentfake")
}

// ProgramControlPath is the side-channel FIFO of the fake program running
// conversationID.
func ProgramControlPath(conversationID string) string {
	return filepath.Join(ProgramDir(), conversationID+".ctl")
}

// programConversationPath is where the fake program running as pid writes
// its conversation id for the Watch, which knows only the pid.
func programConversationPath(pid int) string {
	return filepath.Join(ProgramDir(), strconv.Itoa(pid)+".conversation")
}

// SendProgramControl writes c to the side channel of the fake program
// running conversationID, waiting for the program to create it.
func SendProgramControl(conversationID string, c Control) error {
	return sendControlWhenReady(ProgramControlPath(conversationID), c)
}

// Program runs the fake program on the process's terminal and exits with the
// code an exit control asked for.
func Program() {
	code, err := runProgram()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// programConversationID is the id after --session-id or --resume.
func programConversationID(args []string) (string, bool) {
	for i, arg := range args[:max(len(args)-1, 0)] {
		if arg == "--session-id" || arg == "--resume" {
			return args[i+1], true
		}
	}
	return "", false
}

func runProgram() (int, error) {
	id, ok := programConversationID(os.Args[1:])
	if !ok {
		return 0, errors.New("agentfake: --session-id or --resume is required")
	}
	if term.IsTerminal(os.Stdin.Fd()) {
		if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
			return 0, err
		}
	}
	if err := os.MkdirAll(ProgramDir(), 0o755); err != nil {
		return 0, err
	}
	convPath := programConversationPath(os.Getpid())
	if err := os.WriteFile(convPath, []byte(id), 0o644); err != nil {
		return 0, err
	}
	defer os.Remove(convPath)
	ctlPath := ProgramControlPath(id)
	// A crashed run with the same id leaves its FIFO behind.
	_ = os.Remove(ctlPath)
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
	p := &program{out: os.Stdout, trust: os.Getenv(ProgramTrustEnv) == "1", convPath: convPath}
	return p.run(os.Stdin, ctl)
}

type program struct {
	out      io.Writer
	trust    bool
	convPath string
	// esc and osc track an escape sequence on input, so the terminal's
	// reply to the probe is not echoed as typed text.
	esc, osc bool
}

func (p *program) run(in, ctl io.Reader) (int, error) {
	events := tuiEvents(in, ctl)
	if p.trust {
		fmt.Fprint(p.out, "\x1b[2J\x1b[H"+ProgramTrustPrompt+"\r\n❯ 1. Yes, proceed\r\n  2. No, exit")
	} else {
		p.start()
	}
	for ev := range events {
		switch {
		case ev.err != nil:
			if ptyClosed(ev.err) {
				return 0, nil
			}
			return 0, ev.err
		case ev.control != nil:
			if ev.control.Action == ActionExit {
				p.report("clear", "", "")
				return ev.control.Code, nil
			}
			if err := p.control(*ev.control); err != nil {
				return 0, err
			}
		case p.trust:
			if ev.key == '\r' || ev.key == '\n' {
				p.trust = false
				p.start()
			}
		default:
			p.key(ev.key)
		}
	}
	return 0, nil
}

func (p *program) start() {
	fmt.Fprint(p.out, "\x1b[2J\x1b[H"+harness.ProgramStatusProbe)
}

// key echoes a typed key, skipping OSC sequences.
func (p *program) key(b byte) {
	switch {
	case p.osc:
		p.osc = b != '\a'
		return
	case p.esc:
		p.esc = false
		if b == ']' {
			p.osc = true
			return
		}
	case b == escKey:
		p.esc = true
		return
	}
	if b == '\r' || b == '\n' {
		fmt.Fprint(p.out, "\r\n")
		return
	}
	_, _ = p.out.Write([]byte{b})
}

func (p *program) control(c Control) error {
	switch c.Action {
	case ActionReport:
		p.report(c.State, c.Kind, c.Message)
	case ActionTitle:
		fmt.Fprint(p.out, "\x1b]0;"+c.Title+"\a")
	case ActionWrite:
		fmt.Fprint(p.out, c.Output)
	case ActionConversation:
		return os.WriteFile(p.convPath, []byte(c.ConversationID), 0o644)
	case ActionSize:
		cols, rows, err := term.GetSize(os.Stdout.Fd())
		if err != nil {
			return err
		}
		fmt.Fprintf(p.out, "size=%dx%d\r\n", cols, rows)
	default:
		return fmt.Errorf("unknown program control action %q", c.Action)
	}
	return nil
}

func (p *program) report(state, kind, msg string) {
	s := "\x1b]7501;state=" + state + ":app=agentfake"
	if kind != "" {
		s += ":kind=" + kind
	}
	if msg != "" {
		s += ":msg=" + base64.StdEncoding.EncodeToString([]byte(msg))
	}
	fmt.Fprint(p.out, s+"\a")
}
