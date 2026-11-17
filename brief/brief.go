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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/gocachegate"
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
	// args. env is the environment the run uses: executables are looked up on
	// its PATH, and children start with it. An error exits 2.
	Plan(env []string, dir string, args []string) (Invocation, error)
	// Parse reads the raw output of a run that exited with the given code.
	Parse(out []byte, exit int) Outcome
	// RerunArgs returns the caller's args changed to run only the failing
	// tests, or nil when the failures cannot be re-run on their own.
	RerunArgs(args []string, failures []Unit) []string
	// NarrowArgs returns the caller's args changed to run only the named test
	// (without its package), or nil when the tool cannot narrow to one test.
	NarrowArgs(args []string, test string) []string
	// SplitTestName splits a test's index name (a key of Outcome.TestDurations)
	// into its location and test name; the test is empty when the tool does
	// not name tests that way.
	SplitTestName(name string) (loc, test string)
}

// tools lists the wrapped tools by name.
var tools = map[string]Tool{}

func init() {
	for _, t := range []Tool{GoTest{}, GoBuild{}, GoVet{}, Vitest{}, Tsc{}, Biome{}, Playwright{}} {
		tools[t.Name()] = t
	}
}

// ToolByName finds a wrapped tool by its subcommand name.
func ToolByName(name string) (Tool, bool) {
	t, ok := tools[name]
	return t, ok
}

// splitByRe splits name with a two-group regexp; no match gives an empty test.
func splitByRe(re *regexp.Regexp, name string) (loc, test string) {
	if m := re.FindStringSubmatch(name); m != nil {
		return m[1], m[2]
	}
	return "", ""
}

// noSplit is for the tools that report no per-test names.
func noSplit(string) (string, string) { return "", "" }

// Invocation is the command a tool plans to run.
type Invocation struct {
	Bin  string
	Argv []string
	// Note is printed before the result when not empty.
	Note string
	// Env holds KEY=VALUE defaults added to the child's environment. A
	// variable the caller's environment already sets keeps the caller's value.
	Env []string
	// ReportFile is a file the run writes besides its output. After the run a
	// tool that is a ReportReader reads it for test durations; the file is
	// then removed.
	ReportFile string
	// Dir is the child's working directory when the tool chose one other than
	// the directory it was planned for; empty keeps that directory.
	Dir string
}

// ReportReader is a Tool that times its tests from Invocation.ReportFile.
type ReportReader interface {
	ReadReport(path string) map[string]int64
}

// ContextReader is a Tool that also reads a page snapshot for each failing
// test from Invocation.ReportFile.
type ContextReader interface {
	ReadContexts(path string) map[string]string
	// ReadRefs maps a failing test to where to look: its source line and the
	// path of its error-context.md.
	ReadRefs(path string) map[string]FailureRef
}

// FailureRef locates a failing test: Line is "file:line" of the flow, Context
// the path of its error-context.md. Either can be empty.
type FailureRef struct{ Line, Context string }

// Outcome is what a tool found in the output of one run.
type Outcome struct {
	// Units counts what ran (tests, files, ...). Zero means nothing ran.
	Units int
	// Summary is the noun phrase for a pass, such as "12 tests in 3 packages".
	Summary string
	// Failures are the failure units of a failing run, grouped by location.
	Failures []Unit
	// Tests names the tests that ran, passed and failed, as `location.name`. A
	// tool that cannot list the passing ones names the failing ones only.
	Tests []string
	// TestDurations maps a test name (as in Tests) to its duration in
	// milliseconds. A tool that does not time its tests leaves it nil.
	TestDurations map[string]int64
	// Contexts maps a failing test name (as in Tests) to its page snapshot.
	Contexts map[string]string
	// Refs maps a failing test name (as in Tests) to its source line and
	// error-context.md path.
	Refs map[string]FailureRef
	// Listing holds one line per test of a list-only run. It is printed in
	// place of "no tests ran".
	Listing []string
	// Unhandled describes errors outside any test (vitest's unhandled errors)
	// that fail a run whose tests all passed: a count line, then the first
	// message and frame. Empty when there are none.
	Unhandled string
}

// Options configure one run.
type Options struct {
	Dir     string // working directory of the tool
	CDir    string // the -C value as the caller gave it, for the re-run line
	Args    []string
	DataDir string // gx data dir; logs and the index live under it
	Out     io.Writer
	// Env is the child's whole environment; empty means none, never the
	// process's.
	Env []string
	// CheckRun and CheckKind tag the index line with the `gx check` run (and
	// the kind of it) that started this one.
	CheckRun  string
	CheckKind string
	// StateDir holds the Go cache gate lock. Go tools hold it shared while the
	// child runs; empty means no gate.
	StateDir string
	// Attempt is the number of this try on this tree; zero means compute it
	// from the index.
	Attempt int
	// Sample reads the machine load and process count; nil means SystemSample.
	Sample SampleFunc
}

