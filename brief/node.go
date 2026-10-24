package brief

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// lockfiles map a lockfile to the frozen install of its package manager. The
// same table lives in skills/gx-verify/SKILL.md.
var lockfiles = []struct{ file, install string }{
	{"pnpm-lock.yaml", "pnpm install --frozen-lockfile"},
	{"package-lock.json", "npm ci"},
	{"bun.lock", "bun install --frozen-lockfile"},
	{"bun.lockb", "bun install --frozen-lockfile"},
	{"yarn.lock", "yarn install --frozen-lockfile"},
}

// findNodeBin walks from dir up to the git root and returns the first
// node_modules/.bin/<tool>. It never falls back to PATH, npx or a package
// manager runner.
func findNodeBin(dir, tool string) (string, error) {
	root, err := gitRoot(dir)
	if err != nil {
		return "", err
	}
	for d := dir; ; d = filepath.Dir(d) {
		bin := filepath.Join(d, "node_modules", ".bin", tool)
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			return bin, nil
		}
		if d == root {
			break
		}
	}
	return "", fmt.Errorf("%s not found in node_modules/.bin; %s", tool, installAdvice(dir, root))
}

// gitRoot returns the nearest directory at or above dir that holds .git.
func gitRoot(dir string) (string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		if d == filepath.Dir(d) {
			return "", errors.New("no git root above " + dir)
		}
	}
}

// installAdvice names the install command of the nearest lockfile at or above
// dir. brief never runs it.
func installAdvice(dir, root string) string {
	for d := dir; ; d = filepath.Dir(d) {
		for _, l := range lockfiles {
			if _, err := os.Stat(filepath.Join(d, l.file)); err == nil {
				return fmt.Sprintf("run `%s` in %s", l.install, d)
			}
		}
		if d == root {
			return "install the dependencies (no lockfile was found)"
		}
	}
}

// Vitest wraps `vitest`.
type Vitest struct{}

func (Vitest) Name() string { return "vitest" }

// Prepare finds the binary and refuses watch mode.
func (Vitest) Prepare(dir string, args []string) (string, string, error) {
	for _, a := range args {
		if a == "--watch" || a == "-w" || strings.HasPrefix(a, "--watch=") {
			return "", "", errors.New("vitest --watch is not supported; brief runs vitest once")
		}
	}
	bin, err := findNodeBin(dir, "vitest")
	if err != nil {
		return "", "", err
	}
	note := ""
	if _, replaced := dropReporter(args); replaced {
		note = "brief: --reporter replaced with json"
	}
	return bin, note, nil
}

// Command runs vitest once with the JSON reporter. `related` keeps its place
// as the subcommand.
func (Vitest) Command(args []string) (string, []string) {
	rest, _ := dropReporter(args)
	argv := []string{"run", "--reporter=json"}
	if len(rest) > 0 {
		switch rest[0] {
		case "run":
			rest = rest[1:]
		case "related":
			argv = []string{"related", "--run", "--reporter=json"}
			rest = rest[1:]
		}
	}
	return "vitest", append(argv, rest...)
}

// dropReporter removes the caller's --reporter and reports whether there was one.
func dropReporter(args []string) ([]string, bool) {
	var out []string
	replaced := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--reporter":
			i++ // its value
			replaced = true
		case strings.HasPrefix(a, "--reporter="):
			replaced = true
		default:
			out = append(out, a)
		}
	}
	return out, replaced
}

// Parse reads the test count and the failing tests from the JSON report. The
// report may follow other output; a report that does not decode (a crashed
// reporter) yields an empty Outcome.
func (Vitest) Parse(out []byte, _ int) Outcome {
	i := bytes.Index(out, []byte(`{"numTotalTestSuites"`))
	if i < 0 {
		return Outcome{}
	}
	var rep struct {
		NumTotalTests int
		TestResults   []struct {
			Name             string
			Status           string
			Message          string
			AssertionResults []struct {
				FullName        string
				Status          string
				FailureMessages []string
			}
		}
	}
	if json.NewDecoder(bytes.NewReader(out[i:])).Decode(&rep) != nil {
		return Outcome{}
	}
	var failures []Unit
	for _, f := range rep.TestResults {
		failed := 0
		for _, a := range f.AssertionResults {
			if a.Status == "failed" {
				failed++
				failures = append(failures, Unit{Location: f.Name, Name: a.FullName, Text: vitestText(a.FailureMessages)})
			}
		}
		if failed == 0 && f.Status == "failed" {
			// The file failed without a failing test, such as an import error.
			failures = append(failures, Unit{Location: f.Name, Name: f.Name, Text: vitestText([]string{f.Message}), Kind: KindFileFailed})
		}
	}
	return Outcome{
		Units:    rep.NumTotalTests,
		Summary:  fmt.Sprintf("%d tests", rep.NumTotalTests),
		Failures: failures,
	}
}

