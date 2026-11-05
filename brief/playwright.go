package brief

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const playwrightConfig = "e2e/playwright.config.ts"

// buildWeb builds the web UI. It is a variable so tests do not run make.
var buildWeb = func(env []string, root string) error {
	cmd := exec.Command("make", "web-build")
	cmd.Dir = root
	cmd.Env = envOrEmpty(env)
	if out, err := cmd.CombinedOutput(); err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("make web-build failed: %v\n%s", err, strings.Join(lines[max(0, len(lines)-10):], "\n"))
	}
	return nil
}

// Playwright wraps `playwright test`.
type Playwright struct{}

func (Playwright) Name() string { return "playwright" }

// Plan finds the binary, adds the e2e config unless the caller gave one, and
// runs the line reporter. The gx binary embeds webui/dist, so a missing
// dist/index.html triggers `make web-build`; the global setup rebuilds a
// stale dist, so building here only covers a missing one.
func (Playwright) Plan(env []string, dir string, args []string) (Invocation, error) {
	bin, err := findNodeBin(dir, "playwright")
	if err != nil {
		return Invocation{}, err
	}
	rest, replaced := stripFlag(args, true, "reporter")
	note := ""
	if replaced {
		note = "brief: --reporter replaced with line"
	}
	rest, err = flowNumbersToGrep(rest)
	if err != nil {
		return Invocation{}, err
	}
	// The json reporter writes to a file, so the line report on stdout is
	// unchanged. Playwright creates the file; the run reads and removes it.
	report := filepath.Join(os.TempDir(), fmt.Sprintf("gx-brief-playwright-%d-%d.json", os.Getpid(), time.Now().UnixNano()))
	argv := []string{"test", "--reporter=line,json"}
	if !slices.ContainsFunc(rest, func(a string) bool {
		return a == "-c" || a == "--config" || strings.HasPrefix(a, "--config=")
	}) {
		if _, err := os.Stat(filepath.Join(dir, playwrightConfig)); err != nil {
			return Invocation{}, fmt.Errorf("no %s under %s; run brief with -C webui", playwrightConfig, dir)
		}
		argv = append(argv, "-c", playwrightConfig)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "index.html")); err != nil {
		root, err := gitRoot(dir)
		if err != nil {
			return Invocation{}, err
		}
		if err := buildWeb(env, root); err != nil {
			return Invocation{}, err
		}
	}
	return Invocation{
		Bin: bin, Argv: append(argv, rest...), Note: note,
		Env: []string{"PLAYWRIGHT_JSON_OUTPUT_NAME=" + report}, ReportFile: report,
	}, nil
}

// ReadReport returns the duration in milliseconds of each test in the JSON
// report at path, keyed like Outcome.Tests. A missing or unreadable report
// yields nil.
func (Playwright) ReadReport(path string) map[string]int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	type suite struct {
		Title  string
		File   string
		Suites []suite
		Specs  []struct {
			Title string
			File  string
			Tests []struct {
				Results []struct{ Duration float64 }
			}
		}
	}
	var rep struct{ Suites []suite }
	if json.Unmarshal(data, &rep) != nil {
		return nil
	}
	var durations map[string]int64
	var walk func(s suite, titles []string)
	walk = func(s suite, titles []string) {
		for _, sp := range s.Specs {
			var ms float64
			for _, t := range sp.Tests {
				for _, r := range t.Results {
					ms += r.Duration
				}
			}
			if durations == nil {
				durations = map[string]int64{}
			}
			name := strings.Join(append(slices.Clone(titles), sp.Title), " › ")
			durations[Unit{Location: sp.File, Name: name}.String()] += int64(math.Round(ms))
		}
		for _, c := range s.Suites {
			walk(c, append(slices.Clone(titles), c.Title))
		}
	}
	// A top-level suite is the file; its title is not part of a test's name.
	for _, file := range rep.Suites {
		walk(file, nil)
	}
	return durations
}

// flowNumbersToGrep replaces bare flow numbers (`disk.e2e.ts 69 70`) with one
// -g pattern. Playwright matches -g against "[project] › file:line:col › title",
// so a "^" anchor on the bare title never matches; "› " is the anchor instead.
// A number after a value flag is that flag's value (`--workers 2`) and stays;
// a number after a boolean flag (`--headed 69`) is a flow. Mixing bare numbers
// with a caller's -g/--grep is an error rather than a second -g.
func flowNumbersToGrep(args []string) ([]string, error) {
	var out, nums []string
	for i, a := range args {
		if isDigits(a) && (i == 0 || !takesValue(args[i-1])) {
			nums = append(nums, a)
			continue
		}
		out = append(out, a)
	}
	if len(nums) == 0 {
		return args, nil
	}
	if slices.ContainsFunc(args, isGrepFlag) {
		return nil, fmt.Errorf("brief: bare flow numbers %s cannot be mixed with -g/--grep; use one or the other", strings.Join(nums, " "))
	}
	return append(out, "-g", `› (`+strings.Join(nums, "|")+`)\. `), nil
}

