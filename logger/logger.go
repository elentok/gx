package logger

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
)

// logFile resolves gx.log's path via Repo.ScratchRoot(), so debug logging
// lands in the canonical `.scratch` regardless of which linked worktree of a
// bare-repo checkout the command was run from. Falls back to the old
// cwd-relative path if the cwd isn't inside a git repo.
func logFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".scratch/gx.log"
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return ".scratch/gx.log"
	}
	return filepath.Join(repo.ScratchRoot(), "gx.log")
}

func Debug(format string, args ...any) {
	// if len(os.Getenv("DEBUG")) > 0 {
	logFile := logFile()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		fmt.Println("fatal (can't create log dir):", err)
		os.Exit(1)
	}

	f, err := tea.LogToFile(logFile, "debug")
	if err != nil {
		fmt.Println("fatal (can't open log file):", err)
		os.Exit(1)
	}

	_, err = fmt.Fprintf(f, format, args...)
	if err != nil {
		fmt.Println("fatal (can't write to log file):", err)
		os.Exit(1)
	}

	defer f.Close()
}