// vitestText joins failure messages and drops the stack frames, which would
// use up the line limit before the message is read.
func vitestText(msgs []string) string {
	var lines []string
	for _, m := range msgs {
		for _, l := range strings.Split(strings.TrimRight(m, "\n"), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(l), "at ") {
				lines = append(lines, l)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// RerunArgs drops any test-name filter of the caller and adds an anchored one
// covering all failing tests.
func (Vitest) RerunArgs(args []string, failures []Unit) []string {
	var names []string
	for _, u := range failures {
		if u.Kind == KindTest {
			names = append(names, regexp.QuoteMeta(u.Name))
		}
	}
	if len(names) == 0 {
		return nil
	}
	var re []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-t" || a == "--testNamePattern":
			i++ // its value
		case strings.HasPrefix(a, "--testNamePattern="):
		default:
			re = append(re, a)
		}
	}
	return append(re, "-t", "^("+strings.Join(names, "|")+")$")
}

// Tsc wraps `tsc`.
type Tsc struct{}

func (Tsc) Name() string { return "tsc" }

// Prepare finds the binary and checks that there is a tsconfig to compile.
func (Tsc) Prepare(dir string, args []string) (string, string, error) {
	bin, err := findNodeBin(dir, "tsc")
	if err != nil {
		return "", "", err
	}
	config := filepath.Join(dir, "tsconfig.json")
	if i := slices.IndexFunc(args, func(a string) bool { return a == "-p" || a == "--project" }); i >= 0 && i+1 < len(args) {
		config = args[i+1]
		if !filepath.IsAbs(config) {
			config = filepath.Join(dir, config)
		}
	}
	if st, err := os.Stat(config); err == nil && st.IsDir() {
		config = filepath.Join(config, "tsconfig.json")
	}
	if _, err := os.Stat(config); err != nil {
		return "", "", fmt.Errorf("no tsconfig found at %s", config)
	}
	return bin, "", nil
}

func (Tsc) Command(args []string) (string, []string) {
	return "tsc", append([]string{"--pretty", "false"}, args...)
}

var tscDiagnostic = regexp.MustCompile(`^(.+?)(\(\d+,\d+\): (?:error|warning) TS\d+: .*)$`)

// Parse treats a clean exit as a pass; tsc prints nothing then. Otherwise it
// groups the diagnostics by file, in order of first appearance. Indented lines
// continue the diagnostic above them; any other line is not part of a file.
func (Tsc) Parse(out []byte, exit int) Outcome {
	if exit == 0 {
		return Outcome{Units: 1, Summary: "no type errors"}
	}
	var failures []Unit
	byFile := map[string]int{}
	counts := map[string]int{}
	cur := -1
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimRight(l, "\r")
		if m := tscDiagnostic.FindStringSubmatch(l); m != nil {
			i, ok := byFile[m[1]]
			if !ok {
				i = len(failures)
				byFile[m[1]] = i
				failures = append(failures, Unit{Location: m[1]})
			}
			counts[m[1]]++
			failures[i].Text += m[2] + "\n"
			cur = i
		} else if cur >= 0 && strings.HasPrefix(l, " ") {
			failures[cur].Text += l + "\n"
		} else {
			cur = -1
		}
	}
	for i := range failures {
		u := &failures[i]
		u.Name = fmt.Sprintf("%d errors", counts[u.Location])
		if counts[u.Location] == 1 {
			u.Name = "1 error"
		}
		u.Text = strings.TrimRight(u.Text, "\n")
	}
	return Outcome{Failures: failures}
}

func (Tsc) RerunArgs([]string, []Unit) []string { return nil }
