package check

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
)

// Mode says which files a run covers.
type Mode int

const (
	// ModeChanged is the default: files changed against HEAD plus untracked files.
	ModeChanged Mode = iota
	ModeStaged
	ModeSince
	ModeFull
)

// gitLines runs git in dir and splits its NUL-separated output (-z keeps odd
// file names intact).
func gitLines(dir string, args ...string) ([]string, error) {
	out, stderr, err := git.RunRaw(dir, git.NonInteractiveEnv(), args...)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], firstLine(msg))
	}
	var lines []string
	for f := range bytes.SplitSeq(out, []byte{0}) {
		if len(f) > 0 {
			lines = append(lines, string(f))
		}
	}
	return lines, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func repoRoot(dir string) (string, error) {
	out, _, err := git.Run(dir, git.NonInteractiveEnv(), "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git repo: %s", dir)
	}
	return strings.TrimSpace(out), nil
}

// selectFiles lists the root-relative files for mode.
func selectFiles(root string, mode Mode, since string) ([]string, error) {
	const filter = "--diff-filter=ACMR"
	switch mode {
	case ModeStaged:
		return gitLines(root, "diff", "--cached", "--name-only", "-z", filter)
	case ModeSince:
		return gitLines(root, "diff", "--name-only", "-z", filter, since+"...HEAD")
	case ModeFull:
		return gitLines(root, "ls-files", "-z")
	}
	changed, err := gitLines(root, "diff", "--name-only", "-z", filter, "HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := gitLines(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var files []string
	for _, f := range append(changed, untracked...) {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	return files, nil
}

const filesPlaceholder = "{files}"

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// expandBatches replaces {files} with the quoted files, splitting them into
// several commands when one would pass limit bytes (like xargs). A command
// without {files}, or with no files, yields one command.
func expandBatches(cmd string, files []string, limit int) []string {
	n := strings.Count(cmd, filesPlaceholder)
	if n == 0 {
		return []string{cmd}
	}
	base := len(cmd) - n*len(filesPlaceholder)
	var out, cur []string
	size := base
	flush := func() {
		out = append(out, strings.ReplaceAll(cmd, filesPlaceholder, strings.Join(cur, " ")))
		cur, size = nil, base
	}
	for _, f := range files {
		q := shellQuote(f)
		add := n * (len(q) + 1)
		if len(cur) > 0 && size+add > limit {
			flush()
		}
		cur = append(cur, q)
		size += add
	}
	if len(cur) > 0 || len(out) == 0 {
		flush()
	}
	return out
}
