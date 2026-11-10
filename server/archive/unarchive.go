package archive

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
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
	name := freeActiveName(dir, base, len(halves) > 1)

	out := wire.TargetResult{Target: target, Status: wire.TargetDone}
	if name != base {
		out.NewAddress = project + ":" + name
		if asImpl {
			out.NewAddress += tickets.ImplEpicSuffix
		}
	}
	if err := a.restore(dir, project, halves, name, head); err != nil {
		if r, ok := errors.AsType[refusal](err); ok {
			return wire.RefusedTarget(target, r.reason, r.Error()), nil
		}
		return wire.TargetResult{}, err
	}
	return out, nil
}

// freeActiveName is the archived name, or the first free `-N` suffix when an
// active epic has it. A pair needs both `name` and `name-impl` free.
func freeActiveName(projectDir, base string, pair bool) string {
	taken := func(name string) bool {
		names := []string{name}
		if pair {
			names = append(names, name+tickets.ImplEpicSuffix)
		}
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(projectDir, n)); !os.IsNotExist(err) {
				return true
			}
		}
		return false
	}
	name := base
	for n := 2; taken(name); n++ {
		name = base + "-" + strconv.Itoa(n)
	}
	return name
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
		archiveKey := tickets.ArchiveKey(project+":"+h.epic, h.epic)
		a.rekey(tickets.ArchiveEpicKey, archiveKey, project+":"+finals[i])
	}
	a.placeInWorkspace(project+":"+finals[0], head.ArchivedWorkspace)
	a.d.Events.Publish(wire.EventWorkspacesChanged, "")
	return nil
}

// placeInWorkspace puts key back in its stored workspace, or in Default when
// none is stored or it no longer exists.
func (a *Archive) placeInWorkspace(key, workspace string) {
	if workspace != "" && workspace != workspaces.DefaultName && a.d.SetMember(key, workspace) == nil {
		return
	}
	_ = a.d.Remove(key)
}
