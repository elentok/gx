package logger

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

const (
	LOG_FILE = ".scratch/gx.log"
)

func Debug(format string, args ...any) {
	// if len(os.Getenv("DEBUG")) > 0 {
	if err := os.MkdirAll(filepath.Dir(LOG_FILE), 0755); err != nil {
		fmt.Println("fatal (can't create log dir):", err)
		os.Exit(1)
	}

	f, err := tea.LogToFile(LOG_FILE, "debug")
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
