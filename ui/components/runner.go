package components

import (
	"bytes"
	"os/exec"
	"sync"
)

// CommandRunner executes a command, streams combined output, and supports cancel.
type CommandRunner struct {
	cmdName string
	args    []string
	dir     string

	mu      sync.Mutex
	output  bytes.Buffer
	readPos int
	cmd     *exec.Cmd
	done    chan error
	resErr  error
	doneSet bool
}

func NewCommandRunner(dir, cmdName string, args ...string) *CommandRunner {
	return &CommandRunner{
		cmdName: cmdName,
		args:    append([]string{}, args...),
		dir:     dir,
		done:    make(chan error, 1),
	}
}

func (r *CommandRunner) Start() {
	go func() {
		r.done <- r.run()
	}()
}

func (r *CommandRunner) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}

func (r *CommandRunner) Consume() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.output.String()
	if r.readPos >= len(s) {
		return ""
	}
	chunk := s[r.readPos:]
	r.readPos = len(s)
	return chunk
}

func (r *CommandRunner) Output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.output.String()
}

func (r *CommandRunner) Result() (error, bool) {
	if r.doneSet {
		return r.resErr, true
	}
	select {
	case err := <-r.done:
		r.resErr = err
		r.doneSet = true
		return err, true
	default:
		return nil, false
	}
}

func (r *CommandRunner) Wait() error {
	if r.doneSet {
		return r.resErr
	}
	err := <-r.done
	r.resErr = err
	r.doneSet = true
	return err
}

func (r *CommandRunner) run() error {
	cmd := exec.Command(r.cmdName, r.args...)
	cmd.Dir = r.dir
	cmd.Stdout = commandRunnerOutputWriter{runner: r}
	cmd.Stderr = commandRunnerOutputWriter{runner: r}

	r.mu.Lock()
	r.cmd = cmd
	r.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()

	r.mu.Lock()
	r.cmd = nil
	r.mu.Unlock()
	return err
}

type commandRunnerOutputWriter struct {
	runner *CommandRunner
}

func (w commandRunnerOutputWriter) Write(p []byte) (int, error) {
	w.runner.mu.Lock()
	defer w.runner.mu.Unlock()
	return w.runner.output.Write(p)
}
