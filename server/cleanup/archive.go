package cleanup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// ArchiveEpic moves a done epic that has landed on trunk into the project's
// archive directory. Done and merged are re-checked here, and a queued epic is
// refused, so a stale view cannot archive live work.
func (c *Cleanup) ArchiveEpic(project, epic string) (wire.CleanupWriteResult, error) {
	dir, err := tickets.ProjectDirByName(c.d.TicketStore, project)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	repo := ticketgit.ProjectRepoRoot(c.d.TicketStore, tickets.ProjectName(dir))
	if repo == "" {
		return wire.CleanupWriteResult{}, fmt.Errorf("project %q has no repo", project)
	}
	if c.d.EpicQueued != nil && c.d.EpicQueued(project, epic) {
		return refuse(wire.ReasonCleanupQueued, "the epic is queued"), nil
	}
	res, err := cleanupscan.ScanIn(c.d.TicketStore, repo)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	var row *cleanupscan.EpicScan
	for i := range res.Epics {
		if res.Epics[i].Name == epic {
			row = &res.Epics[i]
		}
	}
	switch {
	case row == nil:
		return refuse(wire.ReasonCleanupUnknown, "no such epic"), nil
	case !row.AllDone:
		return refuse(wire.ReasonCleanupNotDone, "the epic has tickets that are not done"), nil
	case !row.MergedToTrunk:
		return refuse(wire.ReasonCleanupNotMerged, "the epic has not landed on trunk"), nil
	}
	archive := tickets.ArchiveDir(dir)
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return wire.CleanupWriteResult{}, err
	}
	if err := os.Rename(filepath.Join(dir, epic), filepath.Join(archive, epic)); err != nil {
		return refuse(wire.ReasonCleanupGitRefused, err.Error()), nil
	}
	return wire.CleanupWriteResult{}, nil
}