// playwrightBoolFlags are the `playwright test` flags that take no value, so a
// number after one is not its value. `--debug` counts: its optional mode is a
// word, never a number. Pinned against `playwright test --help`; update it
// when the Playwright version adds a flag.
var playwrightBoolFlags = []string{
	"--debug", "--fail-on-flaky-tests", "--forbid-only", "--fully-parallel", "--headed",
	"--ignore-snapshots", "--last-failed", "--list", "--no-deps", "--pass-with-no-tests",
	"--quiet", "--ui",
}

// takesValue reports whether a number after arg is arg's value.
func takesValue(arg string) bool {
	return strings.HasPrefix(arg, "-") && !slices.Contains(playwrightBoolFlags, arg)
}

func isGrepFlag(a string) bool {
	return a == "-g" || a == "--grep" || strings.HasPrefix(a, "--grep=")
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

var (
	// playwrightFailure matches the heading of a failure block, such as
	// `  1) [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right ───`.
	playwrightFailure = regexp.MustCompile(`^\s+\d+\) (?:\[[^\]]+\] › )?(\S+?):\d+:\d+ › (.*?) ─+\s*$`)
	// playwrightTotals matches the counts that follow the failure blocks.
	playwrightTotals = regexp.MustCompile(`^\s+(\d+) (failed|passed|flaky|skipped|did not run)\b`)
	playwrightCode   = regexp.MustCompile(`^\s*>?\s*\d+ \|`)
	// playwrightListed matches a test line of the list under "N failed", such
	// as `    [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right`.
	playwrightListed = regexp.MustCompile(`^\s+(?:\[[^\]]+\] › )?(\S+?):\d+:\d+ › (.+?)\s*$`)
)

// Parse reads the failure blocks and the counts from the line reporter. Each
// failure keeps only its first error: the text stops at the call log, the code
// frame or the first stack line.
func (Playwright) Parse(out []byte, _ int) Outcome {
	var failures []Unit
	var cur *Unit
	var lines []string
	closeUnit := func() {
		if cur != nil {
			cur.Text = strings.TrimSpace(strings.Join(lines, "\n"))
			failures = append(failures, *cur)
			cur = nil
		}
	}
	cut := false
	listing := false
	passed, failed := 0, 0
	for l := range strings.SplitSeq(string(bytes.TrimRight(out, "\n")), "\n") {
		l = strings.TrimRight(l, "\r")
		if m := playwrightFailure.FindStringSubmatch(l); m != nil {
			closeUnit()
			cur, lines, cut, listing = &Unit{Location: m[1], Name: m[2]}, nil, false, false
			continue
		}
		if m := playwrightTotals.FindStringSubmatch(l); m != nil {
			closeUnit()
			n, _ := strconv.Atoi(m[1])
			listing = m[2] == "failed"
			switch m[2] {
			case "passed":
				passed = n
			case "failed":
				failed = n
			}
			continue
		}
		if m := playwrightListed.FindStringSubmatch(l); listing && m != nil {
			// A failure whose block heading did not parse is still named here.
			if !slices.ContainsFunc(failures, func(u Unit) bool { return u.Location == m[1] && u.Name == m[2] }) {
				failures = append(failures, Unit{Location: m[1], Name: m[2]})
			}
			continue
		}
		if cur == nil || cut {
			continue
		}
		if t := strings.TrimSpace(l); t == "Call log:" || strings.HasPrefix(t, "at ") || playwrightCode.MatchString(l) {
			cut = true
			continue
		}
		lines = append(lines, strings.TrimPrefix(l, "    "))
	}
	closeUnit()
	if passed+failed == 0 && len(failures) == 0 {
		return Outcome{}
	}
	// The line reporter names only the failing tests.
	var ran []string
	for _, u := range failures {
		ran = append(ran, u.String())
	}
	return Outcome{Units: passed + failed, Summary: fmt.Sprintf("%d tests", passed+failed), Failures: failures, Tests: ran}
}

// RerunArgs is nil: a name filter would have to be rebuilt as a -g regex.
func (Playwright) RerunArgs([]string, []Unit) []string { return nil }

func (Playwright) SplitTestName(name string) (string, string) {
	return splitByRe(nodeTestNameRe, name)
}

// NarrowArgs runs one test. Bare flow numbers become a -g first, then every
// -g is replaced. The pattern ends at the title with `$` but has no `^`: see
// flowNumbersToGrep for what -g is matched against.
func (Playwright) NarrowArgs(args []string, test string) []string {
	args, err := flowNumbersToGrep(args)
	if err != nil {
		return nil
	}
	re, _ := stripFlag(args, true, "g", "grep")
	return append(re, "-g", "› "+regexp.QuoteMeta(test)+"$")
}
