package nativerunner

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	RawLogFile   = "raw.log"
	ScreenFile   = "screen.txt"
	ExitCodeFile = "exit-code"
)

// The pty is wide so claude's TUI wraps as little as possible.
const (
	HolderCols = 200
	HolderRows = 50
)

// holderDrainTimeout bounds the wait for the last pty output after the child
// exits: a grandchild that inherited the tty can keep it open forever.
const holderDrainTimeout = time.Second

// holderAfterExit runs once the child has exited, before the drain; tests
// hold the holder there.
var holderAfterExit = func() {}

// RunHolder runs a claude holder; see RunHarnessHolder.
func RunHolder(dir string, argv []string) error {
	return RunHarnessHolder(dir, harness.Claude, argv)
}

// RunHarnessHolder runs argv in a pty, appends its output to raw.log in dir
// and feeds it to a screen, while kind's harness watches the child. Once the
// child exits it saves the last screen and the exit code to dir and returns.
// The caller starts the holder in its own session, so it outlives whoever
// launched it.
func RunHarnessHolder(dir string, kind agentrunner.Kind, argv []string) error {
	if len(argv) == 0 {
		return errors.New("agent-holder: no command to run")
	}
	h, err := harness.Lookup(kind)
	if err != nil {
		return err
	}
	return runHolder(dir, h, argv)
}

func runHolder(dir string, h harness.Harness, argv []string) error {
	rawLog, err := os.OpenFile(filepath.Join(dir, RawLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer rawLog.Close()

	scr := screen.NewScreen(HolderCols, HolderRows)
	defer scr.Close()

	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	defer ptmx.Close()
	// Fd makes reads blocking, so Close never waits on the reader, which a
	// grandchild still holding the tty keeps blocked.
	_ = ptmx.Fd()
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: HolderCols, Rows: HolderRows}); err != nil {
		_ = tty.Close()
		return err
	}
	ln, err := net.Listen("unix", filepath.Join(dir, HolderSocketFile))
	if err != nil {
		_ = tty.Close()
		return err
	}
	// Closing the listener removes holder.sock, so no stale socket outlives
	// the holder.
	defer ln.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	spawned := time.Now()
	if err := cmd.Start(); err != nil {
		_ = tty.Close()
		return err
	}
	exited := make(chan struct{})
	watcher, err := h.Watch(cmd.Process.Pid, spawned, exited)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = tty.Close()
		return err
	}
	// Only claude's pid status backs the frozen status and deny-reason ops.
	pid, _ := watcher.(*harness.PidWatcher)
	srv := &holderServer{
		scr:    scr,
		ptmx:   ptmx,
		proc:   cmd.Process,
		pid:    pid,
		ops:    holderOptionalOps,
		exited: exited,
	}
	go srv.serve(ln)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.MultiWriter(rawLog, scr, &programStatus{ptmx: ptmx, watcher: watcher}), ptmx)
	}()

	waitErr := cmd.Wait()
	srv.exitCode = cmd.ProcessState.ExitCode()
	close(srv.exited)
	holderAfterExit()
	// The holder keeps its own tty fd open until now: on macOS the child's
	// last output is dropped once every tty fd is closed, so it is read out
	// of the pty before the holder lets go.
	deadline := time.Now().Add(holderDrainTimeout)
	for unread(tty) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	_ = tty.Close()
	select {
	case <-drained:
	case <-time.After(holderDrainTimeout):
	}

	var exit *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exit) {
		return waitErr
	}
	if err := os.WriteFile(filepath.Join(dir, ScreenFile), []byte(strings.Join(scr.Rows(), "\n")+"\n"), 0o644); err != nil {
		return err
	}
	// ExitCode is -1 for a child killed by a signal.
	code := strconv.Itoa(srv.exitCode)
	return os.WriteFile(filepath.Join(dir, ExitCodeFile), []byte(code+"\n"), 0o644)
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

// unread reports whether the child wrote output the holder has not read yet.
func unread(tty *os.File) bool {
	n, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCOUTQ)
	return err == nil && n > 0
}
