package brief

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// GoBuild wraps `go build`.
type GoBuild struct{}

func (GoBuild) Name() string { return "go-build" }

func (GoBuild) Plan(env []string, _ string, args []string) (Invocation, error) {
	return planGo(env, "build", args)
}

func (GoBuild) Parse(out []byte, exit int) Outcome {
	return parseGoDiagnostics(out, exit, "no build errors")
}

func (GoBuild) RerunArgs([]string, []Unit) []string { return nil }

// GoVet wraps `go vet`.
type GoVet struct{}

func (GoVet) Name() string { return "go-vet" }

func (GoVet) Plan(env []string, _ string, args []string) (Invocation, error) {
	return planGo(env, "vet", args)
}

func (GoVet) Parse(out []byte, exit int) Outcome {
	return parseGoDiagnostics(out, exit, "no vet findings")
}

func (GoVet) RerunArgs([]string, []Unit) []string { return nil }

func planGo(env []string, sub string, args []string) (Invocation, error) {
	exe, err := LookPath(env, "go")
	if err != nil {
		return Invocation{}, errors.New("go not found on PATH")
	}
	return Invocation{Bin: exe, Argv: append([]string{sub}, args...)}, nil
}

var goDiagnostic = regexp.MustCompile(`^([^\s:]+\.go):(\d+:\d+: .*)$`)

// parseGoDiagnostics treats a clean exit as a pass; go build and go vet print
// nothing then. Otherwise it groups `file.go:line:col: message` lines by file,
// in order of first appearance. Indented lines continue the diagnostic above
// them. A failure with no such line (a missing module) yields no units, so the
// report falls back to the raw head of the output.
func parseGoDiagnostics(out []byte, exit int, passSummary string) Outcome {
	if exit == 0 {
		return Outcome{Units: 1, Summary: passSummary}
	}
	var files []*fileErrors
	byFile := map[string]*fileErrors{}
	var cur *fileErrors
	for l := range strings.SplitSeq(string(out), "\n") {
		l = strings.TrimRight(l, "\r")
		if m := goDiagnostic.FindStringSubmatch(l); m != nil {
			f := byFile[m[1]]
			if f == nil {
				f = &fileErrors{file: m[1]}
				byFile[m[1]] = f
				files = append(files, f)
			}
			f.errors++
			f.lines = append(f.lines, m[2])
			cur = f
		} else if cur != nil && (strings.HasPrefix(l, "\t") || strings.HasPrefix(l, " ")) {
			cur.lines = append(cur.lines, l)
		} else {
			cur = nil
		}
	}
	var failures []Unit
	for _, f := range files {
		name := fmt.Sprintf("%d errors", f.errors)
		if f.errors == 1 {
			name = "1 error"
		}
		failures = append(failures, Unit{Location: f.file, Name: name, Text: strings.Join(f.lines, "\n")})
	}
	return Outcome{Failures: failures}
}
