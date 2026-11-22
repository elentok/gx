package brief

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// Failures prints every failure of one brief log with its first lines, which
// is what an agent otherwise rebuilds with rg over the JSON. logPath empty
// means the log of the latest failed run in the index. The tool that wrote a
// log comes from the index; a log it does not list is read as go-test output.
// The text is never trimmed. A non-empty test prints only that failure, with
// the command that re-runs it.
// It exits 1 when the log holds failures, 0 when it holds none.
// byFile prints one "count file" line per file instead, largest count first.
func Failures(dataDir, logPath, test string, byFile bool, w io.Writer) int {
	entries, err := ReadIndex(dataDir)
	if err != nil {
		return cantRun(w, err.Error())
	}
	toolName := GoTest{}.Name()
	if logPath == "" {
		for _, e := range slices.Backward(entries) {
			if e.Exit != 0 {
				logPath, toolName = e.Log, e.Tool
				break
			}
		}
		if logPath == "" {
			return cantRun(w, "no failed brief run in the index")
		}
	} else {
		for _, e := range entries {
			if e.Log == logPath {
				toolName = e.Tool
			}
		}
	}
	tool, ok := ToolByName(toolName)
	if !ok {
		return cantRun(w, fmt.Sprintf("unknown tool %q for %s", toolName, logPath))
	}
	out, err := os.ReadFile(logPath)
	if err != nil {
		return cantRun(w, err.Error())
	}
	var failures []Unit
	if IsCheckLog(out) {
		failures, toolName = ParseCheckLog(out), "check"
	} else {
		failures = tool.Parse(out, ExitFail).Failures
	}
	if len(failures) == 0 && SaysFailed(out) {
		fmt.Fprintf(w, "failed but no parsed failures in %s\n", logPath)
		for _, l := range tailOf(out, 20) {
			fmt.Fprintln(w, l)
		}
		return ExitFail
	}
	if len(failures) == 0 {
		fmt.Fprintf(w, "no failures in %s\n", logPath)
		return ExitPass
	}
	if test != "" {
		i := slices.IndexFunc(failures, func(u Unit) bool { return u.Name == test || u.String() == test })
		if i < 0 {
			return cantRun(w, fmt.Sprintf("no failure named %q in %s", test, logPath))
		}
		failures = failures[i : i+1]
	}
	if byFile {
		for _, f := range countByFile(toolName, failures) {
			fmt.Fprintf(w, "%d %s\n", f.count, f.file)
		}
		fmt.Fprintln(w, logPath)
		return ExitFail
	}
	fmt.Fprintf(w, "%d failed (%s)\n", len(failures), toolName)
	renderUnitsText(w, failures, len(failures), nil, fullText)
	if test != "" {
		printRerun(w, tool, "", nil, failures)
	}
	fmt.Fprintln(w, logPath)
	return ExitFail
}

type fileCount struct {
	file  string
	count int
}

// countByFile counts the findings of each file, largest first then by name.
// A biome unit already holds all findings of its file, one per text line;
// any other tool counts one per failing test.
func countByFile(toolName string, failures []Unit) []fileCount {
	counts := map[string]int{}
	for _, u := range failures {
		n := 1
		if toolName == (Biome{}).Name() && u.Text != "" {
			n = strings.Count(u.Text, "\n") + 1
		}
		counts[u.Location] += n
	}
	var out []fileCount
	for f, n := range counts {
		out = append(out, fileCount{f, n})
	}
	slices.SortFunc(out, func(a, b fileCount) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return strings.Compare(a.file, b.file)
	})
	return out
}

// FailureRuns prints one line per run for the last n runs of the playwright
// tool: its time, then FAIL and the failing e2e files, or PASS.
func FailureRuns(dataDir string, n int, w io.Writer) int {
	entries, err := ReadIndex(dataDir)
	if err != nil {
		return cantRun(w, err.Error())
	}
	var runs []IndexEntry
	for _, e := range entries {
		if e.Tool == (Playwright{}).Name() {
			runs = append(runs, e)
		}
	}
	runs = runs[max(0, len(runs)-n):]
	if len(runs) == 0 {
		return cantRun(w, "no playwright run in the index")
	}
	for _, e := range runs {
		line := e.Time + " PASS"
		if e.Exit != 0 {
			line = e.Time + " FAIL"
			out, err := os.ReadFile(e.Log)
			if err != nil {
				line += " (log unreadable: " + err.Error() + ")"
			}
			var files []string
			for _, u := range (Playwright{}).Parse(out, e.Exit).Failures {
				if !slices.Contains(files, u.Location) {
					files = append(files, u.Location)
				}
			}
			if len(files) > 0 {
				line += " " + strings.Join(files, " ")
			}
		}
		fmt.Fprintln(w, line)
	}
	return ExitPass
}

// tailOf is the last n lines of out.
func tailOf(out []byte, n int) []string {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	return lines[max(0, len(lines)-n):]
}

// fullText is the untrimmed text of a failure.
func fullText(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