// EnvCheckRun and EnvCheckKind carry a `gx check` run and kind to its children.
const (
	EnvCheckRun  = "GX_CHECK_RUN"
	EnvCheckKind = "GX_CHECK_KIND"
	// EnvCheckHang is the hang limit as a Go duration. `gx brief go-test` kills
	// the run when a test runs longer; unset means nothing is watched.
	EnvCheckHang = "GX_CHECK_HANG"
	// EnvKeepReport set to 1 keeps the report of a failing run next to its
	// log as <log stem>.json. Off by default to spare the data dir.
	EnvKeepReport = "GX_BRIEF_KEEP_REPORT"
)

// IndexEntry is one line of index.jsonl.
type IndexEntry = indexEntry

type indexEntry struct {
	Time       string   `json:"time"`
	Tool       string   `json:"tool"`
	Args       []string `json:"args"`
	Dir        string   `json:"dir"`
	Exit       int      `json:"exit"`
	Units      int      `json:"units"`
	DurationMS int64    `json:"duration_ms"`
	Log        string   `json:"log"`
	Tests      []string `json:"tests"`
	Failed     []string `json:"failed,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	TreeID     string   `json:"tree_id,omitempty"`
	CheckRun   string   `json:"check_run,omitempty"`
	CheckKind  string   `json:"check_kind,omitempty"`
	// Attempt is the try number on this tree; old lines lack it (read as 1).
	Attempt    int     `json:"attempt,omitempty"`
	Signature  string  `json:"signature,omitempty"`
	LoadStart  float64 `json:"load_start"`
	LoadEnd    float64 `json:"load_end"`
	ProcsStart int     `json:"procs_start"`
	ProcsEnd   int     `json:"procs_end"`
	Origin     string  `json:"origin,omitempty"`
	// TestDurations is in milliseconds, keyed like Tests. Old lines lack it.
	TestDurations map[string]int64 `json:"test_durations,omitempty"`
}

// unhandledTestName is the failed entry of a vitest run that exits non-zero
// with every test green.
const unhandledTestName = "vitest unhandled errors"

// StableName is the index name of a test: the same on every run and in every
// worktree. A vitest name loses the run directory from its file path, a
// playwright name loses the `e2e/` prefix, and any name loses the trailing
// ` ───` rule the line reporter prints. Every index field that names a test
// goes through it.
func StableName(tool, dir, name string) string {
	name = strings.TrimRight(name, " ")
	name = strings.TrimRight(name, "─")
	name = strings.TrimRight(name, " ")
	switch tool {
	case "vitest":
		if dir != "" {
			name = strings.TrimPrefix(name, strings.TrimSuffix(dir, "/")+"/")
		}
	case "playwright":
		name = strings.TrimPrefix(name, "e2e/")
	}
	return name
}

// nameTests rewrites Tests, Failed and the keys of TestDurations with
// StableName.
func (e *indexEntry) nameTests(tool, dir string) {
	rename := func(names []string) []string {
		for i, n := range names {
			names[i] = StableName(tool, dir, n)
		}
		return names
	}
	rename(e.Tests)
	rename(e.Failed)
	if e.TestDurations != nil {
		named := make(map[string]int64, len(e.TestDurations))
		for n, ms := range e.TestDurations {
			named[StableName(tool, dir, n)] += ms
		}
		e.TestDurations = named
	}
}

// ReadIndex returns the lines of the index under dataDir, oldest first. A
// missing index is empty; a line that does not decode is skipped.
func ReadIndex(dataDir string) ([]IndexEntry, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "brief", indexName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []IndexEntry
	for _, line := range bytes.Split(data, []byte("\n")) {
		var e IndexEntry
		if len(line) > 0 && json.Unmarshal(line, &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// Run executes the tool and prints its report to o.Out. It returns the exit
// code for the process.
func Run(tool Tool, o Options) int {
	exit, _ := RunOutcome(tool, o)
	return exit
}

// RunOutcome is Run that also returns what the tool found, for callers that
// name the failures themselves. The Outcome is empty when the run could not
// start (exit 2).
func RunOutcome(tool Tool, o Options) (int, Outcome) {
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return cantRun(o.Out, err.Error()), Outcome{}
	}
	plan, err := tool.Plan(o.Env, dir, o.Args)
	if err != nil {
		return cantRun(o.Out, err.Error()), Outcome{}
	}
	if plan.Note != "" {
		fmt.Fprintln(o.Out, plan.Note)
	}
	runDir := dir
	if plan.Dir != "" {
		runDir = plan.Dir
		if rel, err := filepath.Rel(dir, runDir); err == nil {
			o.CDir = filepath.Join(o.CDir, rel)
		}
	}

	start := time.Now()
	logDir := filepath.Join(o.DataDir, "brief")
	logFile, logPath, err := createLog(logDir, start.UTC().Format("20060102T150405.000000000")+"-"+tool.Name())
	if err != nil {
		return cantRun(o.Out, "cannot write log: "+err.Error()), Outcome{}
	}
	defer logFile.Close()
	sweep(logDir, start)

	// Outside a git repo both stay empty.
	branch, _ := git.CurrentBranch(dir)
	treeID, _ := history.TreeID(dir)

	sample := o.Sample
	if sample == nil {
		sample = SystemSample
	}
	indexPath := filepath.Join(logDir, "index.jsonl")
	attempt := o.Attempt
	if attempt == 0 {
		attempt = nextAttempt(indexPath, tool.Name(), treeID, o.Args)
	}
	before := sample()

	hang := hangLimit(tool, o.Env)
	run := func() ([]byte, int, error) {
		releaseGate := registerGoBuild(tool, o.StateDir)
		defer releaseGate()
		return execute(plan.Bin, plan.Argv, runDir, withDefaults(o.Env, plan.Env), logFile, hang)
	}
	out, exit, err := run()
	cacheLost := false
	if err == nil && exit != 0 && isGoTool(tool) && lostCache(out) {
		// Another process cleaned the Go build cache mid-run. One retry
		// rebuilds it; both runs share one log and one index line.
		out, exit, err = run()
		cacheLost = err == nil && exit != 0 && lostCache(out)
	}
	if err != nil {
		return cantRun(o.Out, err.Error()), Outcome{}
	}
	if cacheLost {
		fmt.Fprintln(o.Out, lostCacheMessage)
	}
	elapsed := time.Since(start)
	after := sample()

	outcome := tool.Parse(out, exit)
	if exit == 0 && len(outcome.Failures) > 0 {
		// A tool such as biome exits 0 on a finding; the report decides.
		exit = ExitFail
	}
	if rr, ok := tool.(ReportReader); ok && plan.ReportFile != "" {
		outcome.TestDurations = rr.ReadReport(plan.ReportFile)
		if cr, ok := tool.(ContextReader); ok && exit != 0 {
			outcome.Contexts = cr.ReadContexts(plan.ReportFile)
			outcome.Refs = cr.ReadRefs(plan.ReportFile)
		}
		if keep, _ := envValue(o.Env, EnvKeepReport); keep == "1" && exit != 0 {
			_ = os.Rename(plan.ReportFile, strings.TrimSuffix(logPath, ".log")+".json")
		}
		os.Remove(plan.ReportFile)
	}
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
		Tests:      append([]string{}, outcome.Tests...),
		Branch:     branch,
		TreeID:     treeID,
		CheckRun:   o.CheckRun,
		CheckKind:  o.CheckKind,
		Attempt:    attempt,
		Signature:  signatureOf(exit, outcome, out),
		LoadStart:  before.Load,
		LoadEnd:    after.Load,
		ProcsStart: before.Procs,
		ProcsEnd:   after.Procs,
		Origin:     originOf(o.Env),

		TestDurations: outcome.TestDurations,
	}
	for _, u := range outcome.Failures {
		entry.Failed = append(entry.Failed, u.String())
	}
	if len(outcome.Failures) == 0 && outcome.Unhandled != "" {
		entry.Failed = append(entry.Failed, unhandledTestName)
	}
	entry.nameTests(tool.Name(), runDir)
	outcome.Tests = slices.Clone(entry.Tests)
	outcome.TestDurations = entry.TestDurations
	if err := appendIndex(indexPath, entry); err != nil {
		return cantRun(o.Out, "cannot write index: "+err.Error()), Outcome{}
	}
	return report(tool, o, outcome, out, entry), outcome
}

// hangLimit is the hang limit of a run; zero means nothing is watched. Only
// go test is watched: it is the one tool whose stream shows each test start.
func hangLimit(tool Tool, env []string) time.Duration {
	v, ok := envValue(env, EnvCheckHang)
	if _, isGo := tool.(GoTest); !ok || !isGo {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

func isGoTool(tool Tool) bool { return strings.HasPrefix(tool.Name(), "go-") }

const lostCacheMessage = "brief: the Go build cache was lost (deleted while the build ran), twice. This is not a code error: run the command again."

// lostCacheRe matches `could not import <pkg> (open <GOCACHE>/..-d: no such
// file or directory)`.
var lostCacheRe = regexp.MustCompile(`could not import [^\n]*\(open [^\n]*go-build[^\n]*: no such file or directory\)`)

func lostCache(out []byte) bool { return lostCacheRe.Match(out) }

// registerGoBuild holds the cache gate shared for a Go tool's run. Other tools
// and an empty stateDir take no lock. The gate never fails a build: a state dir
// that cannot be made leaves the run unregistered.
func registerGoBuild(tool Tool, stateDir string) (release func()) {
	if stateDir == "" || !isGoTool(tool) {
		return func() {}
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		log.Printf("gocachegate: build runs unregistered: %v", err)
		return func() {}
	}
	return gocachegate.Register(stateDir)
}

// execute runs the command and returns its combined output and exit code. The
// output is also written to logFile as it arrives, so a killed run keeps its
// partial log.
//
// A positive hang makes it watch the `go test -json` stream and interrupt the
// run when a test runs longer; the hung test is then appended as a failed one.
func execute(exe string, argv []string, dir string, env []string, logFile io.Writer, hang time.Duration) (out []byte, exit int, err error) {
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, logFile)
	var stop <-chan struct{}
	var watcher *hangWatcher
	if hang > 0 {
		watcher = newHangWatcher(hang)
		stop = watcher.stop
		done := make(chan struct{})
		defer close(done)
		go watcher.watch(done)
		w = io.MultiWriter(w, watcher)
	}
	cmd := exec.Command(exe, argv...)
	cmd.Dir = dir
	cmd.Env = envOrEmpty(env)
	cmd.Stdout = w
	cmd.Stderr = w
	if watcher != nil {
		defer func() {
			if extra := watcher.killedLines(); extra != nil {
				buf.Write(extra)
				logFile.Write(extra)
				out, exit = buf.Bytes(), ExitFail
			}
		}()
	}
	if err := runForwardingSignals(cmd, stop); err != nil {
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
		if len(outcome.Failures) == 0 && outcome.Unhandled != "" {
			count, detail, _ := strings.Cut(outcome.Unhandled, "\n")
			fmt.Fprintf(o.Out, "FAIL %s: tests passed, %s in %s\n%s\n", tool.Name(), count, dur, detail)
		} else if len(outcome.Failures) == 0 {
			fmt.Fprintf(o.Out, "FAIL %s: exit %d in %s\n", tool.Name(), e.Exit, dur)
			rawHead(o.Out, out)
		} else {
			fmt.Fprintf(o.Out, "FAIL %s: %d failed in %s\n", tool.Name(), len(outcome.Failures), dur)
			renderUnitsWith(o.Out, outcome.Failures, maxFullUnits, func(u Unit) { renderSnapshot(o.Out, outcome, u) })
			if re := tool.RerunArgs(o.Args, outcome.Failures); re != nil {
				fmt.Fprintln(o.Out, rerunLine(tool.Name(), o.CDir, re))
			}
		}
		renderSlowest(tool, o.Out, outcome.TestDurations)
		fmt.Fprintln(o.Out, e.Log)
		return ExitFail
	case len(outcome.Listing) > 0:
		fmt.Fprintf(o.Out, "LIST %s: %d tests\n%s\n%s\n", tool.Name(), len(outcome.Listing), strings.Join(outcome.Listing, "\n"), e.Log)
	case outcome.Units == 0 && runFilter(tool, o.Args) != "":
		fmt.Fprintf(o.Out, "FAIL %s: -run '%s' matched no tests in %s\n%s\n", tool.Name(), runFilter(tool, o.Args), dur, e.Log)
		return ExitFail
	case outcome.Units == 0:
		fmt.Fprintf(o.Out, "PASS %s: no tests ran in %s\n%s\n", tool.Name(), dur, e.Log)
	default:
		fmt.Fprintf(o.Out, "PASS %s: %s in %s\n", tool.Name(), outcome.Summary, dur)
		renderSlowest(tool, o.Out, outcome.TestDurations)
		fmt.Fprintln(o.Out, e.Log)
	}
	return ExitPass
}

// RunFilterer is a Tool whose -run filter that matches nothing is a failure
// instead of "no tests ran".
type RunFilterer interface {
	// RunFilter returns the caller's -run pattern, or "" when there is none.
	RunFilter(args []string) string
}

func runFilter(tool Tool, args []string) string {
	if rf, ok := tool.(RunFilterer); ok {
		return rf.RunFilter(args)
	}
	return ""
}

func cantRun(w io.Writer, msg string) int {
	fmt.Fprintf(w, "brief: %s\n", msg)
	return ExitCantRun
}

// createLog never reuses a name: two runs of one tool can start in the same
// nanosecond (parallel `make test` units), so a taken name gets a counter.
func createLog(dir, stem string) (*os.File, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	name := stem + ".log"
	for n := 2; ; n++ {
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if !errors.Is(err, os.ErrExist) {
			return f, path, err
		}
		name = fmt.Sprintf("%s-%d.log", stem, n)
	}
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
