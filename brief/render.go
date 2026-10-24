package brief

import (
	"fmt"
	"io"
	"strings"
)

// Limits shared by every tool's failure report.
const (
	maxUnitLines = 10  // lines of text shown per failure unit
	maxLineChars = 300 // longer lines are cut
	maxFullUnits = 5   // units shown with text; the rest are listed by name
)

// Unit is one failure: a failing test, or a package that fails to build.
// Field order is the display order: location, name, text.
type Unit struct {
	Location string // package (or file, for tools that report files)
	Name     string // test name; the location again for a build failure
	Text     string
	Build    bool // a build failure rather than a failing test
}

// renderUnits prints the units under their location. Units of one location
// must be adjacent.
func renderUnits(w io.Writer, units []Unit) {
	prev := ""
	for i, u := range units {
		if i == 0 || u.Location != prev {
			fmt.Fprintln(w, u.Location)
			prev = u.Location
		}
		if u.Build {
			fmt.Fprintln(w, "  build failed")
		} else {
			fmt.Fprintf(w, "  %s\n", u.Name)
		}
		if i >= maxFullUnits {
			continue
		}
		for _, l := range limitText(u.Text) {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
	if len(units) > maxFullUnits {
		fmt.Fprintf(w, "%d more failures are listed by name only; see the log for their text\n", len(units)-maxFullUnits)
	}
}

// limitText returns at most maxUnitLines lines, each cut at maxLineChars, with
// a notice line wherever text was cut.
func limitText(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	var out []string
	for i, l := range lines {
		if i == maxUnitLines {
			out = append(out, fmt.Sprintf("[%d more lines cut, search the log]", len(lines)-maxUnitLines))
			break
		}
		if r := []rune(l); len(r) > maxLineChars {
			l = string(r[:maxLineChars]) + fmt.Sprintf("[%d more chars cut, search the log]", len(r)-maxLineChars)
		}
		out = append(out, l)
	}
	return out
}

// rawHead prints the first maxUnitLines lines of output nobody could parse.
func rawHead(w io.Writer, out []byte) {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for _, l := range limitText(strings.Join(lines, "\n")) {
		fmt.Fprintln(w, l)
	}
}

// rerunLine builds the copy-ready command that re-runs only the failures.
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
