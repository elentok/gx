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
	// Plan finds the executable and builds its arguments for the caller's
	// args. The optional note is printed before the result. An error exits 2.
	Plan(dir string, args []string) (exe string, argv []string, note string, err error)
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
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return cantRun(o.Out, err.Error())
	}
	exe, argv, note, err := tool.Plan(dir, o.Args)
	if err != nil {
		return cantRun(o.Out, err.Error())
	}
	if note != "" {
		fmt.Fprintln(o.Out, note)
	}

	start := time.Now()
	logDir := filepath.Join(o.DataDir, "brief")
	logPath := filepath.Join(logDir, start.UTC().Format("20060102T150405.000000000")+"-"+tool.Name()+".log")
	logFile, err := createLog(logDir, logPath)
	if err != nil {
		return cantRun(o.Out, "cannot write log: "+err.Error())
	}
	defer logFile.Close()
	sweep(logDir, start)

	out, exit, err := execute(exe, argv, dir, logFile)
	if err != nil {
		return cantRun(o.Out, err.Error())
	}
	elapsed := time.Since(start)

	outcome := tool.Parse(out, exit)
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
	return report(tool, o, outcome, out, entry)
}

// execute runs the command and returns its combined output and exit code. The
// output is also written to logFile as it arrives, so a killed run keeps its
// partial log.
func execute(exe string, argv []string, dir string, logFile io.Writer) (out []byte, exit int, err error) {
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, logFile)
	cmd := exec.Command(exe, argv...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w
	if err := runForwardingSignals(cmd); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil, 0, fmt.Errorf("cannot run %s: %v", exe, err)
		}
		exit = ee.ExitCode()
		if exit < 0 { // killed by a signal
			exit = ExitFail
		}
	}
	return buf.Bytes(), exit, nil
}

// report prints the final lines of a run and returns the process exit code.
func report(tool Tool, o Options, outcome Outcome, out []byte, e indexEntry) int {
	dur := (time.Duration(e.DurationMS) * time.Millisecond).Round(10 * time.Millisecond)
	switch {
	case e.Exit != 0:
		if len(outcome.Failures) == 0 {
			fmt.Fprintf(o.Out, "FAIL %s: exit %d in %s\n", tool.Name(), e.Exit, dur)
			rawHead(o.Out, out)
		} else {
			fmt.Fprintf(o.Out, "FAIL %s: %d failed in %s\n", tool.Name(), len(outcome.Failures), dur)
			renderUnits(o.Out, outcome.Failures)
			if re := tool.RerunArgs(o.Args, outcome.Failures); re != nil {
				fmt.Fprintln(o.Out, rerunLine(tool.Name(), o.CDir, re))
			}
		}
		fmt.Fprintln(o.Out, e.Log)
		return ExitFail
	case outcome.Units == 0:
		fmt.Fprintf(o.Out, "PASS %s: no tests ran in %s\n%s\n", tool.Name(), dur, e.Log)
	default:
		fmt.Fprintf(o.Out, "PASS %s: %s in %s\n%s\n", tool.Name(), outcome.Summary, dur, e.Log)
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
	unlock, err := lockIndex(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
