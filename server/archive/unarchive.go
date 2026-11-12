package archive

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/archivekey"
)

// Unarchive brings each target back out of .archive on its own, in order. It
// never refuses for a name clash; only a target that is not in .archive is a
// refusal.
func (a *Archive) Unarchive(req wire.UnarchiveRequest) (wire.TargetsResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(req.Targets))}
	for _, target := range req.Targets {
		r, err := a.unarchiveOne(target)
		if err != nil {
			return wire.TargetsResult{}, err
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

func (a *Archive) unarchiveOne(target string) (wire.TargetResult, error) {
	unknown := func(msg string) (wire.TargetResult, error) {
		return wire.RefusedTarget(target, wire.ReasonCleanupUnknown, msg), nil
	}
	project, epic, ok := tickets.ParseEpicAddress(strings.TrimSpace(target))
	if !ok {
		return unknown("not an epic address (want project:epic)")
	}
	dir, err := tickets.ProjectDirByName(a.d.TicketStore, project)
	if err != nil {
		if _, isAddr := errors.AsType[*tickets.AddressError](err); isAddr {
			return unknown(err.Error())
		}
		return wire.TargetResult{}, err
	}
	archived, err := tickets.ArchivedEpicNames(dir)
	if err != nil {
		return wire.TargetResult{}, err
	}
	archiveDir := tickets.ArchiveDir(dir)

	// A target given as the -impl address of an archived pair resolves to its Design.
	base, asImpl := epic, false
	if d, ok := strings.CutSuffix(epic, tickets.ImplEpicSuffix); ok && archived[d] && tickets.LoadEpicTicketMD(filepath.Join(archiveDir, d)).IsMap {
		base, asImpl = d, true
	}
	if !archived[base] {
		return unknown("no such archived epic")
	}
	halves := []half{{epic: base}}
	if impl := base + tickets.ImplEpicSuffix; archived[impl] && tickets.LoadEpicTicketMD(filepath.Join(archiveDir, base)).IsMap {
		halves = append(halves, half{epic: impl})
	}
	head := tickets.LoadEpicTicketMD(filepath.Join(archiveDir, base))
	name := freeNameIn(dir, base, len(halves) > 1)

	out := wire.TargetResult{Target: target, Status: wire.TargetDone, NewAddress: newAddress(project, name, base, asImpl)}
	if err := a.restore(dir, project, halves, name, head); err != nil {
		return writeFailed(target, out.NewAddress, err)
	}
	return out, nil
}

// restore moves the halves out of .archive under name (the Design's name; the
// -impl half follows it), clears the stamps, puts the row back in its
// workspace and re-keys the Sessions. head is the Design's (or epic's) archived
// ticket.md.
func (a *Archive) restore(dir, project string, halves []half, name string, head tickets.Epic) error {
	archive := tickets.ArchiveDir(dir)
	finals := make([]string, len(halves))
	for i := range halves {
		finals[i] = name
		if i > 0 {
			finals[i] += tickets.ImplEpicSuffix
		}
	}
	for i, h := range halves {
		if err := os.Rename(filepath.Join(archive, h.epic), filepath.Join(dir, finals[i])); err != nil {
			for j := range halves[:i] {
				_ = os.Rename(filepath.Join(dir, finals[j]), filepath.Join(archive, halves[j].epic))
			}
			return refusal{wire.ReasonArchiveRenameFailed, err}
		}
	}
	var rekeyErrs []error
	for i, h := range halves {
		path := filepath.Join(dir, finals[i])
		if err := tickets.ClearEpicFields(path, "archived_at", "archived_workspace"); err != nil {
			return err
		}
		if i == 0 {
			// branch_name keeps the first branch: the stored one, else the name before this rename.
			branch := cmp.Or(head.BranchName, h.epic)
			var err error
			if branch == finals[0] {
				err = tickets.ClearEpicFields(path, "branch_name")
			} else {
				err = tickets.OverwriteEpicFields(path, map[string]any{"branch_name": branch})
			}
			if err != nil {
				return err
			}
		}
		archiveKey := archivekey.New(project+":"+h.epic, h.epic)
		rekeyErrs = append(rekeyErrs, a.rekey(archivekey.Epic, archiveKey, project+":"+finals[i]))
	}
	placeErr := a.placeInWorkspace(project+":"+finals[0], head.ArchivedWorkspace)
	a.d.Events.Publish(wire.EventWorkspacesChanged, "")
	if err := errors.Join(append(rekeyErrs, placeErr)...); err != nil {
		return failure{err}
	}
	return nil
}

// placeInWorkspace puts key back in its stored workspace, or in Default when
// none is stored or it no longer exists. Any other SetMember error is returned,
// leaving the row unplaced.
func (a *Archive) placeInWorkspace(key, workspace string) error {
	if workspace != "" && workspace != workspaces.DefaultName {
		err := a.d.SetMember(key, workspace)
		if !errors.Is(err, workspaces.ReasonUnknown) {
			return err
		}
	}
	_ = a.d.Remove(key)
	return nil
}
