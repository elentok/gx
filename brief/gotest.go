package brief

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// GoTest wraps `go test`.
type GoTest struct{}

func (GoTest) Name() string { return "go-test" }

// Command always adds -json and -v, once. Flags after -args belong to the test
// binary and pass through untouched.
func (GoTest) Command(args []string) (string, []string) {
	argv := []string{"test", "-json", "-v"}
	rest := args
	if i := slices.Index(args, "-args"); i >= 0 {
		rest = args[:i]
	}
	for i, a := range args {
		if i >= len(rest) {
			argv = append(argv, a)
			continue
		}
		switch a {
		case "-json", "-v", "-json=true", "-v=true":
		default:
			argv = append(argv, a)
		}
	}
	return "go", argv
}

type goTestEvent struct {
	Action  string
	Package string
	Test    string
}

// Parse counts finished tests and packages from the -json stream. Lines that
// are not JSON (build errors on stderr) are ignored.
func (GoTest) Parse(out []byte, _ int) Outcome {
	tests := 0
	pkgs := map[string]bool{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		var ev goTestEvent
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" {
			continue
		}
		if ev.Test != "" {
			tests++
		} else {
			pkgs[ev.Package] = true
		}
	}
	return Outcome{Units: tests, Summary: fmt.Sprintf("%d tests in %d packages", tests, len(pkgs))}
}
