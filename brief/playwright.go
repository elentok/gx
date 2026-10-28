package brief

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
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
	rest = flowNumbersToGrep(rest)
	argv := []string{"test", "--reporter=line"}
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
	return Invocation{Bin: bin, Argv: append(argv, rest...), Note: note}, nil
}

// flowNumbersToGrep replaces bare flow numbers (`disk.e2e.ts 69 70`) with one
// -g pattern. Playwright matches -g against "[project] › file:line:col › title",
// so a "^" anchor on the bare title never matches; "› " is the anchor instead.
// A number after a flag is that flag's value (`--workers 2`) and stays.
func flowNumbersToGrep(args []string) []string {
	var out, nums []string
	for i, a := range args {
		if isDigits(a) && (i == 0 || !strings.HasPrefix(args[i-1], "-")) {
			nums = append(nums, a)
			continue
		}
		out = append(out, a)
	}
	if len(nums) == 0 {
		return args
	}
	return append(out, "-g", `› (`+strings.Join(nums, "|")+`)\. `)
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
	passed, failed := 0, 0
	for l := range strings.SplitSeq(string(bytes.TrimRight(out, "\n")), "\n") {
		l = strings.TrimRight(l, "\r")
		if m := playwrightFailure.FindStringSubmatch(l); m != nil {
			closeUnit()
			cur, lines, cut = &Unit{Location: m[1], Name: m[2]}, nil, false
			continue
		}
		if m := playwrightTotals.FindStringSubmatch(l); m != nil {
			closeUnit()
			n, _ := strconv.Atoi(m[1])
			switch m[2] {
			case "passed":
				passed = n
			case "failed":
				failed = n
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
