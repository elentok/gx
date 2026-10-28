package nativerunner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

// Files the holder keeps in a pty agent directory.
const (
	RawLogFile = "raw.log"
	ScreenFile = "screen.txt"
	// LastScreenFile is the last screen as styled ANSI rows.
	LastScreenFile = "last-screen.ansi"
	ExitCodeFile   = "exit-code"
)

// AgentSessionEnv carries the gx session id into the child's env.
const AgentSessionEnv = "GX_AGENT_SESSION"

// The pty is wide so claude's TUI wraps as little as possible.
const (
	HolderCols = 200
	HolderRows = 50
)

// holderFlushTimeout bounds the wait for the last pty output after the child
// exits: a grandchild that inherited the tty can keep it open forever.
const holderFlushTimeout = time.Second

// holderAfterExit runs once the child has exited, before the final read; tests
// hold the holder there.
var holderAfterExit = func() {}

// RunHolder runs a claude holder; see RunHarnessHolder.
func RunHolder(dir string, argv []string) error {
	return RunHarnessHolder(dir, harness.Claude, HolderOptions{}, argv)
}

// HolderOptions are a holder's optional parts.
type HolderOptions struct {
	// GXSession, when set, goes into the child's env as AgentSessionEnv.
	GXSession string
	// Cols and Rows size the pty; zero means HolderCols and HolderRows.
	Cols, Rows int
	// Trust answers the harness's trust dialog while the child starts.
	Trust bool
	// Env (KEY=VALUE) is appended to the child's env.
	Env []string
}

func (o HolderOptions) size() (cols, rows int) {
	cols, rows = o.Cols, o.Rows
	if cols <= 0 {
		cols = HolderCols
	}
	if rows <= 0 {
		rows = HolderRows
	}
	return cols, rows
}

// RunHarnessHolder runs argv in a pty, appends its output to raw.log in dir
// and feeds it to a screen, while kind's harness watches the child. Once the
// child exits it saves the last screen and the exit code to dir and returns.
// The caller starts the holder in its own session, so it outlives whoever
// launched it.
func RunHarnessHolder(dir string, kind agentrunner.Kind, opts HolderOptions, argv []string) error {
	if len(argv) == 0 {
		return errors.New("agent-holder: no command to run")
	}
	h, err := harness.Lookup(kind)
	if err != nil {
		return err
	}
	return runHolder(dir, h, opts, argv)
}

