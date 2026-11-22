package brief

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Limits shared by every tool's failure report.
const (
	maxUnitLines = 10  // lines of text shown per failure unit
	maxLineChars = 300 // longer lines are cut
	maxFullUnits = 5   // units shown with text; the rest are listed by name
)

// UnitKind says what failed. The zero value is a failing test.
type UnitKind int

const (
	KindTest          UnitKind = iota
	KindBuildFailed            // a package that fails to build
	KindPackageFailed          // a package that fails without a failing test
	KindFileFailed             // a file that fails without a failing test
)

// label is the heading of a unit that is not a test; tests use their name.
func (k UnitKind) label() string {
	switch k {
	case KindBuildFailed:
		return "build failed"
	case KindPackageFailed:
		return "package failed"
	case KindFileFailed:
		return "file failed"
	}
	return ""
}

// Unit is one failure: a failing test, or a package or file that fails as a whole.
// Field order is the display order: location, name, text.
type Unit struct {
	Location string // package (or file, for tools that report files)
	Name     string // test name; the location again for a non-test kind
	Text     string
	Kind     UnitKind
}

// String names the failure on one line: `package.TestName` for a test, the
// location and what failed otherwise.
func (u Unit) String() string {
	if u.Kind != KindTest {
		return u.Location + " " + u.Kind.label()
	}
	return u.Location + "." + u.Name
}

// renderUnits prints the units under their location. Units of one location
// must be adjacent.
func renderUnits(w io.Writer, units []Unit) { renderUnitsMax(w, units, maxFullUnits) }

// renderUnitsMax is renderUnits with the number of units shown with text
// chosen by the caller.
func renderUnitsMax(w io.Writer, units []Unit, maxFull int) {
	renderUnitsWith(w, units, maxFull, "", nil)
}

// renderUnitsWith is renderUnitsMax that also calls after(unit) right below the
// text of each unit shown with text. Units listed by name only get no call.
// Every cut notice names log, the file that holds the full text.
func renderUnitsWith(w io.Writer, units []Unit, maxFull int, log string, after func(Unit)) {
	renderUnitsText(w, units, maxFull, after, func(s string) []string { return limitText(s, log) })
}

// cutNotice is the bracketed note where text was cut: it says how much, and
// where the full text is.
func cutNotice(n int, what, log string) string {
	if log == "" {
		return fmt.Sprintf("[%d more %s cut, search the log]", n, what)
	}
	return fmt.Sprintf("[%d more %s cut, search the log: %s]", n, what, log)
}

// sharedCause returns the text every unit failed with, when there are more
// units than fit with text and they all fail alike.
func sharedCause(units []Unit, maxFull int) (string, bool) {
	if len(units) <= maxFull || units[0].Text == "" {
		return "", false
	}
	for _, u := range units {
		if u.Text != units[0].Text || u.Kind != KindTest {
			return "", false
		}
	}
	return units[0].Text, true
}

// renderUnitsText is renderUnitsWith with the text lines of a unit chosen by
// the caller; gx brief failures passes an untrimmed one.
func renderUnitsText(w io.Writer, units []Unit, maxFull int, after func(Unit), text func(string) []string) {
	if cause, ok := sharedCause(units, maxFull); ok {
		fmt.Fprintf(w, "%d tests failed with the same cause:\n", len(units))
		for _, l := range text(cause) {
			fmt.Fprintf(w, "  %s\n", l)
		}
		return
	}
	units, counts := collapseUnits(units)
	prev := ""
	for i, u := range units {
		if i == 0 || u.Location != prev {
			fmt.Fprintln(w, u.Location)
			prev = u.Location
		}
		head := u.Name
		if u.Kind != KindTest {
			head = u.Kind.label()
		}
		if counts[i] > 1 {
			head += fmt.Sprintf(" (x%d)", counts[i])
		}
		if i >= maxFull {
			re := goFileLine
			if u.Kind == KindBuildFailed {
				re = goBuildFileLine
			}
			if loc := re.FindString(u.Text); loc != "" {
				head += " " + loc
			}
			fmt.Fprintf(w, "  %s\n", head)
			continue
		}
		fmt.Fprintf(w, "  %s\n", head)
		for _, l := range text(u.Text) {
			fmt.Fprintf(w, "    %s\n", l)
		}
		if after != nil {
			after(u)
		}
	}
	if len(units) > maxFull {
		fmt.Fprintf(w, "%d more failures are listed by name only; see the log for their text\n", len(units)-maxFull)
	}
}

