// Package archive moves finished loop epics into the project's .archive
// folder: it stops their live Sessions, clears the hold, stores the workspace
// in the frontmatter and re-keys the Sessions to the archive key.
package archive

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// DesignNotFinishedText is the refusal of a Design until Designs archive.
const DesignNotFinishedText = "a Design archives once its design step is done"

// Deps is what Archive reads from the Server. The embedded Directory is the
// one the workspace store uses, so Hold and Archive share one stop path.
type Deps struct {
	workspaces.Directory
	TicketStore string
	Events      *broker.Broker
	// Workspace names the workspace a key is in; Unhold clears its hold mark.
	Workspace func(key string) string
	Unhold    func(key string) error
	// RekeySession rewrites a Session's ticket and fix_of through rekey.
	RekeySession func(id string, rekey func(ticket, fixOf string) (string, string)) error
}

// Archive runs the archive writes.
type Archive struct {
	d  Deps
	mu sync.Mutex // one archive at a time, so two clashes cannot take the same suffix
}

// New builds Archive.
func New(d Deps) *Archive { return &Archive{d: d} }

// Epics archives each target on its own, in order. A refusal is a result and
// blocks no other target.
func (a *Archive) Epics(req wire.ArchiveRequest) (wire.TargetsResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(req.Targets))}
	for _, target := range req.Targets {
		r, err := a.one(target, req.Yes, req.DryRun)
		if err != nil {
			return wire.TargetsResult{}, err
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

// Cleanup maps the one-epic alias onto the same code.
func (a *Archive) Cleanup(project, epic string, yes bool) (wire.CleanupWriteResult, error) {
	res, err := a.Epics(wire.ArchiveRequest{Targets: []string{project + ":" + epic}, Yes: yes})
	if err != nil || len(res.Results) == 0 {
		return wire.CleanupWriteResult{}, err
	}
	if r := res.Results[0]; r.Refused() {
		return wire.CleanupWriteResult{Refused: true, Reason: r.Reason, Message: r.Message}, nil
	}
	return wire.CleanupWriteResult{}, nil
}

func (a *Archive) one(target string, yes, dryRun bool) (wire.TargetResult, error) {
	refuse := func(reason, msg string) (wire.TargetResult, error) {
		return wire.RefusedTarget(target, reason, msg), nil
	}
	project, epic, ok := tickets.ParseEpicAddress(strings.TrimSpace(target))
	if !ok {
		return refuse(wire.ReasonCleanupUnknown, "not an epic address (want project:epic)")
	}
	dir, err := tickets.ProjectDirByName(a.d.TicketStore, project)
	if err != nil {
		if _, isAddr := errors.AsType[*tickets.AddressError](err); isAddr {
			return refuse(wire.ReasonCleanupUnknown, err.Error())
		}
		return wire.TargetResult{}, err
	}
	key := project + ":" + epic
	if a.isDesign(key) {
		return refuse(wire.ReasonArchiveNotFinished, DesignNotFinishedText)
	}
	if a.d.RootQueued(key) {
		return refuse(wire.ReasonCleanupQueued, "the epic is queued")
	}
	repo := ticketgit.ProjectRepoRoot(a.d.TicketStore, tickets.ProjectName(dir))
	if repo == "" {
		return wire.TargetResult{}, fmt.Errorf("project %q has no repo", project)
	}
	row, err := cleanupscan.ScanEpic(a.d.TicketStore, repo, epic)
	if err != nil {
		return wire.TargetResult{}, err
	}
	switch {
	case row == nil:
		return refuse(wire.ReasonCleanupUnknown, "no such epic")
	case !row.AllDone:
		return refuse(wire.ReasonCleanupNotDone, "the epic has tickets that are not done")
	}
	name := freeName(tickets.ArchiveDir(dir), epic)
	live := workspaces.LiveEpicSessions(a.d, key, allKinds)
	out := wire.TargetResult{Target: target, Status: wire.TargetDone, Sessions: live, OnTrunk: &row.MergedToTrunk}
	if name != epic {
		out.NewAddress = project + ":" + name
	}
	if dryRun {
		out.Status = wire.TargetFits
		return out, nil
	}
	if len(live) > 0 && !yes {
		r := wire.RefusedTarget(target, wire.ReasonNeedsYes,
			fmt.Sprintf("archiving stops the live sessions: %s; confirm to continue", strings.Join(live, ", ")))
		r.Sessions = live
		return r, nil
	}
	out.OnTrunk = nil
	if err := a.write(dir, key, epic, name); err != nil {
		if r, ok := errors.AsType[refusal](err); ok {
			return refuse(r.reason, r.Error())
		}
		return wire.TargetResult{}, err
	}
	return out, nil
}

const allKinds = workspaces.OwnSessions | workspaces.FixSessions | workspaces.DesignSessions

type refusal struct {
	reason string
	err    error
}

func (r refusal) Error() string { return r.err.Error() }
func (r refusal) Unwrap() error { return r.err }

func (a *Archive) isDesign(key string) bool {
	if a.d.DesignExists(key) {
		return true
	}
	design, ok := strings.CutSuffix(key, tickets.ImplEpicSuffix)
	return ok && a.d.DesignExists(design)
}

// freeName is the epic's name, or the first free `-N` suffix when .archive has it.
func freeName(archiveDir, epic string) string {
	name := epic
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(archiveDir, name)); os.IsNotExist(err) {
			return name
		}
		name = epic + "-" + strconv.Itoa(n)
	}
}

// write runs the effects in order. Sessions the stop already ended stay stopped.
func (a *Archive) write(dir, key, epic, name string) error {
	if _, err := workspaces.StopEpicSessions(a.d, key, allKinds); err != nil {
		return err
	}
	if err := a.d.Unhold(key); err != nil {
		return err
	}
	workspace := a.d.Workspace(key)
	archive := tickets.ArchiveDir(dir)
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return err
	}
	branch := epic
	if e, ok := a.d.Epic(key); ok {
		branch = cmp.Or(e.BranchName, epic)
	}
	dest := filepath.Join(archive, name)
	if err := os.Rename(filepath.Join(dir, epic), dest); err != nil {
		return refusal{wire.ReasonArchiveRenameFailed, err}
	}
	fields := map[string]any{"archived_at": time.Now().UTC()}
	if workspace != workspaces.DefaultName {
		fields["archived_workspace"] = workspace
	}
	if name != epic {
		fields["branch_name"] = branch
	}
	if err := tickets.OverwriteEpicFields(dest, fields); err != nil {
		return err
	}
	a.rekey(key, name)
	a.d.Events.Publish(wire.EventWorkspacesChanged, "")
	return nil
}

// rekey points every Session of the epic, live or exited, at the archive key.
func (a *Archive) rekey(key, name string) {
	rekeyOne := func(addr string) string {
		if tickets.EpicAddress(addr) == key {
			return tickets.ArchiveKey(addr, name)
		}
		return addr
	}
	for _, si := range a.d.Sessions() {
		if tickets.EpicAddress(si.Ticket) != key && tickets.EpicAddress(si.FixOf) != key {
			continue
		}
		err := a.d.RekeySession(si.ID, func(ticket, fixOf string) (string, string) {
			return rekeyOne(ticket), rekeyOne(fixOf)
		})
		if err == nil {
			a.d.Events.Publish(wire.EventSessionChanged, si.ID)
		}
	}
}
