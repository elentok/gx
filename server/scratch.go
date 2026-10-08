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

// scratchSubdir is where one scratch ticket runs.
func scratchSubdir(ticketStore, epic string) string {
	return filepath.Join(ScratchWorkspace(ticketStore), epic)
}

// pruneScratchWorkspace deletes the subdirs of tickets that have left the
// scratch project, which is what archiving one does.
func pruneScratchWorkspace(ticketStore string) error {
	workspace := ScratchWorkspace(ticketStore)
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(ticketStore, ScratchProject, e.Name())); err == nil {
			continue
		}
		if err := os.RemoveAll(filepath.Join(workspace, e.Name())); err != nil {
			return err
		}
	}
	return nil
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