// maxSlowest is how many of the slowest tests renderSlowest prints.
const maxSlowest = 5

// renderSlowest prints the slowest tests, one `<seconds>s  <name>` line each,
// for tools that report page snapshots (playwright). It prints nothing when
// there are no durations.
func renderSlowest(tool Tool, w io.Writer, durations map[string]int64) {
	// Only tools that read page snapshots (playwright) report durations; the
	// ContextReader check is a stand-in for "is playwright".
	if _, ok := tool.(ContextReader); !ok || len(durations) == 0 {
		return
	}
	names := slices.SortedFunc(maps.Keys(durations), func(a, b string) int {
		return cmp.Or(cmp.Compare(durations[b], durations[a]), cmp.Compare(a, b))
	})
	for _, n := range names[:min(maxSlowest, len(names))] {
		fmt.Fprintf(w, "%.1fs  %s\n", float64(durations[n])/1000, n)
	}
}

// renderSnapshot prints the lines of u's page snapshot that match the failing
// locator. A failure without a snapshot prints nothing.
func renderSnapshot(w io.Writer, o Outcome, u Unit) {
	ref := o.Refs[contextKey(u.Location, u.Name)]
	if ref.Line != "" {
		fmt.Fprintf(w, "  at: %s\n", ref.Line)
	}
	if ref.Context != "" {
		fmt.Fprintf(w, "  error-context: %s\n", ref.Context)
	}
	lines := snapshotExcerpt(u.Text, o.Contexts[contextKey(u.Location, u.Name)])
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "  snapshot for %s:\n", u.Name)
	for _, l := range lines {
		fmt.Fprintf(w, "    %s\n", l)
	}
}

// goFileLine finds the first file:line of a Go test failure in its output.
var goFileLine = regexp.MustCompile(`[\w.-]+_test\.go:\d+`)

// goBuildFileLine finds the first file:line of a compiler error, which can be
// in any .go file, not only a test file.
var goBuildFileLine = regexp.MustCompile(`[\w.-]+\.go:\d+`)

// collapseUnits merges units that repeat another one exactly, as -count=N
// does, and returns how many times each kept unit occurred. A repeat joins the
// first occurrence, so units of one location stay adjacent.
func collapseUnits(units []Unit) ([]Unit, []int) {
	var out []Unit
	var counts []int
	for _, u := range units {
		if i := slices.Index(out, u); i >= 0 {
			counts[i]++
			continue
		}
		out = append(out, u)
		counts = append(counts, 1)
	}
	return out, counts
}

// assertionLine matches the line that says what a test asserted: a Go
// file_test.go:NN: line, or a vitest assertion error or Expected/Received line.
var assertionLine = regexp.MustCompile(`[\w.-]+_test\.go:\d+:|AssertionError|^\s*(Expected|Received)\b`)

// actWarningStart and actWarningEnd bracket the React act() warning block,
// which spans blank lines and a code sample.
const (
	actWarningStart = "not wrapped in act("
	actWarningEnd   = "wrap-tests-with-act"
)

// isJSONLogLine reports whether l is a structured server-log line.
func isJSONLogLine(l string) bool {
	l = strings.TrimSpace(l)
	return strings.HasPrefix(l, `{"`) && (strings.Contains(l, `"level"`) || strings.Contains(l, `"msg"`))
}

// DropNoise removes server-log JSON lines and React act() warning blocks,
// which bury the failing assertion. The full text stays in the log.
func DropNoise(lines []string) []string {
	var out []string
	inAct := false
	for _, l := range lines {
		switch {
		case inAct:
			inAct = !strings.Contains(l, actWarningEnd)
		case strings.Contains(l, actWarningStart):
			inAct = !strings.Contains(l, actWarningEnd)
		case isJSONLogLine(l):
		default:
			out = append(out, l)
		}
	}
	return out
}

// maxContextLines is how many lines follow the assertion line.
const maxContextLines = 3

// focus drops log noise and starts at the assertion line when there is one.
func focus(lines []string) (_ []string, found bool) {
	lines = DropNoise(lines)
	if i := slices.IndexFunc(lines, assertionLine.MatchString); i >= 0 {
		return lines[i:], true
	}
	return lines, false
}

// TrimTail applies the failure-report trimming to the last lines of a log:
// noise goes, and a tail that holds an assertion line shrinks to it plus
// maxContextLines. Any other tail keeps all its lines.
func TrimTail(lines []string) []string {
	lines, found := focus(lines)
	if found {
		lines = lines[:min(len(lines), 1+maxContextLines)]
	}
	return lines
}

