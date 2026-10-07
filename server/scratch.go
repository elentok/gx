package server

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

// ScratchWorkspace is the scratch project's working area: beside the ticket
// store, not inside it, so the store's git repo never sees it.
func ScratchWorkspace(ticketStore string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(ticketStore)), ScratchProject)
}

// ensureScratchProject registers the built-in scratch project (idempotent).
func ensureScratchProject(ticketStore string) error {
	workspace := ScratchWorkspace(ticketStore)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	name, vcs := ScratchProject, config.VCSNone
	return tickets.WriteProjectFile(filepath.Join(ticketStore, ScratchProject),
		config.ProjectFile{Name: &name, Repo: &workspace, VCS: &vcs})
}
