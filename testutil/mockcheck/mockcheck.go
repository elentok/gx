// Package mockcheck validates a mock check's result lines. A `pass` must name
// the PNGs the agent compared, and each must exist and decode as a non-empty
// image, so a state whose screenshot reads came back empty cannot be passed.
package mockcheck

import (
	"fmt"
	"image/png"
	"os"
	"strings"
)

// Validate checks result lines of the form
//
//	<state>: pass <png> <png>...
//	<state>: fail <differences>
//	<state>: unchecked <reason>
//
// and returns one problem per bad line. Blank lines are skipped.
func Validate(text string) []string {
	var problems []string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if p := validateLine(line); p != "" {
			problems = append(problems, fmt.Sprintf("line %d: %s", i+1, p))
		}
	}
	return problems
}

func validateLine(line string) string {
	state, rest, ok := strings.Cut(line, ":")
	if !ok || strings.TrimSpace(state) == "" {
		return "want `<state>: pass|fail|unchecked ...`"
	}
	verdict, detail, _ := strings.Cut(strings.TrimSpace(rest), " ")
	switch verdict {
	case "fail", "unchecked":
		return ""
	case "pass":
		paths := strings.Fields(detail)
		if len(paths) < 2 {
			return "pass needs the mock and built PNG paths"
		}
		for _, p := range paths {
			if err := checkPNG(p); err != nil {
				return fmt.Sprintf("pass names an unviewable PNG: %v", err)
			}
		}
		return ""
	}
	return fmt.Sprintf("unknown verdict %q", verdict)
}

func checkPNG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		return fmt.Errorf("%s: empty image", path)
	}
	return nil
}