// DiagnosticLines keeps the first line of each compiler diagnostic: `info:`
// lines and indented continuations go, and each line is cut at maxLineChars.
func DiagnosticLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" || l[0] == ' ' || l[0] == '\t' || strings.HasPrefix(l, "info:") {
			continue
		}
		if r := []rune(l); len(r) > maxLineChars {
			l = string(r[:maxLineChars-1]) + "…"
		}
		out = append(out, l)
	}
	return out
}

// FirstErrorLine is the line of a failure's text that says what failed: the
// assertion line, else the first non-blank line after log noise. Empty when
// there is none.
func FirstErrorLine(text string) string {
	lines, _ := focus(strings.Split(text, "\n"))
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// limitText drops log noise, then returns the assertion line and at most
// maxContextLines after it. Text without an assertion line gets its first
// maxUnitLines. Each line is cut at maxLineChars, with a notice line wherever
// text was cut.
func limitText(text, log string) []string {
	if text == "" {
		return nil
	}
	lines, found := focus(strings.Split(text, "\n"))
	limit := maxUnitLines
	if found {
		limit = 1 + maxContextLines
	}
	var out []string
	for i, l := range lines {
		if i == limit {
			out = append(out, cutNotice(len(lines)-limit, "lines", log))
			break
		}
		// The first line is the assertion or error, the message the reader
		// needs (a guard's whole finding); only the lines after it are cut.
		if r := []rune(l); i > 0 && len(r) > maxLineChars {
			l = string(r[:maxLineChars]) + cutNotice(len(r)-maxLineChars, "chars", log)
		}
		out = append(out, l)
	}
	return out
}

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	// buildNoise matches vite output that buries the error: asset sizes,
	// chunk-size warnings and their hints.
	buildNoise = regexp.MustCompile(`^(dist/|\(!\)|- |\s*$)`)
	errorLine  = regexp.MustCompile(`(?i)\berror\b|\bfailed\b|\bFAIL\b|✘|×|Expected|Received|Test timeout`)
)

// rawHead prints the part of unparsed output that matters: from the first
// error line on, or the last lines when none looks like an error. ANSI codes
// and build noise are dropped first.
func rawHead(w io.Writer, out []byte, log string) {
	var lines []string
	for l := range strings.SplitSeq(ansiEscape.ReplaceAllString(string(out), ""), "\n") {
		if !buildNoise.MatchString(strings.TrimRight(l, "\r")) {
			lines = append(lines, l)
		}
	}
	if i := slices.IndexFunc(lines, errorLine.MatchString); i >= 0 {
		lines = lines[i:]
	} else if len(lines) > maxUnitLines {
		lines = lines[len(lines)-maxUnitLines:]
	}
	for _, l := range limitText(strings.Join(lines, "\n"), log) {
		fmt.Fprintln(w, l)
	}
}

// maxRerunChars caps the rerun command; a run with many failures would
// otherwise print a name filter of thousands of characters.
const maxRerunChars = 1000

// printRerun prints the copy-ready command that re-runs only the failures.
// Past the cap it re-runs the largest prefix of failures that fits, since the
// long part is one name-filter argument that cannot be sliced.
func printRerun(w io.Writer, tool Tool, cDir string, callerArgs []string, failures []Unit) {
	build := func(n int) string {
		re := tool.RerunArgs(callerArgs, failures[:n])
		if re == nil {
			return ""
		}
		return rerunLine(tool.Name(), cDir, re)
	}
	line := build(len(failures))
	if line == "" || len(line) <= maxRerunChars {
		if line != "" {
			fmt.Fprintln(w, line)
		}
		return
	}
	n := sort.Search(len(failures), func(i int) bool { return len(build(i+1)) > maxRerunChars })
	if n == 0 {
		fmt.Fprintln(w, "too many failing tests to list a re-run command; use `gx brief failures <log> <test>` for one")
		return
	}
	fmt.Fprintf(w, "%s\n(first %d of %d failing tests; use `gx brief failures <log> <test>` for the rest)\n", build(n), n, len(failures))
}

// rerunLine builds the copy-ready command for the given rerun args.
func rerunLine(tool string, cDir string, args []string) string {
	parts := []string{"gx", "brief"}
	if cDir != "" {
		parts = append(parts, "-C", shellQuote(cDir))
	}
	parts = append(parts, tool)
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return "re-run only the failing tests: " + strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,@%+") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
