package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	tea "charm.land/bubbletea/v2"
)

var (
	dirMu sync.RWMutex
	dir   string
)

// SetDir sets the directory gx.log is written to (the state dir, never the
// ticket store where it would get committed). Until it is called, Debug drops
// its lines.
func SetDir(d string) {
	dirMu.Lock()
	defer dirMu.Unlock()
	dir = d
}

func Debug(format string, args ...any) {
	dirMu.RLock()
	d := dir
	dirMu.RUnlock()
	if d == "" {
		return
	}
	logFile := filepath.Join(d, "gx.log")
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
