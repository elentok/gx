// Package brief runs a developer tool (go test, ...) and prints a short
// report instead of its full output. A Tool builds the command line and
// parses the output; this core owns the log, the index, exit codes and the
// final line.
package brief

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Exit codes of a brief run.
const (
	ExitPass    = 0
	ExitFail    = 1
	ExitCantRun = 2
)

// Tool is one wrapped command.
type Tool interface {
	Name() string
	// Command returns the executable and arguments for the caller's args.
	Command(args []string) (name string, argv []string)
	// Parse reads the raw output of a run that exited with the given code.
	Parse(out []byte, exit int) Outcome
	// RerunArgs returns the caller's args changed to run only the failing
	// tests, or nil when the failures cannot be re-run on their own.
	RerunArgs(args []string, failures []Unit) []string
}

// Outcome is what a tool found in the output of one run.
type Outcome struct {
	// Units counts what ran (tests, files, ...). Zero means nothing ran.
	Units int
	// Summary is the noun phrase for a pass, such as "12 tests in 3 packages".
	Summary string
	// Failures are the failure units of a failing run, grouped by location.
	Failures []Unit
}

// Options configure one run.
type Options struct {
	Dir     string // working directory of the tool
	CDir    string // the -C value as the caller gave it, for the re-run line
	Args    []string
	DataDir string // gx data dir; logs and the index live under it
	Out     io.Writer
}

// indexEntry is one line of index.jsonl.
type indexEntry struct {
	Time       string   `json:"time"`
	Tool       string   `json:"tool"`
	Args       []string `json:"args"`
	Dir        string   `json:"dir"`
	Exit       int      `json:"exit"`
	Units      int      `json:"units"`
	DurationMS int64    `json:"duration_ms"`
	Log        string   `json:"log"`
}

// Run executes the tool and prints its report to o.Out. It returns the exit
// code for the process.
func Run(tool Tool, o Options) int {
	name, argv := tool.Command(o.Args)
	if _, err := exec.LookPath(name); err != nil {
		return cantRun(o.Out, fmt.Sprintf("%s not found on PATH", name))
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return cantRun(o.Out, err.Error())
	}

	start := time.Now()
	logDir := filepath.Join(o.DataDir, "brief")
	logPath := filepath.Join(logDir, start.UTC().Format("20060102T150405.000000000")+"-"+tool.Name()+".log")
	logFile, err := createLog(logDir, logPath)
	if err != nil {
		return cantRun(o.Out, "cannot write log: "+err.Error())
	}
	defer logFile.Close()

	var out bytes.Buffer
	cmd := exec.Command(name, argv...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	exit := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return cantRun(o.Out, fmt.Sprintf("cannot run %s: %v", name, err))
		}
		exit = ee.ExitCode()
	}
	elapsed := time.Since(start)

	if _, err := logFile.Write(out.Bytes()); err != nil {
		return cantRun(o.Out, "cannot write log: "+err.Error())
	}
	outcome := tool.Parse(out.Bytes(), exit)
	if exit != 0 {
		outcome.Units = len(outcome.Failures)
	}
	entry := indexEntry{
		Time:       start.Format(time.RFC3339),
		Tool:       tool.Name(),
		Args:       append([]string{}, o.Args...),
		Dir:        dir,
		Exit:       exit,
		Units:      outcome.Units,
		DurationMS: elapsed.Milliseconds(),
		Log:        logPath,
	}
	if err := appendIndex(filepath.Join(logDir, "index.jsonl"), entry); err != nil {
		return cantRun(o.Out, "cannot write index: "+err.Error())
	}

	dur := elapsed.Round(10 * time.Millisecond)
	switch {
	case exit != 0:
		if len(outcome.Failures) == 0 {
			fmt.Fprintf(o.Out, "FAIL %s: exit %d in %s\n", tool.Name(), exit, dur)
			rawHead(o.Out, out.Bytes())
		} else {
			fmt.Fprintf(o.Out, "FAIL %s: %d failed in %s\n", tool.Name(), len(outcome.Failures), dur)
			renderUnits(o.Out, outcome.Failures)
			if re := tool.RerunArgs(o.Args, outcome.Failures); re != nil {
				fmt.Fprintln(o.Out, rerunLine(tool.Name(), o.CDir, re))
			}
		}
		fmt.Fprintln(o.Out, logPath)
		return ExitFail
	case outcome.Units == 0:
		fmt.Fprintf(o.Out, "PASS %s: no tests ran in %s\n%s\n", tool.Name(), dur, logPath)
	default:
		fmt.Fprintf(o.Out, "PASS %s: %s in %s\n%s\n", tool.Name(), outcome.Summary, dur, logPath)
	}
	return ExitPass
}

func cantRun(w io.Writer, msg string) int {
	fmt.Fprintf(w, "brief: %s\n", msg)
	return ExitCantRun
}

func createLog(dir, path string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
}

// appendIndex writes the whole line in one O_APPEND write so concurrent runs
// never interleave.
func appendIndex(path string, e indexEntry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
