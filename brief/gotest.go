package brief

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// GoTest wraps `go test`.
type GoTest struct{}

func (GoTest) Name() string { return "go-test" }

// Plan always adds -json and -v, once. Flags after -args belong to the test
// binary and pass through untouched.
func (GoTest) Plan(_ string, args []string) (Invocation, error) {
	exe, err := exec.LookPath("go")
	if err != nil {
		return Invocation{}, errors.New("go not found on PATH")
	}
	rest, tail := splitArgs(args)
	rest, _ = stripFlag(rest, false, "json", "v")
	argv := append([]string{"test", "-json", "-v"}, rest...)
	return Invocation{Bin: exe, Argv: append(argv, tail...)}, nil
}

type goTestEvent struct {
	Action      string
	Package     string
	Test        string
	Output      string
	ImportPath  string // build-output and build-fail events
	FailedBuild string // package fail event of a package that did not build
}

// goPkg collects the events of one package.
type goPkg struct {
	name        string
	output      []string // package-level output
	failedBuild string
	failed      bool
	tests       []string // failed tests, in failure order
	testOutput  map[string][]string
}

// Parse counts finished tests and packages from the -json stream and collects
// the failure units. Lines that are not JSON are ignored.
func (GoTest) Parse(out []byte, _ int) Outcome {
	tests := 0
	pkgs := map[string]*goPkg{}
	var order []*goPkg
	pkg := func(name string) *goPkg {
		p := pkgs[name]
		if p == nil {
			p = &goPkg{name: name, testOutput: map[string][]string{}}
			pkgs[name] = p
			order = append(order, p)
		}
		return p
	}
	buildText := map[string][]string{}
	finished := map[string]bool{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		var ev goTestEvent
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev.Action {
		case "build-output":
			buildText[ev.ImportPath] = append(buildText[ev.ImportPath], strings.TrimRight(ev.Output, "\n"))
		case "output":
			p := pkg(ev.Package)
			l := strings.TrimRight(ev.Output, "\n")
			if ev.Test == "" {
				p.output = append(p.output, l)
			} else if !isGoTestNoise(l) {
				p.testOutput[ev.Test] = append(p.testOutput[ev.Test], l)
			}
		case "pass", "fail":
			p := pkg(ev.Package)
			if ev.Test == "" {
				finished[ev.Package] = true
				if ev.Action == "fail" {
					p.failed = true
					p.failedBuild = ev.FailedBuild
				}
				continue
			}
			tests++
			if ev.Action == "fail" {
				p.tests = append(p.tests, ev.Test)
			}
		}
	}

	var failures []Unit
	for _, p := range order {
		leaves := 0
		for _, t := range p.tests {
			if hasFailingSubtest(p.tests, t) {
				continue
			}
			leaves++
			failures = append(failures, Unit{Location: p.name, Name: t, Text: strings.Join(p.testOutput[t], "\n")})
		}
		switch {
		case p.failedBuild != "":
			failures = append(failures, Unit{Location: p.name, Name: p.name, Text: strings.Join(buildText[p.failedBuild], "\n"), Kind: KindBuildFailed})
		case p.failed && leaves == 0:
			// The package failed without a failing test, such as a panic in TestMain.
			failures = append(failures, Unit{Location: p.name, Name: p.name, Text: strings.Join(p.output, "\n"), Kind: KindPackageFailed})
		}
	}
	return Outcome{
		Units:    tests,
		Summary:  fmt.Sprintf("%d tests in %d packages", tests, len(finished)),
		Failures: failures,
	}
}

// isGoTestNoise reports the framing lines go test -v prints around a test's
// own output.
func isGoTestNoise(l string) bool {
	l = strings.TrimSpace(l)
	return strings.HasPrefix(l, "=== ") || strings.HasPrefix(l, "--- ")
}

func hasFailingSubtest(failed []string, name string) bool {
	return slices.ContainsFunc(failed, func(t string) bool { return strings.HasPrefix(t, name+"/") })
}

// RerunArgs drops any -run of the caller and adds an anchored one covering the
// top-level tests of all failures.
func (GoTest) RerunArgs(args []string, failures []Unit) []string {
	var names []string
	for _, u := range failures {
		if u.Kind != KindTest {
			continue
		}
		top, _, _ := strings.Cut(u.Name, "/")
		if !slices.Contains(names, top) {
			names = append(names, top)
		}
	}
	if len(names) == 0 {
		return nil
	}
	for i, n := range names {
		names[i] = regexp.QuoteMeta(n)
	}
	pattern := "^(" + strings.Join(names, "|") + ")$"

	rest, tail := splitArgs(args)
	re, _ := stripFlag(rest, true, "run")
	re = append(re, "-run", pattern)
	return append(re, tail...)
}
