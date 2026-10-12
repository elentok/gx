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

	"github.com/elentok/gx/agentrunner/screen"
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

// RunHolder runs argv in a pty, appends its output to raw.log in dir and
// feeds it to a screen. Once the child exits it saves the last screen and
// the exit code to dir and returns. The caller starts the holder in its own
// session, so it outlives whoever launched it.
func RunHolder(dir string, argv []string) error {
	if len(argv) == 0 {
		return errors.New("agent-holder: no command to run")
	}
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
	sessionsDir, err := ClaudeSessionsDir()
	if err != nil {
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
	srv := &holderServer{
		scr:    scr,
		ptmx:   ptmx,
		proc:   cmd.Process,
		pid:    newPidWatcher(sessionsDir, cmd.Process.Pid, spawned),
		ops:    holderOptionalOps,
		exited: make(chan struct{}),
	}
	go srv.serve(ln)
	go srv.pid.watch(srv.exited)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.MultiWriter(rawLog, scr), ptmx)
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

// unread reports whether the child wrote output the holder has not read yet.
func unread(tty *os.File) bool {
	n, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCOUTQ)
	return err == nil && n > 0
}
