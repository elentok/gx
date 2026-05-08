package logger

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

const (
	LOG_FILE = "gx.log"
)

func Debug(format string, args ...any) {
	// if len(os.Getenv("DEBUG")) > 0 {
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
