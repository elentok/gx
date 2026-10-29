package agentfake

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

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

// ProgramIdleEnv, set to "1", makes the fake program report idle once it is
// up, for a harness whose sessions nothing else drives to a status.
const ProgramIdleEnv = "GX_AGENTFAKE_PROGRAM_IDLE"

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

// ProgramDirEnv is the dir of the fake program's side channels. The holder and
// the program read it from the env the server hands them; NewProgramDir makes
// one per test.
const ProgramDirEnv = "GX_AGENTFAKE_DIR"

// ProgramFifoDelayEnv is a Go duration the program waits between making its
// control FIFO and opening it, so a test can send in that window.
const ProgramFifoDelayEnv = "GX_AGENTFAKE_PROGRAM_FIFO_DELAY"

var (
	dirsMu sync.Mutex
	dirs   []string
)

// NewProgramDir makes the side-channel dir of one test's fake programs, removed
// with the test. Put ProgramDirEnv=<dir> in the env the programs run with; the
// test-side helpers (SendProgramControl, ProgramSubmittedPath) find the dir
// of a conversation among the dirs made here.
func NewProgramDir(t testing.TB) string {
	dir := t.TempDir()
	dirsMu.Lock()
	dirs = append(dirs, dir)
	dirsMu.Unlock()
	t.Cleanup(func() {
		dirsMu.Lock()
		defer dirsMu.Unlock()
		dirs = slices.DeleteFunc(dirs, func(d string) bool { return d == dir })
	})
	return dir
}

// programDir is the dir of the process's own side channels.
func programDir() string {
	if dir := os.Getenv(ProgramDirEnv); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "gx-agentfake")
}

// testDir is the dir of the program running conversationID, found by the
// ready file the program leaves. Before the program started it is the newest
// dir: the file the caller wants does not exist yet in any of them.
func testDir(conversationID string) string {
	dirsMu.Lock()
	defer dirsMu.Unlock()
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, conversationID+".ready")); err == nil {
			return dir
		}
	}
	if len(dirs) > 0 {
		return dirs[len(dirs)-1]
	}
	return programDir()
}

// ProgramControlPath is the side-channel FIFO of the fake program running
// conversationID.
func ProgramControlPath(conversationID string) string {
	return filepath.Join(testDir(conversationID), conversationID+".ctl")
}

// programReadyPath marks that the program running conversationID has opened
// its control FIFO. It stays after the program exits, so a test can still find
// the program's dir.
func programReadyPath(dir, conversationID string) string {
	return filepath.Join(dir, conversationID+".ready")
}

// programConversationPath is where the fake program running as pid writes
// its conversation id for the Watch, which knows only the pid.
func programConversationPath(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".conversation")
}

// SendProgramControl writes c to the side channel of the fake program
// running conversationID, waiting for the program to open it.
func SendProgramControl(conversationID string, c Control) error {
	return sendControlWhenReady(func() (string, bool) {
		dir := testDir(conversationID)
		ctl := filepath.Join(dir, conversationID+".ctl")
		// The ready file outlives the program; the FIFO does not.
		_, readyErr := os.Stat(programReadyPath(dir, conversationID))
		_, ctlErr := os.Stat(ctl)
		return ctl, readyErr == nil && ctlErr == nil
	}, c)
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
	dir := programDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	convPath := programConversationPath(dir, os.Getpid())
	if err := os.WriteFile(convPath, []byte(id), 0o644); err != nil {
		return 0, err
	}
	defer os.Remove(convPath)
	_ = os.Remove(programSubmittedPath(id))
	readyPath := programReadyPath(dir, id)
	ctlPath := filepath.Join(dir, id+".ctl")
	// A crashed or earlier run with the same id leaves its marker and FIFO
	// behind; a sender must not take them for this run's.
	_ = os.Remove(readyPath)
	_ = os.Remove(ctlPath)
	if err := syscall.Mkfifo(ctlPath, 0o600); err != nil {
		return 0, err
	}
	defer os.Remove(ctlPath)
	if s := os.Getenv(ProgramFifoDelayEnv); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, err
		}
		time.Sleep(d)
	}
	// Read-write so the FIFO never reads EOF between two senders.
	ctl, err := os.OpenFile(ctlPath, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer ctl.Close()
	if err := os.WriteFile(readyPath, nil, 0o644); err != nil {
		return 0, err
	}
	p := &program{out: os.Stdout, trust: os.Getenv(ProgramTrustEnv) == "1", convPath: convPath, convID: id}
	if os.Getenv(ProgramInputEnv) == "1" {
		p.box = newInputBox(os.Stdout, os.Getenv(ProgramDraftEnv))
		if s := os.Getenv(ProgramReadyDelayEnv); s != "" {
			if p.readyDelay, err = time.ParseDuration(s); err != nil {
				return 0, err
			}
		}
	}
	return p.run(os.Stdin, ctl)
}

