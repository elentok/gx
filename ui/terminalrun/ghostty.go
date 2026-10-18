package terminalrun

import (
	"fmt"
	"strconv"
	"strings"
)

// Every script reads exactly [cwd, initialInput] from argv, so gx never
// escapes text into AppleScript source. `command` is deliberately unused: it
// forces wait-after-command and skips the user's shell rc.
const (
	ghosttyScriptHead = `on run argv
	set cwd to item 1 of argv
	set typed to item 2 of argv
	tell application "Ghostty"
		set cfg to new surface configuration
		set initial working directory of cfg to cwd
		if typed is not "" then set initial input of cfg to typed
`
	ghosttyScriptTail = `
	end tell
end run
`
	ghosttyHSplitScript = ghosttyScriptHead +
		`		split (focused terminal of selected tab of front window) direction down with configuration cfg` + ghosttyScriptTail
	ghosttyVSplitScript = ghosttyScriptHead +
		`		split (focused terminal of selected tab of front window) direction right with configuration cfg` + ghosttyScriptTail
	ghosttyTabScript = ghosttyScriptHead +
		`		new tab in front window with configuration cfg` + ghosttyScriptTail
)

// FallbackError means a Ghostty split failed before anything could have
// opened, so the caller may safely run the same program in place. Any other
// Ghostty error may follow a split that already opened and must never be
// retried.
type FallbackError struct {
	Hint string
}

func (e *FallbackError) Error() string { return e.Hint }

// runOsaScript runs script with argv passed through `on run argv`.
func runOsaScript(script string, argv ...string) ([]byte, error) {
	return runCommand("osascript", append([]string{"-e", script}, argv...)...)
}

// CheckGhosttySplits reports whether a Ghostty with the given
// TERM_PROGRAM_VERSION supports AppleScript splits (>= 1.3.0). It returns a
// *FallbackError when it does not. There is no upper bound.
func CheckGhosttySplits(version string) error {
	if v, ok := parseVersion(version); ok && !versionBefore(v, [3]int{1, 3, 0}) {
		return nil
	}
	return &FallbackError{Hint: fmt.Sprintf("Ghostty %s has no AppleScript splits (needs 1.3.0 or later)", version)}
}

// parseVersion reads a leading MAJOR.MINOR.PATCH, ignoring a -… or +… suffix.
func parseVersion(s string) ([3]int, bool) {
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	var v [3]int
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func versionBefore(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// ghosttyInput builds the initial input typed into the new surface's shell.
// A shell launch types nothing: the split already runs the user's shell.
func ghosttyInput(program string, args []string, shell bool) string {
	if shell {
		return ""
	}
	tokens := make([]string, 0, len(args)+1)
	tokens = append(tokens, shellQuote(program))
	for _, arg := range args {
		tokens = append(tokens, shellQuote(arg))
	}
	return strings.Join(tokens, " ") + "; exit\n"
}

func launchGhostty(worktreeRoot string, splitType SplitType, program string, args []string, shell bool, version string) error {
	if err := CheckGhosttySplits(version); err != nil {
		return err
	}
	var script string
	switch splitType {
	case HSplit:
		script = ghosttyHSplitScript
	case VSplit:
		script = ghosttyVSplitScript
	case Tab:
		script = ghosttyTabScript
	}
	out, err := runOsaScript(script, worktreeRoot, ghosttyInput(program, args, shell))
	if err != nil {
		return classifyGhosttyFailure(string(out), err)
	}
	return nil
}

// classifyGhosttyFailure maps osascript output to a hint. Only the codes that
// can't have opened a split become a *FallbackError.
func classifyGhosttyFailure(out string, err error) error {
	out = strings.TrimSpace(out)
	if out == "" {
		out = err.Error()
	}
	switch {
	case strings.Contains(out, "-1743") && strings.Contains(out, "macos-applescript"):
		return &FallbackError{Hint: "Ghostty: set macos-applescript = true in your Ghostty config"}
	case strings.Contains(out, "-1743"):
		return &FallbackError{Hint: "allow gx/Ghostty under System Settings › Privacy & Security › Automation"}
	}
	failed := fmt.Sprintf("Ghostty split failed: %s", out)
	for _, code := range []string{"-2740", "-2741", "-1708", "-1728"} {
		if strings.Contains(out, code) {
			return &FallbackError{Hint: failed}
		}
	}
	return fmt.Errorf("%s", failed)
}
