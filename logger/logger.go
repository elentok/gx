package logger

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
)

// logFile puts gx.log in the state dir, never inside the ticket store where
// it would get committed. Falls back to a cwd-relative path if the state dir
// can't be resolved.
func logFile() string {
	dir, err := config.StateDir()
	if err != nil {
		return "gx.log"
	}
	return filepath.Join(dir, "gx.log")
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
