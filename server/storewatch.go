package server

import (
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// StoreWatch reports changes under a ticket store as fsnotify-style events.
// On macOS it uses FSEvents, which watches the whole tree without an fd per
// file (kqueue needs one, which exhausts the fd limit on a big store).
type StoreWatch struct {
	Events <-chan fsnotify.Event
	stop   func()
	rewalk func()
}

// Close ends the watch. The Events channel is not closed.
func (w *StoreWatch) Close() { w.stop() }

// Rewatch picks up directories created since the last call. FSEvents is
// recursive, so only the fsnotify fallback has anything to do.
func (w *StoreWatch) Rewatch() { w.rewalk() }

// inGitDir reports whether path is inside a .git directory, whose churn never
// changes a ticket.
func inGitDir(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}
