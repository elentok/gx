package components

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"gx/git"

	"github.com/creack/pty"
)

type CredentialPolicy int

const (
	CredentialPolicyFail CredentialPolicy = iota
	CredentialPolicyPrompt
)

type PromptKind int

const (
	PromptKindText PromptKind = iota
	PromptKindSecret
)

type CredentialPrompt struct {
	Text string
	Kind PromptKind
	seq  int
}

var errPromptAlreadyOpen = errors.New("credential prompt already open")

// CommandRunner executes a command, streams combined output, and supports cancel.
type CommandRunner struct {
	cmdName          string
	args             []string
	dir              string
	credentialPolicy CredentialPolicy

	mu            sync.Mutex
	output        bytes.Buffer
	readPos       int
	cmd           *exec.Cmd
	pty           *os.File
	done          chan error
	resErr        error
	doneSet       bool
	prompt        *CredentialPrompt
	lastPromptSeq int
	detector      credentialPromptDetector
}

func NewCommandRunner(dir, cmdName string, args ...string) *CommandRunner {
	return NewCommandRunnerWithPolicy(dir, cmdName, CredentialPolicyFail, args...)
}

func NewCommandRunnerWithPolicy(dir, cmdName string, policy CredentialPolicy, args ...string) *CommandRunner {
	return &CommandRunner{
		cmdName:          cmdName,
		args:             append([]string{}, args...),
		dir:              dir,
		credentialPolicy: policy,
		done:             make(chan error, 1),
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

func (r *CommandRunner) Prompt() (CredentialPrompt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prompt == nil || r.prompt.seq == r.lastPromptSeq {
		return CredentialPrompt{}, false
	}
	r.lastPromptSeq = r.prompt.seq
	return *r.prompt, true
}

func (r *CommandRunner) SubmitPromptInput(input string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pty == nil || r.prompt == nil {
		return nil
	}
	r.prompt = nil
	r.detector.Reset()
	_, err := io.WriteString(r.pty, input+"\r")
	return err
}

func (r *CommandRunner) ClearPrompt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompt = nil
}

func (r *CommandRunner) run() error {
	if r.credentialPolicy == CredentialPolicyPrompt {
		return r.runPromptable()
	}
	return r.runPlain()
}

func (r *CommandRunner) runPlain() error {
	cmd := exec.Command(r.cmdName, r.args...)
	cmd.Dir = r.dir
	if r.cmdName == "git" {
		cmd.Env = git.NonInteractiveEnv()
	}
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

func (r *CommandRunner) runPromptable() error {
	cmd := exec.Command(r.cmdName, r.args...)
	cmd.Dir = r.dir
	if r.cmdName == "git" {
		cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C", "LC_MESSAGES=C")
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.cmd = cmd
	r.pty = ptmx
	r.mu.Unlock()

	readDone := make(chan error, 1)
	go func() {
		readDone <- r.readPTY(ptmx)
	}()

	waitErr := cmd.Wait()
	_ = ptmx.Close()
	readErr := <-readDone

	r.mu.Lock()
	r.cmd = nil
	r.pty = nil
	r.prompt = nil
	r.mu.Unlock()

	if waitErr != nil {
		return waitErr
	}
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return nil
}

func (r *CommandRunner) readPTY(ptmx *os.File) error {
	buf := make([]byte, 1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			r.appendOutput(string(buf[:n]))
		}
		if err != nil {
			return err
		}
	}
}

func (r *CommandRunner) appendOutput(chunk string) {
	if chunk == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.output.WriteString(chunk)
	if r.prompt != nil {
		r.detector.Feed(chunk)
		return
	}
	if prompt, ok := r.detector.Feed(chunk); ok {
		prompt.seq = r.lastPromptSeq + 1
		r.prompt = &prompt
	}
}

type commandRunnerOutputWriter struct {
	runner *CommandRunner
}

func (w commandRunnerOutputWriter) Write(p []byte) (int, error) {
	w.runner.appendOutput(string(p))
	return len(p), nil
}

var (
	sshPassphrasePrompt = regexp.MustCompile(`Enter passphrase for key '.*':\s*$`)
	httpsUsernamePrompt = regexp.MustCompile(`Username for '.*':\s*$`)
	httpsPasswordPrompt = regexp.MustCompile(`Password for '.*':\s*$`)
	sshPasswordPrompt   = regexp.MustCompile(`(?i)[^\n]*password:\s*$`)
)

func detectCredentialPrompt(tail string) (CredentialPrompt, bool) {
	plain := strings.ReplaceAll(tail, "\r", "")
	switch {
	case sshPassphrasePrompt.MatchString(plain):
		return CredentialPrompt{Text: strings.TrimSpace(plain), Kind: PromptKindSecret}, true
	case httpsUsernamePrompt.MatchString(plain):
		return CredentialPrompt{Text: strings.TrimSpace(plain), Kind: PromptKindText}, true
	case httpsPasswordPrompt.MatchString(plain):
		return CredentialPrompt{Text: strings.TrimSpace(plain), Kind: PromptKindSecret}, true
	case sshPasswordPrompt.MatchString(plain):
		return CredentialPrompt{Text: strings.TrimSpace(plain), Kind: PromptKindSecret}, true
	default:
		return CredentialPrompt{}, false
	}
}

type credentialPromptDetector struct {
	buf strings.Builder
}

func (d *credentialPromptDetector) Reset() {
	d.buf.Reset()
}

func (d *credentialPromptDetector) Feed(chunk string) (CredentialPrompt, bool) {
	for len(chunk) > 0 {
		r, size := utf8.DecodeRuneInString(chunk)
		chunk = chunk[size:]
		switch r {
		case '\n':
			d.buf.Reset()
			continue
		case utf8.RuneError:
			if size == 0 {
				continue
			}
		}
		_, _ = d.buf.WriteRune(r)
		if prompt, ok := detectCredentialPrompt(d.buf.String()); ok {
			d.buf.Reset()
			return prompt, true
		}
	}
	return CredentialPrompt{}, false
}