type program struct {
	out      io.Writer
	trust    bool
	convPath string
	// convID is the id the program launched with, which names its side channels.
	convID string
	// esc and osc track an escape sequence on input, so the terminal's
	// reply to the probe is not echoed as typed text.
	esc, osc bool
	// box is nil unless ProgramInputEnv is set. It takes keys once ready.
	box        *inputBox
	ready      bool
	readyDelay time.Duration
	readyC     <-chan time.Time
}

func (p *program) run(in, ctl io.Reader) (int, error) {
	events := tuiEvents(in, ctl)
	if p.trust {
		fmt.Fprint(p.out, "\x1b[2J\x1b[H"+ProgramTrustPrompt+"\r\n❯ 1. Yes, proceed\r\n  2. No, exit")
	} else {
		p.start()
	}
	for {
		select {
		case <-p.readyC:
			p.readyC = nil
			p.becomeReady()
		case ev, ok := <-events:
			if !ok {
				return 0, nil
			}
			if code, done, err := p.event(ev); done || err != nil {
				return code, err
			}
		}
	}
}

// event handles one event; done means the program exits with code.
func (p *program) event(ev tuiEvent) (code int, done bool, err error) {
	switch {
	case ev.err != nil:
		if ptyClosed(ev.err) {
			return 0, true, nil
		}
		return 0, false, ev.err
	case ev.control != nil:
		if ev.control.Action == ActionExit {
			p.report("clear", "", "")
			return ev.control.Code, true, nil
		}
		return 0, false, p.control(*ev.control)
	case p.trust:
		if ev.key == '\r' || ev.key == '\n' {
			p.trust = false
			p.start()
		}
	default:
		return 0, false, p.key(ev.key)
	}
	return 0, false, nil
}

func (p *program) start() {
	fmt.Fprint(p.out, "\x1b[2J\x1b[H"+harness.ProgramStatusProbe)
	if os.Getenv(ProgramIdleEnv) == "1" {
		p.report("idle", "", "")
	}
	switch {
	case p.box == nil:
	case p.readyDelay > 0:
		p.readyC = time.After(p.readyDelay)
	default:
		p.becomeReady()
	}
}

func (p *program) becomeReady() {
	p.ready = true
	p.box.draw()
}

// key takes a typed key, skipping OSC sequences: into the input box if there
// is one, else echoed.
func (p *program) key(b byte) error {
	switch {
	case p.osc:
		p.osc = b != '\a'
		return nil
	case p.esc:
		p.esc = false
		if b == ']' {
			p.osc = true
			return nil
		}
	case b == escKey:
		p.esc = true
		return nil
	}
	if p.box != nil {
		if !p.ready {
			return nil
		}
		if b == ctrlC {
			return logSubmitted(p.convID, ProgramInterruptEntry)
		}
		text, submitted := p.box.key(b)
		if !submitted {
			return nil
		}
		if err := logSubmitted(p.convID, text); err != nil {
			return err
		}
		if text == ProgramClearCommand {
			return p.setConversation(p.convID + ProgramClearedSuffix)
		}
		return nil
	}
	if b == '\r' || b == '\n' {
		fmt.Fprint(p.out, "\r\n")
		return nil
	}
	_, _ = p.out.Write([]byte{b})
	return nil
}

func (p *program) setConversation(id string) error {
	if err := os.WriteFile(p.convPath, []byte(id), 0o644); err != nil {
		return err
	}
	// Claude redraws on /clear; the holder's watch wakes on that output.
	// An SGR reset stands in for it without changing the screen.
	fmt.Fprint(p.out, "\x1b[0m")
	return nil
}

func (p *program) control(c Control) error {
	if p.box != nil && p.box.control(c) {
		return nil
	}
	switch c.Action {
	case ActionReport:
		p.report(c.State, c.Kind, c.Message)
	case ActionTitle:
		fmt.Fprint(p.out, "\x1b]0;"+c.Title+"\a")
	case ActionWrite:
		fmt.Fprint(p.out, c.Output)
	case ActionConversation:
		return p.setConversation(c.ConversationID)
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
	writeReport(p.out, state, kind, msg)
}

func writeReport(out io.Writer, state, kind, msg string) {
	s := "\x1b]7501;state=" + state + ":app=agentfake"
	if kind != "" {
		s += ":kind=" + kind
	}
	if msg != "" {
		s += ":msg=" + base64.StdEncoding.EncodeToString([]byte(msg))
	}
	fmt.Fprint(out, s+"\a")
}