func runHolder(dir string, h harness.Harness, opts HolderOptions, argv []string) error {
	rawLog, err := os.OpenFile(filepath.Join(dir, RawLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer rawLog.Close()

	cols, rows := opts.size()
	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	defer ptmx.Close()
	// Fd makes reads blocking, so Close never waits on the reader, which a
	// grandchild still holding the tty keeps blocked.
	_ = ptmx.Fd()
	// The screen answers the child's terminal queries and encodes attach
	// input, so the child gets a real terminal's replies.
	scr := screen.NewScreenWithReplies(cols, rows, ptmx)
	defer scr.Close()
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}); err != nil {
		_ = tty.Close()
		return err
	}
	sock := filepath.Join(dir, HolderSocketFile)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		_ = tty.Close()
		if errors.Is(err, syscall.EINVAL) {
			// bind reports an over-long path as a bare EINVAL.
			return fmt.Errorf("%w (unix socket path is %d bytes; the OS caps it near 100)", err, len(sock))
		}
		return err
	}
	// Closing the listener removes holder.sock, so no stale socket outlives
	// the holder.
	defer ln.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.Env = append(terminalEnv(os.Environ()), opts.Env...)
	if opts.GXSession != "" {
		cmd.Env = append(cmd.Env, AgentSessionEnv+"="+opts.GXSession)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	spawned := time.Now()
	if err := cmd.Start(); err != nil {
		_ = tty.Close()
		return err
	}
	exited := make(chan struct{})
	watcher, err := h.Watch(cmd.Process.Pid, spawned, scr, exited)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = tty.Close()
		return err
	}
	pid, _ := watcher.(harness.PidStatusWatcher)
	srv := &holderServer{
		scr:     scr,
		ptmx:    ptmx,
		proc:    cmd.Process,
		pid:     pid,
		watcher: watcher,
		name:    &displayName{read: h.DisplayName},
		frames: newFrameHub(scr, cols, rows, func(cols, rows int) error {
			// The screen first, so it already has the new size when the child
			// redraws for it.
			scr.Resize(cols, rows)
			return pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
		}),
		ops:      holderOptionalOps,
		exited:   exited,
		finished: make(chan struct{}),
	}
	watcher.OnChange(srv.changes.notify)
	// Runs before ln.Close, so watch streams get their exit before the
	// socket goes.
	defer srv.finish()
	go srv.serve(ln)
	if opts.Trust {
		go answerTrust(h, scr, ptmx, watcher, exited)
	}
	output := make(outputSignal, 1)
	go srv.frames.run(output, srv.finished)
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		_, _ = io.Copy(io.MultiWriter(rawLog, scr, output, srv.name, &programStatus{ptmx: ptmx, watcher: watcher}, &srv.changes), ptmx)
	}()

	exitCode, waitErr := waitChild(cmd.Process.Pid)
	srv.exitCode = exitCode
	close(srv.exited)
	holderAfterExit()
	// The holder keeps its own tty fd open until now: on macOS the child's
	// last output is dropped once every tty fd is closed, so it is read out
	// of the pty before the holder lets go.
	deadline := time.Now().Add(holderFlushTimeout)
	for unread(tty) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	_ = tty.Close()
	select {
	case <-copied:
	case <-time.After(holderFlushTimeout):
	}

	if waitErr != nil {
		return waitErr
	}
	if err := os.WriteFile(filepath.Join(dir, ScreenFile), []byte(strings.Join(scr.Rows(), "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, LastScreenFile), []byte(strings.Join(scr.StyledRows(), "\n")+"\n"), 0o644); err != nil {
		return err
	}
	// ExitCode is -1 for a child killed by a signal.
	code := strconv.Itoa(srv.exitCode)
	return os.WriteFile(filepath.Join(dir, ExitCodeFile), []byte(code+"\n"), 0o644)
}

// waitChild waits for the child pid to exit and returns its exit code, -1 for
// a child killed by a signal. It resumes the child each time it stops: the
// child leads its own session with no shell to fg it, so a ctrl+z would
// leave it stopped for good.
func waitChild(pid int) (int, error) {
	for {
		var ws syscall.WaitStatus
		_, err := syscall.Wait4(pid, &ws, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return -1, err
		}
		// On BSD, Go reports a SIGSTOP stop as Continued, not Stopped.
		// Without WCONTINUED, Wait4 never reports a real continue.
		if ws.Stopped() || ws.Continued() {
			_ = syscall.Kill(-pid, syscall.SIGCONT)
			continue
		}
		return ws.ExitStatus(), nil
	}
}

// terminalEnv is env with the TERM and COLORTERM of the holder's screen, the
// child's terminal, in place of whatever the server had: launchd gives it no
// TERM at all, and without one the child prints no colors.
func terminalEnv(env []string) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		return strings.HasPrefix(kv, "TERM=") || strings.HasPrefix(kv, "COLORTERM=")
	})
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
}

// answerTrust types the trust dialog's key until the child is past starting
// up. As in Pty.Start, keys repeat while the dialog stays up, since claude
// drops a key typed before it reads input.
func answerTrust(h harness.Harness, scr *screen.Screen, keys io.Writer, watcher harness.Watcher, exited <-chan struct{}) {
	var next time.Time
	for {
		st := watcher.Status()
		if st.State != harness.Starting && st.Blocked != harness.BlockedDialog {
			return
		}
		if !time.Now().Before(next) {
			if startup := h.StartupScreen(scr.ScrollbackTail(startupScrollback), scr.Rows()); startup.Dialog == harness.TrustDialog {
				if _, err := io.WriteString(keys, startup.Key); err != nil {
					return
				}
				next = time.Now().Add(trustKeyRepeat)
			}
		}
		select {
		case <-exited:
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// programStatus makes the holder the child's terminal for OSC 7501, whatever
// the harness: it answers each probe on the pty and hands each report to the
// watcher.
type programStatus struct {
	parser  harness.ProgramStatusParser
	ptmx    io.Writer
	watcher harness.Watcher
}

func (s *programStatus) Write(p []byte) (int, error) {
	probes, reports := s.parser.Feed(p)
	for range probes {
		_, _ = io.WriteString(s.ptmx, harness.ProgramStatusProbe)
	}
	for _, r := range reports {
		s.watcher.Program(r)
	}
	return len(p), nil
}

// displayNameTail is how much earlier output displayName keeps, so a title
// split across two reads is still seen whole.
const displayNameTail = 1024

// displayName keeps the harness's Display name of the child's output.
type displayName struct {
	read func(raw []byte) (string, bool)
	mu   sync.Mutex
	tail []byte
	name string
}

func (d *displayName) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	buf := append(d.tail, p...)
	if name, ok := d.read(buf); ok {
		d.name = name
	}
	d.tail = bytes.Clone(buf[max(len(buf)-displayNameTail, 0):])
	return len(p), nil
}

func (d *displayName) get() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.name
}

// unread reports whether the child wrote output the holder has not read yet.
func unread(tty *os.File) bool {
	n, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCOUTQ)
	return err == nil && n > 0
}
