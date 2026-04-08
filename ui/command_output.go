package ui

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type CommandOutput struct {
	Stdout string
	Stderr string
}

func (o CommandOutput) String() string {
	if o.Stdout == "" {
		return o.Stderr
	}
	if o.Stderr == "" {
		return o.Stdout
	}
	return o.Stdout + "\n" + o.Stderr
}

type CommandOutputRecorder struct {
	mu     sync.Mutex
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func NewCommandOutputRecorder() *CommandOutputRecorder {
	return &CommandOutputRecorder{}
}

func (r *CommandOutputRecorder) Attach(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, commandOutputWriter{recorder: r, stderr: false})
	cmd.Stderr = io.MultiWriter(os.Stderr, commandOutputWriter{recorder: r, stderr: true})
}

func (r *CommandOutputRecorder) Output() CommandOutput {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CommandOutput{Stdout: r.stdout.String(), Stderr: r.stderr.String()}
}

type commandOutputWriter struct {
	recorder *CommandOutputRecorder
	stderr   bool
}

func (w commandOutputWriter) Write(p []byte) (int, error) {
	w.recorder.mu.Lock()
	defer w.recorder.mu.Unlock()
	if w.stderr {
		return w.recorder.stderr.Write(p)
	}
	return w.recorder.stdout.Write(p)
}

type CommandOutputLog struct {
	b bytes.Buffer
}

func NewCommandOutputLog() *CommandOutputLog {
	return &CommandOutputLog{}
}

func CommandOutputLogFrom(output string) *CommandOutputLog {
	log := NewCommandOutputLog()
	log.b.WriteString(strings.TrimSpace(output))
	return log
}

func (l *CommandOutputLog) AppendCommand(name string, args []string, output string) {
	if l.b.Len() > 0 {
		l.b.WriteString("\n\n")
	}
	l.b.WriteString("$ ")
	l.b.WriteString(formatCommandForOutput(name, args))
	l.b.WriteString("\n")
	output = strings.TrimSpace(output)
	if output == "" {
		l.b.WriteString("(no output)")
		return
	}
	l.b.WriteString(output)
}

func (l *CommandOutputLog) String() string {
	if l == nil {
		return ""
	}
	return l.b.String()
}

var safeCommandOutputArg = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,\-]+$`)

func formatCommandForOutput(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteCommandOutputArg(name))
	for _, arg := range args {
		parts = append(parts, quoteCommandOutputArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCommandOutputArg(arg string) string {
	if arg != "" && safeCommandOutputArg.MatchString(arg) {
		return arg
	}
	return strconv.Quote(arg)
}
