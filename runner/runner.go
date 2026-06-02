// Package runner runs a child command with inherited stdio and, when the
// command fails, keeps the surrounding pane open: it prints a footer naming the
// failed command and its exit code, then blocks until the user presses Enter.
//
// It is the error-resilience core behind `gx run` (see docs/adr/0004). It knows
// nothing about cobra or terminal detection so it can be unit-tested in
// isolation with fake stdio.
package runner

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// IO bundles the streams a run is wired to. The child inherits all three; the
// failure footer is written to Err and the dismiss key is read from In.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Run executes program with args, wiring the child's stdin/stdout/stderr to
// io. On a zero exit it returns 0 and writes nothing of its own. On a non-zero
// exit (or a failure to start the child) it writes the failure footer to
// io.Err and blocks reading a line from io.In before returning the child's
// exit code, so a pane launched for the command stays visible until dismissed.
func Run(program string, args []string, io IO) int {
	cmd := exec.Command(program, args...)
	cmd.Stdin = io.In
	cmd.Stdout = io.Out
	cmd.Stderr = io.Err

	err := cmd.Run()
	if err == nil {
		return 0
	}

	code := 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else {
		// The child never ran (e.g. program not found); surface why.
		fmt.Fprintf(io.Err, "\ngx: %v\n", err)
	}

	writeFooter(io.Err, code, program, args)
	waitForEnter(io.In)
	return code
}

// writeFooter prints the separator, the failure line with the exit code, the
// exact command that ran, and the dismiss prompt.
func writeFooter(w io.Writer, code int, program string, args []string) {
	const rule = "─────────────────────────────────"
	fmt.Fprintf(w, "\n%s\n", rule)
	fmt.Fprintf(w, "gx: command failed (exit %d)\n", code)
	fmt.Fprintf(w, "$ %s\n", QuoteCommand(program, args))
	fmt.Fprint(w, "press Enter to close…")
}

// waitForEnter blocks until a newline (or EOF) arrives on r.
func waitForEnter(r io.Reader) {
	if r == nil {
		return
	}
	bufio.NewReader(r).ReadString('\n')
}

// QuoteCommand renders program plus args as a single shell-style command line
// for display only: arguments containing whitespace or quotes are wrapped in
// single quotes (with embedded single quotes escaped). It is not meant to be
// re-parsed by a shell.
func QuoteCommand(program string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(program))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
