package brief

import (
	"fmt"
	"io"
	"os"
	"slices"
)

// Failures prints every failure of one brief log with its first lines, which
// is what an agent otherwise rebuilds with rg over the JSON. logPath empty
// means the log of the latest failed run in the index. The tool that wrote a
// log comes from the index; a log it does not list is read as go-test output.
// It exits 1 when the log holds failures, 0 when it holds none.
func Failures(dataDir, logPath string, w io.Writer) int {
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
	failures := tool.Parse(out, ExitFail).Failures
	if len(failures) == 0 {
		fmt.Fprintf(w, "no failures in %s\n", logPath)
		return ExitPass
	}
	fmt.Fprintf(w, "%d failed (%s)\n", len(failures), toolName)
	renderUnitsMax(w, failures, len(failures))
	fmt.Fprintln(w, logPath)
	return ExitFail
}
