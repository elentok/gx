// Package holderdiag describes the agent directories of a session that did not
// start, for the fixture's error messages.
package holderdiag

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
)

const holderErrTailLines = 20

// StartFailureState names what the fixture saw of the agent directories (the
// ones whose path contains id, or every one when id is empty) when a session
// did not start: the holder.err tail, the files, the holder pid and whether
// it lives, the saved exit code, and the machine's process count and load.
func StartFailureState(root, id string) string {
	var b strings.Builder
	dirs := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != nativerunner.MetaFile || !strings.Contains(path, id) {
			return nil
		}
		dirs++
		b.WriteString(agentDirState(filepath.Dir(path)))
		return nil
	})
	sample := brief.SystemSample()
	fmt.Fprintf(&b, "\nagent dirs found: %d; user processes: %d; load: %.2f", dirs, sample.Procs, sample.Load)
	return b.String()
}

func agentDirState(dir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nagent dir %s:", dir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "\n  %s %d bytes %s", e.Name(), info.Size(), info.ModTime().Format("15:04:05.000"))
		}
	}
	var meta nativerunner.Meta
	if raw, err := os.ReadFile(filepath.Join(dir, nativerunner.MetaFile)); err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	if meta.HolderPID > 0 {
		fmt.Fprintf(&b, "\n  holder pid %d alive: %t", meta.HolderPID, syscall.Kill(meta.HolderPID, 0) == nil)
	} else {
		b.WriteString("\n  holder pid: none recorded in meta.json")
	}
	if code, err := os.ReadFile(filepath.Join(dir, holder.ExitCodeFile)); err == nil {
		fmt.Fprintf(&b, "\n  saved exit code: %s", strings.TrimSpace(string(code)))
	} else {
		b.WriteString("\n  saved exit code: none")
	}
	if tail := holderErrTail(dir, "", holderErrTailLines); tail != "" {
		fmt.Fprintf(&b, "\n  holder.err (last lines):\n%s", tail)
	} else {
		b.WriteString("\n  holder.err: empty or missing")
	}
	return b.String()
}

// holderErrTail returns the last n lines of the holder.err in the agent dir
// whose path contains the session id, or "" when there is none or it is empty.
func holderErrTail(root, id string, n int) string {
	var tail string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != holder.HolderErrFile || !strings.Contains(path, id) {
			return nil
		}
		lines := strings.Split(holder.HolderErrText(filepath.Dir(path)), "\n")
		tail = strings.Join(lines[max(0, len(lines)-n):], "\n")
		return filepath.SkipAll
	})
	return strings.TrimSpace(tail)
}
