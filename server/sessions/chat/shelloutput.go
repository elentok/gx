package chat

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

const (
	// shellTailBytes bounds what one read takes from the end of an output file.
	shellTailBytes = 4096
	// shellLastLineRunes caps the last-line detail.
	shellLastLineRunes = 200
)

var shellExitTrailer = regexp.MustCompile(`^\[exited with code (-?\d+)\]$`)

// ShellOutput is what a Claude shell's output file shows.
type ShellOutput struct {
	Size int64
	// LastLine is the last non-empty line that is not the trailer.
	LastLine string
	// Outcome and ExitCode come from the trailer line; Outcome is empty while
	// the file has none.
	Outcome  string
	ExitCode *int
	MTime    time.Time
}

// readShellOutput reads the tail of a shell's output file. A missing file is
// no details, not an error.
func readShellOutput(path string) (ShellOutput, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ShellOutput{}, nil
	}
	if err != nil {
		return ShellOutput{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ShellOutput{}, err
	}
	out := ShellOutput{Size: info.Size(), MTime: info.ModTime()}
	start := max(info.Size()-shellTailBytes, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return ShellOutput{}, err
	}
	var lines []string
	for l := range strings.SplitSeq(string(buf), "\n") {
		if l = strings.TrimRight(l, "\r"); strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if n := len(lines); n > 0 {
		last := strings.TrimSpace(lines[n-1])
		if m := shellExitTrailer.FindStringSubmatch(last); m != nil {
			code, _ := strconv.Atoi(m[1])
			out.ExitCode = &code
			out.Outcome = transcript.TaskOutcomeSuccess
			if code != 0 {
				out.Outcome = transcript.TaskOutcomeFailure
			}
			lines = lines[:n-1]
		} else if last == "[killed]" {
			out.Outcome = transcript.TaskOutcomeKilled
			lines = lines[:n-1]
		}
	}
	if n := len(lines); n > 0 {
		out.LastLine = capRunes(lines[n-1], shellLastLineRunes)
	}
	return out, nil
}

func capRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// safeShellOutputPath accepts only <dir>/tasks/<name>.output, because the path
// comes from transcript text.
func safeShellOutputPath(p string) bool {
	return filepath.IsAbs(p) && filepath.Base(filepath.Dir(p)) == "tasks" &&
		strings.HasSuffix(filepath.Base(p), ".output") && filepath.Clean(p) == p
}

// withShellOutputs returns records with each shell's output details filled.
// It copies what it changes: the records belong to the cached snapshot.
func (c *Chat) withShellOutputs(records []transcript.TaskRecord) []transcript.TaskRecord {
	compute := readShellOutput
	if c.reads.shellOutput != nil {
		compute = c.reads.shellOutput
	}
	return enrichTasks(records, enrichSpec[ShellOutput]{
		cache: &c.shellOutputs, compute: compute,
		path: func(r transcript.TaskRecord) (string, bool) {
			return r.OutputPath, r.Kind == transcript.TaskKindShell && safeShellOutputPath(r.OutputPath)
		},
		usable: func(o ShellOutput) bool { return !o.MTime.IsZero() },
		merge: func(r *transcript.TaskRecord, o ShellOutput) {
			r.OutputBytes, r.LastLine = o.Size, o.LastLine
			if r.ExitCode == nil {
				r.ExitCode = o.ExitCode
			}
			if r.End == nil && o.Outcome != "" {
				r.Trailer = &transcript.TaskEnd{At: o.MTime, Outcome: o.Outcome}
			}
		},
	})
}
