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

// minPNGBytes is the smallest file a real part crop can be; a blank or
// failed capture encodes to less.
const minPNGBytes = 200

// Validate checks result lines of the form
//
//	<state>: pass <mock.png> <built.png> | <mock selector> | <built selector>
//	<state>: fail <differences>
//	<state>: unchecked <reason>
//
// and returns one problem per bad line, plus one line per image a `pass`
// validated, so silence is never a pass. Blank lines are skipped.
func Validate(text string) (problems, viewed []string) {
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v, p := validateLine(line)
		viewed = append(viewed, v...)
		if p != "" {
			problems = append(problems, fmt.Sprintf("line %d: %s", i+1, p))
		}
	}
	return problems, viewed
}

func validateLine(line string) (viewed []string, problem string) {
	state, rest, ok := strings.Cut(line, ":")
	state = strings.TrimSpace(state)
	if !ok || state == "" {
		return nil, "want `<state>: pass|fail|unchecked ...`"
	}
	verdict, detail, _ := strings.Cut(strings.TrimSpace(rest), " ")
	switch verdict {
	case "fail", "unchecked":
		return nil, ""
	case "pass":
		parts := strings.Split(detail, "|")
		paths := strings.Fields(parts[0])
		if len(paths) != 2 {
			return nil, "pass needs the mock and built PNG paths"
		}
		if len(parts) != 3 {
			return nil, "pass needs `| <mock selector> | <built selector>` after the PNG paths"
		}
		mockSel, builtSel := strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		if mockSel == "" || mockSel != builtSel {
			return nil, fmt.Sprintf("pass compares different selectors: %q vs %q", mockSel, builtSel)
		}
		for _, p := range paths {
			info, err := checkPNG(p)
			if err != nil {
				return viewed, fmt.Sprintf("pass names an unviewable PNG: %v", err)
			}
			viewed = append(viewed, fmt.Sprintf("%s: %s ok (%s, %d bytes)", state, p, mockSel, info))
		}
		return viewed, ""
	}
	return nil, fmt.Sprintf("unknown verdict %q", verdict)
}

func checkPNG(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if st.Size() < minPNGBytes {
		return 0, fmt.Errorf("%s: only %d bytes, a blank capture", path, st.Size())
	}
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		return 0, fmt.Errorf("%s: empty image", path)
	}
	return st.Size(), nil
}
