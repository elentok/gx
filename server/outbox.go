package server

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

// OutboxProject is the project evolve findings are filed in. A maintainer
// registers the real one; a contributor gets a ticket-only stand-in.
const OutboxProject = "gx"

// OutboxWorkspace is the outbox project's empty working area, beside the
// ticket store like the scratch workspace.
func OutboxWorkspace(ticketStore string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(ticketStore)), "gx-outbox")
}

// EnsureOutboxProject creates the ticket-only `gx` project in contributor mode
// when the store has none. It never touches an existing `gx` project and never
// acts in other modes. The project is not queued, so it never runs.
func EnsureOutboxProject(ticketStore, mode string) error {
	if mode != config.EvolveContributor {
		return nil
	}
	_, err := tickets.ProjectDirByName(ticketStore, OutboxProject)
	if err == nil {
		return nil
	}
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownProject {
		return err
	}
	workspace := OutboxWorkspace(ticketStore)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	name, vcs := OutboxProject, config.VCSNone
	return tickets.WriteProjectFile(filepath.Join(ticketStore, OutboxProject),
		config.ProjectFile{Name: &name, Repo: &workspace, VCS: &vcs})
}
