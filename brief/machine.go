package brief

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/elentok/gx/syshealth"
)

// Sample is how busy the machine is at one moment.
type Sample struct {
	// Load is the 1-minute load average; 0 when unknown.
	Load float64
	// Procs is the number of processes owned by the current user; 0 when unknown.
	Procs int
}

// SampleFunc reads a Sample. Options.Sample defaults to SystemSample; tests
// inject a fake.
type SampleFunc func() Sample

// SystemSample reads the real machine.
func SystemSample() Sample {
	return Sample{Load: SystemLoad(), Procs: userProcs()}
}

// SystemLoad is the 1-minute load average of the machine; 0 when unknown.
func SystemLoad() float64 {
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		return firstFloat(string(data))
	}
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0
	}
	return firstFloat(strings.Trim(string(out), "{} \n"))
}

func firstFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.Fields(s + " ")[0], 64)
	return f
}

// userProcs counts the current user's processes, the number scripts/proc-cap
// caps against.
func userProcs() int {
	table, err := syshealth.DefaultProcess().Read()
	if err != nil {
		return 0
	}
	uid := os.Getuid()
	n := 0
	for _, p := range table.Processes {
		if !p.Unreadable && p.UID == uid {
			n++
		}
	}
	return n
}

// EnvCheckOrigin marks who started a run: `release` for a release run, anything
// else (or nothing) is an agent.
const EnvCheckOrigin = "GX_CHECK_ORIGIN"

func originOf(env []string) string {
	if v, _ := envValue(env, EnvCheckOrigin); v == "release" {
		return "release"
	}
	return "agent"
}

// maxSignatureChars bounds an index signature.
const maxSignatureChars = 200

// signatureOf names why a failing run failed; empty for a pass.
func signatureOf(exit int, outcome Outcome, out []byte) string {
	if exit == 0 {
		return ""
	}
	var text string
	if len(outcome.Failures) > 0 {
		text = outcome.Failures[0].Text
	} else if outcome.Unhandled != "" {
		_, detail, _ := strings.Cut(outcome.Unhandled, "\n")
		text = detail
	} else {
		var b bytes.Buffer
		rawHead(&b, out, "")
		text = b.String()
	}
	return truncateRunes(firstLine(text), maxSignatureChars)
}

func firstLine(s string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// attemptTailBytes is how much of the index end the attempt lookup reads. The
// real index is hundreds of MB, so it is never read whole.
const attemptTailBytes = 16 << 20

// nextAttempt is one more than the newest earlier index line with the same
// tool, tree and args; 1 when there is none within the tail budget.
func nextAttempt(indexPath, tool, treeID string, args []string) int {
	return nextAttemptWithin(indexPath, tool, treeID, args, attemptTailBytes)
}

func nextAttemptWithin(indexPath, tool, treeID string, args []string, budget int64) int {
	if treeID == "" {
		return 1
	}
	f, err := os.Open(indexPath)
	if err != nil {
		return 1
	}
	defer f.Close()
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 1
	}
	start := max(size-budget, 0)
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return 1
	}
	lines := bytes.Split(buf, []byte("\n"))
	if start > 0 {
		lines = lines[1:] // the first line is probably cut
	}
	for _, line := range slices.Backward(lines) {
		var e indexEntry
		if len(line) == 0 || json.Unmarshal(line, &e) != nil {
			continue
		}
		if e.Tool == tool && e.TreeID == treeID && slices.Equal(e.Args, args) {
			return max(e.Attempt, 1) + 1
		}
	}
	return 1
}
